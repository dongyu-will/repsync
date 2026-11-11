package workspace

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"time"
)

const (
	maxReviewTerms    = 64
	maxReviewFiles    = 128
	maxReviewBytes    = 4 * 1024 * 1024
	maxReviewFindings = 50
)

var (
	reviewHunk    = regexp.MustCompile(`^@@ -(\d+)(?:,\d+)? \+\d+(?:,\d+)? @@`)
	reviewToken   = regexp.MustCompile(`/[A-Za-z][A-Za-z0-9._/-]{2,}|[A-Za-z][A-Za-z0-9_.-]{3,}`)
	reviewSymbol  = regexp.MustCompile(`^[A-Za-z][A-Za-z0-9_]{3,79}$`)
	reviewCamel   = regexp.MustCompile(`[a-z0-9][A-Z]`)
	reviewWord    = regexp.MustCompile(`[A-Za-z0-9_]`)
	reviewStorage = regexp.MustCompile(`(?i)mysql|mariadb|postgresql|postgres|sqlite|mongodb`)
)

type reviewTerm struct {
	text      string
	symbol    string
	successor string
	evidence  DocumentReviewEvidence
}

type reviewDocument struct {
	name    string
	version string
	lines   []string
	start   int
}

// ScanDocumentReview finds Markdown passages that refer to changed source terms.
func (s *Service) ScanDocumentReview(path, scope, keywords string) (DocumentReview, error) {
	result := DocumentReview{Findings: []DocumentFinding{}, Warnings: []string{}, Graph: "未查询"}
	if _, err := s.registeredProject(path); err != nil {
		return result, err
	}
	if len(keywords) > 1024 {
		return result, errors.New("补充关键词超过长度限制")
	}
	diff, basis, memories, warning, err := s.documentReviewContext(path, scope)
	if err != nil {
		return result, err
	}
	result.Basis = basis
	if warning != "" {
		result.Warnings = append(result.Warnings, warning)
	}
	terms, memoryByTerm := addReviewMemoryTerms(documentReviewTerms(diff, keywords), memories)
	if len(terms) == 0 {
		result.Warnings = append(result.Warnings, "未从所选变更提取到可查的旧名称，可以补充关键词")
		return result, nil
	}
	if len(terms) > maxReviewTerms {
		terms = terms[:maxReviewTerms]
		result.Warnings = append(result.Warnings, "关键词超过上限，本次仅检查前 64 项")
	}
	rg, err := reviewTool("rg")
	if err != nil {
		return result, errors.New("找不到 rg，请安装 ripgrep 后重新扫描")
	}
	documents, input, warnings, err := readReviewDocuments(path)
	if err != nil {
		return result, err
	}
	result.Files = len(documents)
	result.Warnings = append(result.Warnings, warnings...)
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	result.Findings, warnings, err = matchReviewDocuments(ctx, rg, terms, documents, input)
	if err != nil {
		return result, err
	}
	result.Warnings = append(result.Warnings, warnings...)
	if len(result.Findings) > 0 {
		addReviewMemoryEvidence(result.Findings, memoryByTerm)
		result.Graph = addDocumentGraphEvidence(ctx, path, terms, result.Findings)
	}
	if _, current, _, _, err := s.documentReviewContext(path, scope); err != nil || current != basis {
		return result, errors.New("扫描期间 Git 变更或已确认记忆发生变化，请重新扫描")
	}
	return result, nil
}

func (s *Service) documentReviewContext(path, scope string) (diff, basis string, memories []ProjectMemory, warning string, err error) {
	diff, gitBasis, err := documentReviewDiff(path, scope)
	if err != nil {
		return "", "", nil, "", err
	}
	memories, memoryErr := s.ConfirmedProjectMemories(path)
	if memoryErr != nil {
		warning = "项目记忆读取失败，本次仅检查代码和补充关键词"
	}
	return diff, gitDigest([]any{gitBasis, memories, warning}), memories, warning, nil
}

func addReviewMemoryTerms(terms []reviewTerm, memories []ProjectMemory) ([]reviewTerm, map[string][]ProjectMemory) {
	seen := map[string]bool{}
	for _, term := range terms {
		seen[strings.ToLower(term.text)] = true
	}
	byTerm := map[string][]ProjectMemory{}
	for _, item := range memories {
		for _, keyword := range item.Keywords {
			if strings.TrimSpace(keyword) == "" || len(keyword) > 100 || strings.ContainsAny(keyword, "\x00\r\n") {
				continue
			}
			key := strings.ToLower(keyword)
			byTerm[key] = append(byTerm[key], item)
			if !seen[key] {
				terms = append(terms, reviewTerm{text: keyword})
				seen[key] = true
			}
		}
	}
	return terms, byTerm
}

func addReviewMemoryEvidence(findings []DocumentFinding, byTerm map[string][]ProjectMemory) {
	for i := range findings {
		finding := &findings[i]
		hasOtherEvidence := len(finding.Evidence) > 0
		seen := map[string]bool{}
		for _, term := range finding.Terms {
			for _, item := range byTerm[strings.ToLower(term)] {
				if seen[item.ID] || len(seen) == 4 {
					continue
				}
				seen[item.ID] = true
				detail := item.Statement
				if item.Environment != "" {
					detail = item.Environment + " · " + detail
				}
				finding.Evidence = append(finding.Evidence, DocumentReviewEvidence{
					Source: "已确认项目记忆", Path: item.Source, Detail: detail, MemoryID: item.ID,
				})
			}
		}
		if len(seen) > 0 {
			if hasOtherEvidence {
				finding.Suggestion += "同时核对记忆的来源和适用环境。"
			} else {
				finding.Suggestion = "核对引用记忆的来源和适用环境，再决定改写、保留或删除本段。"
			}
		}
	}
}

func documentReviewDiff(path, scope string) (string, string, error) {
	args := []string{"diff", "--no-color", "--no-ext-diff", "--no-textconv", "--no-renames",
		"--ignore-submodules=all", "--relative", "--src-prefix=a/", "--dst-prefix=b/", "--unified=0"}
	head, err := git(path, "rev-parse", "--verify", "HEAD")
	if err != nil {
		return "", "", errors.New("文档核对需要已有提交的 Git 仓库")
	}
	switch scope {
	case "worktree":
		args = append(args, "HEAD")
	case "commit":
		args = append(args, "HEAD^1", "HEAD")
	default:
		return "", "", errors.New("请选择要核对的 Git 变更")
	}
	// Document edits do not change the source evidence for other findings.
	args = append(args, "--", ".", ":(exclude,glob,icase)**/*.md", ":(exclude,glob,icase)**/*.markdown")
	diff, err := git(path, args...)
	if err != nil {
		return "", "", fmt.Errorf("读取所选 Git 变更: %w", err)
	}
	if strings.HasSuffix(diff, "[输出超过 1 MiB，已截断]") {
		return "", "", errors.New("Git 变更超过 1 MiB，请缩小变更范围后再扫描")
	}
	return diff, gitDigest([]string{scope, head, diff}), nil
}

func documentReviewTerms(diff, keywords string) []reviewTerm {
	terms := []reviewTerm{}
	seen := map[string]bool{}
	add := func(term, symbol, successor string, evidence DocumentReviewEvidence) {
		if len(term) < 4 || len(term) > 100 || seen[strings.ToLower(term)] {
			return
		}
		seen[strings.ToLower(term)] = true
		terms = append(terms, reviewTerm{text: term, symbol: symbol, successor: successor, evidence: evidence})
	}
	for term := range strings.FieldsFuncSeq(keywords, func(r rune) bool { return strings.ContainsRune(",，\n", r) }) {
		term = strings.TrimSpace(term)
		add(term, "", "", DocumentReviewEvidence{Source: "补充关键词", Detail: term})
	}
	var name string
	var oldLine int
	var deleted bool
	var added strings.Builder
	type removedLine struct {
		line int
		text string
	}
	var removed []removedLine
	collect := func() {
		if name == "" {
			return
		}
		current := strings.ToLower(added.String())
		successors := map[string]bool{}
		for _, storage := range reviewStorage.FindAllString(current, -1) {
			successors[storageName(storage)] = true
		}
		var successor string
		if len(successors) == 1 {
			for name := range successors {
				successor = name
			}
		}
		for _, line := range removed {
			for _, token := range reviewToken.FindAllString(line.text, -1) {
				if strings.Contains(current, strings.ToLower(token)) {
					continue
				}
				evidence := DocumentReviewEvidence{Source: "Git diff", Path: name, Line: line.line, Detail: "移除 " + token}
				var symbol string
				camel := reviewCamel.MatchString(token)
				if (camel || strings.Contains(token, "_")) && reviewSymbol.MatchString(token) {
					symbol = token
				}
				for _, storage := range reviewStorage.FindAllString(token, -1) {
					if !strings.Contains(current, strings.ToLower(storage)) {
						next := successor
						if storageName(storage) == next {
							next = ""
						}
						add(storage, symbol, next, evidence)
					}
				}
				switch {
				case strings.HasPrefix(token, "/"):
					add(token, "", "", evidence)
					label := strings.ReplaceAll(strings.TrimPrefix(token, "/"), "-", " ")
					if label != strings.TrimPrefix(token, "/") {
						add(label, "", "", evidence)
					}
				case camel || strings.ContainsAny(token, "-_"):
					add(token, symbol, "", evidence)
					label := reviewCamel.ReplaceAllStringFunc(token, func(pair string) string { return pair[:1] + " " + pair[1:] })
					label = strings.NewReplacer("-", " ", "_", " ").Replace(label)
					add(strings.ToLower(label), symbol, "", evidence)
				}
			}
		}
		if deleted {
			base := strings.TrimSuffix(filepath.Base(name), filepath.Ext(name))
			if strings.ContainsAny(base, "-_") {
				evidence := DocumentReviewEvidence{Source: "Git diff", Path: name, Detail: "删除文件 " + filepath.Base(name)}
				add(base, "", "", evidence)
				add(strings.NewReplacer("-", " ", "_", " ").Replace(base), "", "", evidence)
			}
		}
	}
	for line := range strings.SplitSeq(diff, "\n") {
		switch {
		case strings.HasPrefix(line, "diff --git "):
			collect()
			name, oldLine, removed, deleted = "", 0, nil, false
			added.Reset()
		case strings.HasPrefix(line, "deleted file mode "):
			deleted = true
		case oldLine == 0 && strings.HasPrefix(line, "--- "):
			file := strings.TrimSuffix(strings.TrimPrefix(line, "--- "), "\t")
			if strings.HasPrefix(file, "\"") {
				file, _ = strconv.Unquote(file)
			}
			if strings.HasPrefix(file, "a/") {
				name = strings.TrimPrefix(file, "a/")
			}
		case strings.HasPrefix(line, "@@ "):
			if match := reviewHunk.FindStringSubmatch(line); len(match) > 0 {
				oldLine, _ = strconv.Atoi(match[1])
			}
		case oldLine > 0 && strings.HasPrefix(line, "-"):
			removed = append(removed, removedLine{oldLine, line[1:]})
			oldLine++
		case strings.HasPrefix(line, "+") && !strings.HasPrefix(line, "+++ "):
			added.WriteString(line[1:] + "\n")
		}
	}
	collect()
	return terms
}

func storageName(value string) string {
	switch strings.ToLower(value) {
	case "mysql":
		return "MySQL"
	case "mariadb":
		return "MariaDB"
	case "postgres", "postgresql":
		return "PostgreSQL"
	case "sqlite":
		return "SQLite"
	default:
		return "MongoDB"
	}
}

func readReviewDocuments(path string) ([]reviewDocument, string, []string, error) {
	list, err := git(path, "ls-files", "-z", "--cached", "--others", "--exclude-standard", "--", ".")
	if err != nil {
		return nil, "", nil, err
	}
	root, err := os.OpenRoot(path)
	if err != nil {
		return nil, "", nil, err
	}
	defer root.Close()
	var documents []reviewDocument
	var input strings.Builder
	var warnings []string
	seen := map[string]bool{}
	line := 1
	for name := range strings.SplitSeq(list, "\x00") {
		if seen[name] || !reviewDocumentPath(name) {
			continue
		}
		seen[name] = true
		if len(documents) == maxReviewFiles {
			warnings = append(warnings, "达到 128 个文档的扫描上限")
			break
		}
		content, err := readReviewDocument(root, name)
		if err != nil {
			if len(warnings) < 5 && !errors.Is(err, os.ErrNotExist) {
				warnings = append(warnings, "跳过 "+name+"："+err.Error())
			}
			continue
		}
		if input.Len()+len(content)+1 > maxReviewBytes {
			warnings = append(warnings, "达到 4 MiB 文档扫描上限")
			break
		}
		lines := reviewDocumentLines(content)
		if len(lines) == 0 {
			continue
		}
		documents = append(documents, reviewDocument{name, gitDigest(content), lines, line})
		input.WriteString(content)
		if !strings.HasSuffix(content, "\n") {
			input.WriteByte('\n')
		}
		line += len(lines)
	}
	if strings.HasSuffix(list, "[输出超过 1 MiB，已截断]") {
		warnings = append(warnings, "Git 文件清单被截断，部分文档未检查")
	}
	return documents, input.String(), warnings, nil
}

func matchReviewDocuments(ctx context.Context, rg string, terms []reviewTerm, documents []reviewDocument, input string) ([]DocumentFinding, []string, error) {
	findings := []DocumentFinding{}
	if input == "" {
		return findings, nil, nil
	}
	patterns := make([]string, 0, len(terms))
	byTerm := map[string]reviewTerm{}
	for _, term := range terms {
		pattern := regexp.QuoteMeta(term.text)
		if reviewWord.MatchString(term.text[:1]) {
			pattern = `(?-u:\b)` + pattern
		}
		if reviewWord.MatchString(term.text[len(term.text)-1:]) {
			pattern += `(?-u:\b)`
		}
		patterns = append(patterns, pattern)
		byTerm[strings.ToLower(term.text)] = term
	}
	cmd := exec.CommandContext(ctx, rg, "--no-config", "--json", "--ignore-case", "--regexp", strings.Join(patterns, "|"), "-")
	cmd.Stdin = strings.NewReader(input)
	var output, stderr limitedBuffer
	cmd.Stdout, cmd.Stderr = &output, &stderr
	var exit *exec.ExitError
	if err := cmd.Run(); err != nil && !(errors.As(err, &exit) && exit.ExitCode() == 1 && ctx.Err() == nil) {
		return nil, nil, fmt.Errorf("rg 查找文档: %w %s", err, strings.TrimSpace(stderr.String()))
	}
	var warnings []string
	index := map[[2]int]int{}
	docIndex := 0
	lastDoc, start, end := -1, 0, 0
	for entry := range strings.SplitSeq(output.String(), "\n") {
		var event struct {
			Type string
			Data struct {
				Line       int `json:"line_number"`
				Submatches []struct {
					Match struct{ Text string }
				}
			}
		}
		if json.Unmarshal([]byte(entry), &event) != nil || event.Type != "match" {
			continue
		}
		for docIndex+1 < len(documents) && documents[docIndex+1].start <= event.Data.Line {
			docIndex++
		}
		doc := documents[docIndex]
		hit := event.Data.Line - doc.start
		if hit < 0 || hit >= len(doc.lines) {
			continue
		}
		if docIndex != lastDoc || hit < start || hit >= end {
			start, end = reviewParagraph(doc.lines, hit)
			lastDoc = docIndex
		}
		key := [2]int{docIndex, start}
		itemIndex, exists := index[key]
		if !exists {
			if len(findings) == maxReviewFindings {
				warnings = append(warnings, "仅显示前 50 个段落，请缩小扫描范围后再检查其余内容")
				break
			}
			original := strings.Join(doc.lines[start:end], "")
			if len(original) > 8192 {
				if !slices.Contains(warnings, "部分段落超过 8 KiB，已跳过") {
					warnings = append(warnings, "部分段落超过 8 KiB，已跳过")
				}
				continue
			}
			itemIndex = len(findings)
			index[key] = itemIndex
			findings = append(findings, DocumentFinding{
				Path: doc.name, StartLine: start + 1, EndLine: end, Original: original, Version: doc.version,
				Terms: []string{}, Evidence: []DocumentReviewEvidence{},
			})
		}
		item := &findings[itemIndex]
		for _, match := range event.Data.Submatches {
			term, ok := byTerm[strings.ToLower(match.Match.Text)]
			if !ok || slices.Contains(item.Terms, term.text) {
				continue
			}
			item.Terms = append(item.Terms, term.text)
			if term.successor != "" && item.Suggestion == "" {
				item.Suggestion = fmt.Sprintf("核实 %s 的实际配置后，改写本段的启动、连接或运维步骤。旧说明若仍用于恢复，可保留在恢复指南。", term.successor)
			}
			if term.evidence.Source != "" && !slices.Contains(item.Evidence, term.evidence) {
				item.Evidence = append(item.Evidence, term.evidence)
			}
		}
	}
	if output.overflow {
		warnings = append(warnings, "rg 输出达到上限，结果仅覆盖部分段落")
	}
	for i := range findings {
		item := &findings[i]
		if item.Suggestion == "" {
			item.Suggestion = "核对 " + strings.Join(item.Terms, "、") + " 的现行入口。确认已下线且无保留用途后，删除对应说明和链接。"
		}
	}
	return findings, warnings, nil
}

func reviewParagraph(lines []string, hit int) (int, int) {
	// Keep fenced code blocks intact, including blank lines inside a block.
	open, marker := -1, ""
	for i, line := range lines {
		trimmed := strings.TrimSpace(line)
		if open >= 0 && strings.HasPrefix(trimmed, marker) && strings.Trim(trimmed, marker[:1]) == "" {
			if hit >= open && hit <= i {
				return open, i + 1
			}
			open = -1
		} else if open < 0 && (strings.HasPrefix(trimmed, "```") || strings.HasPrefix(trimmed, "~~~")) {
			open = i
			marker = trimmed[:3]
			for len(marker) < len(trimmed) && trimmed[len(marker)] == marker[0] {
				marker = trimmed[:len(marker)+1]
			}
		}
		if open < 0 && i >= hit {
			break
		}
	}
	if open >= 0 && hit >= open {
		return open, len(lines)
	}
	if strings.HasPrefix(strings.TrimSpace(lines[hit]), "#") {
		return hit, hit + 1
	}
	start, end := hit, hit+1
	boundary := func(line string) bool {
		line = strings.TrimSpace(line)
		return line == "" || strings.HasPrefix(line, "#") || strings.HasPrefix(line, "```") || strings.HasPrefix(line, "~~~")
	}
	for start > 0 && !boundary(lines[start-1]) {
		start--
	}
	for end < len(lines) && !boundary(lines[end]) {
		end++
	}
	return start, end
}

func reviewTool(name string) (string, error) {
	if binary, err := exec.LookPath(name); err == nil {
		return binary, nil
	}
	home, _ := os.UserHomeDir()
	for _, directory := range []string{filepath.Join(home, ".local", "bin"), "/opt/homebrew/bin", "/usr/local/bin"} {
		if binary, err := exec.LookPath(filepath.Join(directory, name)); err == nil {
			return binary, nil
		}
	}
	return "", exec.ErrNotFound
}

func runDocumentGraph(ctx context.Context, binary, path string, args ...string) ([]byte, error) {
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, binary, args...)
	cmd.Dir = path
	cmd.Env = append(os.Environ(), "CODEGRAPH_TELEMETRY=0", "CODEGRAPH_NO_DAEMON=1")
	cmd.WaitDelay = time.Second
	var output limitedBuffer
	cmd.Stdout = &output
	if err := cmd.Run(); err != nil {
		return nil, err
	}
	if output.overflow {
		return nil, errors.New("CodeGraph 输出超过 1 MiB")
	}
	return output.Bytes(), nil
}

func addDocumentGraphEvidence(ctx context.Context, path string, terms []reviewTerm, findings []DocumentFinding) string {
	binary, err := reviewTool("codegraph")
	if err != nil {
		return "未找到 CodeGraph"
	}
	var status struct {
		Initialized bool
		ProjectPath string
	}
	data, err := runDocumentGraph(ctx, binary, path, "status", "--json", ".")
	if err != nil || json.Unmarshal(data, &status) != nil {
		return "CodeGraph 状态读取失败"
	}
	if !status.Initialized || status.ProjectPath != path {
		return "当前目录尚未建立 CodeGraph 索引"
	}
	if _, err := runDocumentGraph(ctx, binary, path, "sync", "."); err != nil {
		return "CodeGraph 索引同步失败"
	}
	symbols := map[string]string{}
	for _, term := range terms {
		symbols[term.text] = term.symbol
	}
	cache := map[string][]DocumentReviewEvidence{}
	partial := false
	for i := range findings {
		for _, term := range findings[i].Terms {
			symbol := symbols[term]
			if symbol == "" {
				continue
			}
			evidence, ok := cache[symbol]
			if !ok {
				if len(cache) >= 8 || ctx.Err() != nil {
					partial = true
					continue
				}
				evidence, err = documentGraphCallers(ctx, binary, path, symbol)
				partial = partial || err != nil
				cache[symbol] = evidence
			}
			for _, item := range evidence {
				if !slices.Contains(findings[i].Evidence, item) {
					findings[i].Evidence = append(findings[i].Evidence, item)
				}
			}
		}
	}
	switch {
	case partial:
		return "CodeGraph 仅核对了部分符号"
	case len(cache) == 0:
		return "CodeGraph 索引已同步，未提取到可查符号"
	default:
		return "CodeGraph 已核对可查符号"
	}
}

func documentGraphCallers(ctx context.Context, binary, path, symbol string) ([]DocumentReviewEvidence, error) {
	data, err := runDocumentGraph(ctx, binary, path, "query", "--json", "--limit", "8", "--", symbol)
	if err != nil {
		return nil, err
	}
	type queryMatch struct{ Node struct{ Name string } }
	var matches []queryMatch
	if err := json.Unmarshal(data, &matches); err != nil {
		return nil, err
	}
	if !slices.ContainsFunc(matches, func(match queryMatch) bool { return match.Node.Name == symbol }) {
		return []DocumentReviewEvidence{{Source: "CodeGraph", Detail: "索引查询未匹配 " + symbol}}, nil
	}
	data, err = runDocumentGraph(ctx, binary, path, "callers", "--json", "--limit", "3", "--", symbol)
	if err != nil {
		return nil, err
	}
	var graph struct {
		Targets []struct {
			Name      string
			FilePath  string
			StartLine int
		}
		Callers []struct {
			Name      string
			FilePath  string
			StartLine int
		}
	}
	if err := json.Unmarshal(data, &graph); err != nil {
		return nil, err
	}
	if len(graph.Targets) == 0 {
		return nil, errors.New("CodeGraph 未返回匹配的符号定义")
	}
	evidence := []DocumentReviewEvidence{{Source: "CodeGraph", Path: graph.Targets[0].FilePath, Line: graph.Targets[0].StartLine, Detail: "当前索引仍找到 " + symbol}}
	for _, caller := range graph.Callers {
		evidence = append(evidence, DocumentReviewEvidence{Source: "CodeGraph", Path: caller.FilePath, Line: caller.StartLine, Detail: "引用 " + symbol + "：" + caller.Name})
	}
	return evidence, nil
}
