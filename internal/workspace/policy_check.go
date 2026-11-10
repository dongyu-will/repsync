package workspace

import (
	"bytes"
	"context"
	_ "embed"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"syscall"
	"time"
)

//go:embed policy_engine.cjs
var policyEngine string

type PolicyFinding struct {
	RuleID  string `json:"ruleId"`
	Path    string `json:"path"`
	Line    int    `json:"line"`
	Column  int    `json:"column"`
	Message string `json:"message"`
	Level   string `json:"level"`
	Kind    string `json:"kind"`
}

type PolicyReport struct {
	RulesChecked []string        `json:"rulesChecked"`
	Skipped      []string        `json:"skipped"`
	Findings     []PolicyFinding `json:"findings"`
	Errors       []string        `json:"errors"`
	Baseline     string          `json:"baseline"`
	Passed       bool            `json:"passed"`
	Complete     bool            `json:"complete"`
	CheckedAt    string          `json:"checkedAt"`
}

func policyPattern(pattern string) (*regexp.Regexp, error) {
	if pattern == "" || strings.HasPrefix(pattern, "/") || strings.Contains(pattern, "\\") || strings.Contains(pattern, "..") || len(pattern) > 256 || strings.ContainsAny(pattern, "\x00\r\n") {
		return nil, errors.New("规则路径需要项目内的相对模式，支持 *、** 和 ?")
	}
	var out strings.Builder
	out.WriteString("^")
	for i := 0; i < len(pattern); i++ {
		switch pattern[i] {
		case '*':
			if i+1 < len(pattern) && pattern[i+1] == '*' {
				i++
				if i+1 < len(pattern) && pattern[i+1] == '/' {
					i++
					out.WriteString("(?:.*/)?")
				} else {
					out.WriteString(".*")
				}
			} else {
				out.WriteString("[^/]*")
			}
		case '?':
			out.WriteString("[^/]")
		default:
			out.WriteString(regexp.QuoteMeta(string(pattern[i])))
		}
	}
	out.WriteString("$")
	return regexp.Compile(out.String())
}

func matchesPolicy(patterns []string, name string) bool {
	for _, pattern := range patterns {
		re, err := policyPattern(pattern)
		if err == nil && re.MatchString(name) {
			return true
		}
	}
	return false
}

func excludedByMarker(rule PolicyRule, content string) bool {
	for _, line := range strings.Split(content, "\n") {
		for _, marker := range rule.ExcludeMarkers {
			if strings.TrimSpace(line) == marker {
				return true
			}
		}
	}
	return false
}

type policyFile struct {
	Path     string `json:"path"`
	Content  string `json:"content"`
	Baseline string `json:"-"`
	Known    bool   `json:"-"`
	Added    bool   `json:"-"`
}

func policyGit(ctx context.Context, path string, args ...string) (string, error) {
	cmd := exec.CommandContext(ctx, "git", append([]string{"--no-optional-locks", "-c", "core.fsmonitor=false", "-C", path}, args...)...)
	cmd.Env = commandEnvironment(false)
	var output, diagnostics limitedBuffer
	cmd.Stdout, cmd.Stderr = &output, &diagnostics
	if err := cmd.Run(); err != nil {
		return "", fmt.Errorf("Git 基线读取失败: %w %s", err, diagnostics.String())
	}
	if output.overflow {
		return "", errors.New("Git 文件列表或基线超过 1 MiB")
	}
	return output.String(), nil
}

func policyFiles(ctx context.Context, path string) ([]string, map[string]bool, bool, error) {
	names := []string{}
	tracked := map[string]bool{}
	if _, err := policyGit(ctx, path, "rev-parse", "--git-dir"); err == nil {
		list, err := policyGit(ctx, path, "ls-files", "-z", "--cached", "--others", "--exclude-standard")
		if err != nil {
			return nil, nil, false, err
		}
		for _, name := range strings.Split(list, "\x00") {
			if name != "" {
				names = append(names, name)
			}
		}
		_, err = policyGit(ctx, path, "rev-parse", "--verify", "HEAD")
		if err != nil {
			return names, tracked, false, nil
		}
		base, err := policyGit(ctx, path, "ls-tree", "-r", "-z", "--name-only", "HEAD")
		if err != nil {
			return nil, nil, false, err
		}
		for _, name := range strings.Split(base, "\x00") {
			tracked[name] = true
		}
		return names, tracked, true, nil
	}
	err := filepath.WalkDir(path, func(name string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			if name != path && slices.Contains([]string{".git", "node_modules", "vendor", "build", "dist", ".mygo"}, d.Name()) {
				return filepath.SkipDir
			}
			return nil
		}
		relative, err := filepath.Rel(path, name)
		if err != nil {
			return err
		}
		names = append(names, filepath.ToSlash(relative))
		slices.Sort(names)
		names = slices.Compact(names)
		if len(names) > 5000 {
			return errors.New("项目文件超过 5000 个，请缩小检查范围")
		}
		return nil
	})
	return names, tracked, false, err
}

var policyHunk = regexp.MustCompile(`(?m)^@@ -[0-9]+(?:,[0-9]+)? \+([0-9]+)(?:,([0-9]+))? @@`)

func policyAddedLines(before, after string) (map[int]bool, error) {
	result := map[int]bool{}
	if before == after {
		return result, nil
	}
	diff, err := documentDiff(before, after)
	if err != nil {
		return nil, err
	}
	line := 0
	for _, text := range strings.Split(diff, "\n") {
		if match := policyHunk.FindStringSubmatch(text); match != nil {
			line, _ = strconv.Atoi(match[1])
			continue
		}
		if line == 0 || text == "" {
			continue
		}
		switch text[0] {
		case '+':
			result[line] = true
			line++
		case ' ':
			line++
		}
	}

	return result, nil
}

func (s *Service) CheckPolicies(path string) (PolicyReport, error) {
	if _, err := s.registeredProject(path); err != nil {
		return PolicyReport{}, err
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	s.taskMu.Lock()
	if s.closing {
		s.taskMu.Unlock()
		cancel()
		return PolicyReport{}, errors.New("应用正在退出")
	}
	if _, err := s.registeredProject(path); err != nil {
		s.taskMu.Unlock()
		cancel()
		return PolicyReport{}, err
	}
	if s.activePolicyChecks[path] != nil {
		s.taskMu.Unlock()
		cancel()
		return PolicyReport{}, errors.New("当前项目的规则检查正在运行")
	}
	if s.activePolicyChecks == nil {
		s.activePolicyChecks = map[string]context.CancelFunc{}
	}
	s.activePolicyChecks[path] = cancel
	s.taskWG.Add(1)
	s.taskMu.Unlock()
	defer func() {
		cancel()
		s.taskMu.Lock()
		delete(s.activePolicyChecks, path)
		s.taskMu.Unlock()
		s.taskWG.Done()
	}()
	return checkPolicies(ctx, path, s.path)
}

func checkPolicies(ctx context.Context, path, registry string) (PolicyReport, error) {
	canonical, err := filepath.EvalSymlinks(path)
	if err != nil {
		return PolicyReport{}, err
	}
	info, err := os.Stat(canonical)
	if err != nil {
		return PolicyReport{}, err
	}
	if !info.IsDir() {
		return PolicyReport{}, errors.New("规则检查需要项目目录")
	}
	path = canonical
	config, err := projectPolicies(path, registry)
	if err != nil {
		return PolicyReport{}, err
	}
	report := PolicyReport{RulesChecked: []string{}, Skipped: []string{}, Findings: []PolicyFinding{}, Errors: []string{}, Baseline: "not-checked", Passed: true, Complete: true, CheckedAt: time.Now().UTC().Format(time.RFC3339)}
	rules := []PolicyRule{}
	blockingIncomplete := false
	for _, entry := range config.Effective {
		if entry.Applies && slices.Contains([]string{"warn", "error"}, entry.Rule.Level) {
			rules = append(rules, entry.Rule)
			report.RulesChecked = append(report.RulesChecked, entry.Rule.ID)
		}
	}
	if len(rules) == 0 {
		return report, nil
	}
	names, tracked, known, err := policyFiles(ctx, path)
	if err != nil {
		return report, err
	}
	if !known {
		report.Baseline = "unavailable"
	} else {
		report.Baseline = "HEAD"
	}
	if len(names) > 5000 {
		return report, errors.New("项目文件超过 5000 个，请缩小检查范围")
	}
	root, err := os.OpenRoot(path)
	if err != nil {
		return report, err
	}
	defer root.Close()
	files := map[string]policyFile{}
	errorFiles := map[string]bool{}
	jsFiles := []policyFile{}
	var reactRule *PolicyRule
	for _, r := range rules {
		if r.ID == "react-no-use-state" {
			copy := r
			reactRule = &copy
		}
		if r.ID == "no-unit-tests" && len(r.Include) == 0 {
			report.Errors = append(report.Errors, "禁止单元测试：尚未配置单元测试路径及例外，未推测测试语义")
			blockingIncomplete = blockingIncomplete || r.Level == "error"
		}
	}
	prefix := ""
	if known {
		prefix, err = policyGit(ctx, path, "rev-parse", "--show-prefix")
		if err != nil {
			return report, err
		}
		prefix = strings.TrimSpace(prefix)
	}
	total := 0
	for _, name := range names {
		if err := ctx.Err(); err != nil {
			return report, err
		}
		needed := false
		neededError := false
		reactCandidate := false
		for _, r := range rules {
			if matchesPolicy(r.Exclude, name) {
				continue
			}
			if r.ID == "no-unit-tests" && matchesPolicy(r.Include, name) {
				needed = true
				neededError = neededError || r.Level == "error"
			}
			if r.ID == "react-no-use-state" && slices.Contains([]string{".js", ".jsx", ".mjs", ".cjs", ".ts", ".tsx", ".mts", ".cts"}, filepath.Ext(name)) && (len(r.Include) == 0 || matchesPolicy(r.Include, name)) {
				needed = true
				neededError = neededError || r.Level == "error"
				reactCandidate = true
			}
		}
		if !needed {
			continue
		}
		errorFiles[name] = neededError
		info, err := root.Lstat(name)
		if errors.Is(err, os.ErrNotExist) {
			continue
		}
		if err != nil || !info.Mode().IsRegular() {
			report.Errors = append(report.Errors, "无法检查普通源文件："+name)
			blockingIncomplete = blockingIncomplete || neededError
			continue
		}
		f, err := root.Open(name)
		if err != nil {
			report.Errors = append(report.Errors, name+": "+err.Error())
			blockingIncomplete = blockingIncomplete || neededError
			continue
		}
		content, err := readLimitedText(f)
		f.Close()
		if err != nil {
			report.Errors = append(report.Errors, name+": "+err.Error())
			blockingIncomplete = blockingIncomplete || neededError
			continue
		}
		total += len(content)
		if total > 40*1024*1024 {
			return report, errors.New("规则检查源码超过 40 MiB")
		}
		file := policyFile{Path: name, Content: content, Known: known, Added: known && !tracked[name]}
		if known && tracked[name] {
			file.Baseline, err = policyGit(ctx, path, "show", "HEAD:"+prefix+name)
			if err != nil {
				report.Errors = append(report.Errors, name+": "+err.Error())
				blockingIncomplete = blockingIncomplete || neededError
				continue
			}
		}
		files[name] = file
		if reactCandidate && !excludedByMarker(*reactRule, content) {
			jsFiles = append(jsFiles, file)
		}
	}
	changed := map[string]map[int]bool{}
	kindAt := func(file policyFile, line int) string {
		if !file.Known {
			return "unknown"
		}
		if file.Added {
			return "new"
		}
		lines, ok := changed[file.Path]
		if !ok {
			var err error
			lines, err = policyAddedLines(file.Baseline, file.Content)
			if err != nil {
				report.Errors = append(report.Errors, err.Error())
				blockingIncomplete = blockingIncomplete || errorFiles[file.Path]
				return "unknown"
			}
			changed[file.Path] = lines
		}
		if lines[line] {
			return "new"
		}
		return "historical"
	}
	for _, r := range rules {
		if r.ID != "no-unit-tests" {
			continue
		}
		for _, file := range files {
			if !matchesPolicy(r.Include, file.Path) || matchesPolicy(r.Exclude, file.Path) || excludedByMarker(r, file.Content) {
				continue
			}
			line := 1
			kind := kindAt(file, line)
			if file.Known && !file.Added {
				first := 0
				for n := range changed[file.Path] {
					if first == 0 || n < first {
						first = n
					}
				}
				if first > 0 {
					line, kind = first, "new"
				}
			}
			report.Findings = append(report.Findings, PolicyFinding{RuleID: r.ID, Path: file.Path, Line: line, Column: 1, Level: r.Level, Kind: kind, Message: "匹配项目声明的单元测试路径；新增文件及新增行受此规则限制"})
		}
	}
	if reactRule != nil && len(jsFiles) > 0 {
		previousErrors := len(report.Errors)
		directory, err := commandDirectory(path, reactRule.Cwd)
		if err != nil {
			report.Errors = append(report.Errors, err.Error())
			blockingIncomplete = blockingIncomplete || reactRule.Level == "error"
			report.Complete = false
			report.Passed = !blockingIncomplete
			for _, finding := range report.Findings {
				if finding.Level == "error" && finding.Kind != "historical" {
					report.Passed = false
				}
			}
			return report, nil
		}
		payload, _ := json.Marshal(struct {
			Root        string       `json:"root"`
			ProjectRoot string       `json:"projectRoot"`
			Files       []policyFile `json:"files"`
		}{Root: directory, ProjectRoot: path, Files: jsFiles})
		cmd := exec.CommandContext(ctx, "node", "-e", policyEngine)
		cmd.Dir = path
		cmd.Env = commandEnvironment(false)
		cmd.Stdin = bytes.NewReader(payload)
		configureCommandProcess(cmd, false)
		var output, diagnostics limitedBuffer
		cmd.Stdout, cmd.Stderr = &output, &diagnostics
		if err := cmd.Run(); err != nil {
			report.Errors = append(report.Errors, "React 规则需要项目已有的 Node.js、ESLint 9+ 与 flat config："+diagnostics.String()+" "+err.Error())
		} else if output.overflow {
			report.Errors = append(report.Errors, "React 检查输出超过 1 MiB")
		} else {
			var result struct {
				Findings []PolicyFinding `json:"findings"`
				Errors   []string        `json:"errors"`
				Skipped  []string        `json:"skipped"`
			}
			if err := json.Unmarshal(output.Bytes(), &result); err != nil {
				report.Errors = append(report.Errors, "无法解析 ESLint 检查结果："+err.Error())
			} else {
				report.Errors = append(report.Errors, result.Errors...)
				report.Skipped = append(report.Skipped, result.Skipped...)
				for _, finding := range result.Findings {
					finding.RuleID = reactRule.ID
					finding.Level = reactRule.Level
					finding.Kind = kindAt(files[finding.Path], finding.Line)
					report.Findings = append(report.Findings, finding)
				}
			}
		}
		if len(report.Errors) > previousErrors && reactRule.Level == "error" {
			blockingIncomplete = true
		}
	}
	slices.SortFunc(report.Findings, func(a, b PolicyFinding) int {
		if a.Path != b.Path {
			return strings.Compare(a.Path, b.Path)
		}
		return a.Line - b.Line
	})
	report.Complete = len(report.Errors) == 0
	if blockingIncomplete {
		report.Passed = false
	}
	for _, f := range report.Findings {
		if f.Level == "error" && f.Kind != "historical" {
			report.Passed = false
		}
	}
	return report, nil
}

// RunPolicyCheckCLI permits existing project scripts to use the same gate without a running app.
func RunPolicyCheckCLI(args []string, out, diagnostics io.Writer) int {
	if len(args) == 1 && (args[0] == "--help" || args[0] == "-h") {
		fmt.Fprintln(out, "用法：repsync check-rules PATH")
		return 0
	}
	if len(args) != 1 {
		fmt.Fprintln(diagnostics, "用法：repsync check-rules PATH")
		return 2
	}
	path, err := filepath.Abs(args[0])
	if err != nil {
		fmt.Fprintln(diagnostics, err)
		return 2
	}
	config, err := os.UserConfigDir()
	if err != nil {
		fmt.Fprintln(diagnostics, err)
		return 1
	}
	interrupted, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	ctx, cancel := context.WithTimeout(interrupted, 30*time.Second)
	defer cancel()
	report, err := checkPolicies(ctx, path, filepath.Join(config, "repsync", "projects.json"))
	if err != nil {
		fmt.Fprintln(diagnostics, err)
		return 1
	}
	encoder := json.NewEncoder(out)
	encoder.SetIndent("", "  ")
	if err := encoder.Encode(report); err != nil {
		fmt.Fprintln(diagnostics, err)
		return 1
	}
	if !report.Passed {
		return 1
	}
	return 0
}
