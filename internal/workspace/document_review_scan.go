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
	result := DocumentReview{Findings: []DocumentFinding{}, Warnings: []string{}, Graph: "Not queried"}
	if _, err := s.registeredProject(path); err != nil {
		return result, err
	}
	if len(keywords) > 1024 {
		return result, errors.New("additional keywords exceed the length limit")
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
		result.Warnings = append(result.Warnings, "No searchable old names extracted from the changes; add keywords if needed")
		return result, nil
	}
	if len(terms) > maxReviewTerms {
		terms = terms[:maxReviewTerms]
		result.Warnings = append(result.Warnings, "Keyword limit exceeded; only the first 64 are checked")
	}
	rg, err := reviewTool("rg")
	if err != nil {
		return result, errors.New("rg not found; install ripgrep and rescan")
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
		return result, errors.New("Git changes or confirmed memories changed during scanning; rescan")
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
		warning = "Could not read project memory; checking code and additional keywords only"
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
					Source: "Confirmed project memories", Path: item.Source, Detail: detail, MemoryID: item.ID,
				})
			}
		}
		if len(seen) > 0 {
			if hasOtherEvidence {
				finding.Suggestion += "Also verify the memory's source and applicable environment."
			} else {
				finding.Suggestion = "Verify the cited memory's source and environment before rewriting, keeping, or deleting this paragraph."
			}
		}
	}
}

func documentReviewDiff(path, scope string) (string, string, error) {
	args := []string{"diff", "--no-color", "--no-ext-diff", "--no-textconv", "--no-renames",
		"--ignore-submodules=all", "--relative", "--src-prefix=a/", "--dst-prefix=b/", "--unified=0"}
	head, err := git(path, "rev-parse", "--verify", "HEAD")
	if err != nil {
		return "", "", errors.New("document review requires a Git repository with commits")
	}
	switch scope {
	case "worktree":
		args = append(args, "HEAD")
	case "commit":
		args = append(args, "HEAD^1", "HEAD")
	default:
		return "", "", errors.New("select Git changes to review")
	}
	// Document edits do not change the source evidence for other findings.
	args = append(args, "--", ".", ":(exclude,glob,icase)**/*.md", ":(exclude,glob,icase)**/*.markdown")
	diff, err := git(path, args...)
	if err != nil {
		return "", "", fmt.Errorf("read selected Git changes: %w", err)
	}
	if strings.HasSuffix(diff, "[output exceeds 1 MiB; truncated]") {
		return "", "", errors.New("Git changes exceed 1 MiB; narrow the scope before scanning")
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
		add(term, "", "", DocumentReviewEvidence{Source: "Additional keywords", Detail: term})
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
				evidence := DocumentReviewEvidence{Source: "Git diff", Path: name, Line: line.line, Detail: "Removed " + token}
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
				evidence := DocumentReviewEvidence{Source: "Git diff", Path: name, Detail: "Deleted file " + filepath.Base(name)}
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
			warnings = append(warnings, "Reached the scan limit of 128 documents")
			break
		}
		content, err := readReviewDocument(root, name)
		if err != nil {
			if len(warnings) < 5 && !errors.Is(err, os.ErrNotExist) {
				warnings = append(warnings, "Skipped "+name+": "+err.Error())
			}
			continue
		}
		if input.Len()+len(content)+1 > maxReviewBytes {
			warnings = append(warnings, "Reached the 4 MiB document scan limit")
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
	if strings.HasSuffix(list, "[output exceeds 1 MiB; truncated]") {
		warnings = append(warnings, "Git file list truncated; some documents not checked")
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
		return nil, nil, fmt.Errorf("rg document search: %w %s", err, strings.TrimSpace(stderr.String()))
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
				warnings = append(warnings, "Showing the first 50 paragraphs; narrow the scope to review the rest")
				break
			}
			original := strings.Join(doc.lines[start:end], "")
			if len(original) > 8192 {
				if !slices.Contains(warnings, "Some paragraphs exceed 8 KiB and were skipped") {
					warnings = append(warnings, "Some paragraphs exceed 8 KiB and were skipped")
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
				item.Suggestion = fmt.Sprintf("Verify the actual %s configuration, then rewrite startup, connection, or operational steps. Keep old instructions in recovery guides if still needed.", term.successor)
			}
			if term.evidence.Source != "" && !slices.Contains(item.Evidence, term.evidence) {
				item.Evidence = append(item.Evidence, term.evidence)
			}
		}
	}
	if output.overflow {
		warnings = append(warnings, "rg output reached the limit; results cover only some paragraphs")
	}
	for i := range findings {
		item := &findings[i]
		if item.Suggestion == "" {
			item.Suggestion = "Verify " + strings.Join(item.Terms, ", ") + " current entry points. Remove instructions and links only after confirming retirement and no remaining use."
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
		return nil, errors.New("CodeGraph output exceeds 1 MiB")
	}
	return output.Bytes(), nil
}

func addDocumentGraphEvidence(ctx context.Context, path string, terms []reviewTerm, findings []DocumentFinding) string {
	binary, err := reviewTool("codegraph")
	if err != nil {
		return "CodeGraph not found"
	}
	var status struct {
		Initialized bool
		ProjectPath string
	}
	data, err := runDocumentGraph(ctx, binary, path, "status", "--json", ".")
	if err != nil || json.Unmarshal(data, &status) != nil {
		return "Failed to read CodeGraph status"
	}
	if !status.Initialized || status.ProjectPath != path {
		return "No CodeGraph index for this folder"
	}
	if _, err := runDocumentGraph(ctx, binary, path, "sync", "."); err != nil {
		return "CodeGraph index sync failed"
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
		return "CodeGraph checked only some symbols"
	case len(cache) == 0:
		return "CodeGraph index synced; no queryable symbols extracted"
	default:
		return "CodeGraph checked queryable symbols"
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
		return []DocumentReviewEvidence{{Source: "CodeGraph", Detail: "No index match for " + symbol}}, nil
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
		return nil, errors.New("CodeGraph returned no matching symbol definitions")
	}
	evidence := []DocumentReviewEvidence{{Source: "CodeGraph", Path: graph.Targets[0].FilePath, Line: graph.Targets[0].StartLine, Detail: "Current index still contains " + symbol}}
	for _, caller := range graph.Callers {
		evidence = append(evidence, DocumentReviewEvidence{Source: "CodeGraph", Path: caller.FilePath, Line: caller.StartLine, Detail: "Reference " + symbol + ": " + caller.Name})
	}
	return evidence, nil
}
