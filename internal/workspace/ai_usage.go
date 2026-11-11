package workspace

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"io/fs"
	"net/url"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"
)

type AIUsageEvent struct {
	ID         string `json:"id"`
	ResourceID string `json:"resourceId"`
	Kind       string `json:"kind"`
	Name       string `json:"name"`
	Action     string `json:"action"`
	Status     string `json:"status"`
	Purpose    string `json:"purpose"`
	Actor      string `json:"actor"`
	Project    string `json:"project"`
	Session    string `json:"session"`
	Timestamp  string `json:"timestamp"`
	Source     string `json:"source"`
	Line       int    `json:"line"`
	Detail     string `json:"detail"`
}

type AIUsageSummary struct {
	ResourceID string `json:"resourceId"`
	Kind       string `json:"kind"`
	Name       string `json:"name"`
	Sessions   int    `json:"sessions"`
	Calls      int    `json:"calls"`
	Succeeded  int    `json:"succeeded"`
	Failed     int    `json:"failed"`
	Unknown    int    `json:"unknown"`
	Declined   int    `json:"declined"`
	Cancelled  int    `json:"cancelled"`
	Probes     int    `json:"probes"`
	AppRuns    int    `json:"appRuns"`
	LastUsed   string `json:"lastUsed"`
}

type AIUsageCoverage struct {
	Source       string   `json:"source"`
	Status       string   `json:"status"`
	Files        int      `json:"files"`
	Sessions     int      `json:"sessions"`
	PartialFiles int      `json:"partialFiles"`
	Versions     []string `json:"versions"`
	Detail       string   `json:"detail"`
}

type AIUsageReport struct {
	Since     string            `json:"since"`
	Until     string            `json:"until"`
	Summaries []AIUsageSummary  `json:"summaries"`
	Events    []AIUsageEvent    `json:"events"`
	Coverage  []AIUsageCoverage `json:"coverage"`
	Issues    []string          `json:"issues"`
}

// AIUsage reads bounded local evidence on demand. No conversation body or tool
// arguments are returned or persisted; source file and line retain provenance.
func (s *Service) AIUsage(path string) (AIUsageReport, error) {
	projects, err := s.List()
	if err != nil {
		return AIUsageReport{}, err
	}
	if path != "" {
		project, err := s.registeredProject(path)
		if err != nil {
			return AIUsageReport{}, err
		}
		projects = []Project{project}
	}
	paths, err := currentAIPaths()
	if err != nil {
		return AIUsageReport{}, err
	}
	report := scanCodexUsage(filepath.Join(paths.codex, "sessions"), path, time.Now())
	coverage := AIUsageCoverage{Source: "repsync pipeline receipts", Status: "partial", Versions: []string{}, Detail: "Latest 20 entry records per project; internal CLIs, initiators, and business acceptance are not inferred"}
	for _, project := range projects {
		runs, err := s.PipelineRuns(project.Path)
		if err != nil {
			report.Issues = append(report.Issues, "Failed to read pipeline receipts: "+project.Path)
			continue
		}
		for _, run := range runs {
			started, timeErr := time.Parse(time.RFC3339Nano, run.StartedAt)
			since, _ := time.Parse(time.RFC3339, report.Since)
			if timeErr != nil || started.Before(since) {
				continue
			}
			coverage.Files++
			status := "unknown"
			if run.Status == "success" {
				status = "succeeded"
			} else if run.Status == "failed" {
				status = "failed"
			}
			report.Events = append(report.Events, AIUsageEvent{ID: aiID("entry", project.Path+":"+run.ID), ResourceID: "entry:pipeline:" + project.Path + ":" + run.PipelineID, Kind: "cli", Name: run.Name, Action: "pipeline", Status: status, Purpose: "entry", Actor: "repsync", Project: project.Path, Timestamp: run.StartedAt, Source: "RepSync / " + run.ID, Detail: "Pipeline entry receipt; internal commands not expanded; initiator not recorded"})
		}
	}
	report.Coverage = append(report.Coverage, coverage)
	finishAIUsage(&report)
	return report, nil
}

func scanCodexUsage(directory, scope string, now time.Time) AIUsageReport {
	since := now.AddDate(0, 0, -30)
	report := AIUsageReport{Since: since.UTC().Format(time.RFC3339), Until: now.UTC().Format(time.RFC3339), Summaries: []AIUsageSummary{}, Events: []AIUsageEvent{}, Coverage: []AIUsageCoverage{}, Issues: []string{}}
	coverage := AIUsageCoverage{Source: "Local Codex sessions", Status: "partial", Versions: []string{}, Detail: "Readable item_completed events from the last 30 days; excludes archives, cloud sessions, other AI tools, and sessions without events. Skills count only verifiable successful reads."}
	if _, err := os.Stat(directory); err != nil {
		coverage.Status = "unavailable"
		coverage.Detail = "Local Codex session directory missing or unreadable; usage unknown"
		report.Coverage = append(report.Coverage, coverage)
		return report
	}
	type file struct {
		path     string
		modified time.Time
	}
	files := []file{}
	nodes := 0
	deadline := time.Now().Add(6 * time.Second)
	err := filepath.WalkDir(directory, func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			coverage.PartialFiles++
			return nil
		}
		nodes++
		if nodes > 8000 || time.Now().After(deadline) {
			return fs.SkipAll
		}
		if entry.Type()&fs.ModeSymlink != 0 {
			return nil
		}
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".jsonl") {
			return nil
		}
		info, err := entry.Info()
		if err != nil {
			coverage.PartialFiles++
			return nil
		}
		if info.Mode().IsRegular() && !info.ModTime().Before(since) {
			files = append(files, file{path, info.ModTime()})
		}
		return nil
	})
	if err != nil {
		report.Issues = append(report.Issues, "Some Codex session directories are unreadable")
	}
	slices.SortFunc(files, func(a, b file) int { return b.modified.Compare(a.modified) })
	if len(files) > 256 {
		files = files[:256]
		report.Issues = append(report.Issues, "Scans only the 256 most recently modified session files")
	}
	if nodes > 8000 {
		report.Issues = append(report.Issues, "Session scan reached the 8000 directory entry limit")
	}
	remaining := int64(64 * 1024 * 1024)
	matched := map[string]bool{}
	for _, file := range files {
		if remaining <= 0 || time.Now().After(deadline) || len(report.Events) >= 4000 {
			report.Issues = append(report.Issues, "Usage scan reached 64 MiB, 6 seconds, or 4000 events; statistics incomplete")
			break
		}
		coverage.Files++
		events, session, version, bytes, partial := readCodexUsageFile(file.path, scope, since, now, remaining, 4000-len(report.Events), deadline)
		remaining -= bytes
		if partial {
			coverage.PartialFiles++
		}
		if session != "" {
			matched[session] = true
		}
		if version != "" && !slices.Contains(coverage.Versions, version) {
			coverage.Versions = append(coverage.Versions, version)
		}
		report.Events = append(report.Events, events...)
	}
	coverage.Sessions = len(matched)
	if coverage.PartialFiles > 0 {
		report.Issues = append(report.Issues, fmt.Sprintf("%d files incompletely read or contain unrecognized records; missing data is not counted as zero usage", coverage.PartialFiles))
	}
	report.Coverage = append(report.Coverage, coverage)
	finishAIUsage(&report)
	return report
}

func evidencePath(path string) string {
	if strings.HasPrefix(path, "file://") {
		if parsed, err := url.Parse(path); err == nil {
			path = parsed.Path
		}
	}
	if !filepath.IsAbs(path) {
		return ""
	}
	return filepath.Clean(path)
}

func withinAIScope(path, scope string) bool {
	if scope == "" {
		return true
	}
	return path == scope || strings.HasPrefix(path, scope+string(filepath.Separator))
}

func readCodexUsageFile(path, scope string, since, until time.Time, budget int64, eventLimit int, deadline time.Time) ([]AIUsageEvent, string, string, int64, bool) {
	events := []AIUsageEvent{}
	f, err := os.Open(path)
	if err != nil {
		return events, "", "", 0, true
	}
	defer f.Close()
	// Limit the file as well as the total scan, including oversized JSONL lines.
	limit := min(budget, int64(16*1024*1024))
	reader := &io.LimitedReader{R: f, N: limit}
	scanner := bufio.NewScanner(reader)
	scanner.Buffer(make([]byte, 64*1024), 2*1024*1024)
	session, cwd, version := "", "", ""
	matched, partial, sawItems := false, false, false
	line := 0
	for scanner.Scan() {
		line++
		if len(events) >= eventLimit {
			partial = true
			break
		}
		if line%100 == 0 && time.Now().After(deadline) {
			partial = true
			break
		}
		var record struct {
			Type      string          `json:"type"`
			Timestamp string          `json:"timestamp"`
			Payload   json.RawMessage `json:"payload"`
		}
		if err := json.Unmarshal(scanner.Bytes(), &record); err != nil {
			partial = true
			continue
		}
		if record.Type == "session_meta" {
			var meta struct {
				ID        string `json:"id"`
				SessionID string `json:"session_id"`
				Cwd       string `json:"cwd"`
				Version   string `json:"cli_version"`
			}
			if json.Unmarshal(record.Payload, &meta) != nil {
				partial = true
				continue
			}
			session, cwd, version = meta.ID, evidencePath(meta.Cwd), meta.Version
			if session == "" {
				session = meta.SessionID
			}
			if len(session) > 200 {
				partial = true
				session = ""
			}
			if cwd != "" && withinAIScope(cwd, scope) {
				matched = true
			}
			continue
		}
		if record.Type == "turn_context" {
			var context struct {
				Cwd string `json:"cwd"`
			}
			if json.Unmarshal(record.Payload, &context) == nil && context.Cwd != "" {
				cwd = evidencePath(context.Cwd)
			}
			if cwd != "" && withinAIScope(cwd, scope) {
				matched = true
			}
			continue
		}
		if record.Type != "event_msg" {
			continue
		}
		var payload struct {
			Type string          `json:"type"`
			Item json.RawMessage `json:"item"`
		}
		if json.Unmarshal(record.Payload, &payload) != nil || payload.Type != "item_completed" {
			continue
		}
		sawItems = true
		var item struct {
			Type     string          `json:"type"`
			ID       string          `json:"id"`
			Cwd      string          `json:"cwd"`
			Command  []string        `json:"command"`
			Status   string          `json:"status"`
			ExitCode *int            `json:"exit_code"`
			Server   string          `json:"server"`
			Tool     string          `json:"tool"`
			Result   json.RawMessage `json:"result"`
		}
		if json.Unmarshal(payload.Item, &item) != nil {
			partial = true
			continue
		}
		if item.Type != "CommandExecution" && item.Type != "McpToolCall" {
			continue
		}
		project := cwd
		if item.Cwd != "" {
			project = evidencePath(item.Cwd)
		}
		if project == "" || !withinAIScope(project, scope) {
			continue
		}
		matched = true
		stamp, err := time.Parse(time.RFC3339Nano, record.Timestamp)
		if err != nil || stamp.Before(since) || stamp.After(until) {
			continue
		}
		if session == "" || item.ID == "" {
			partial = true
			continue
		}
		if len(item.ID) > 200 || len(item.Server) > 200 || len(item.Tool) > 200 {
			partial = true
			continue
		}
		event := AIUsageEvent{ID: "codex:" + session + ":" + item.ID, Actor: "codex", Project: project, Session: session, Timestamp: stamp.UTC().Format(time.RFC3339Nano), Source: path, Line: line, Status: "unknown", Purpose: "unknown"}
		if item.Type == "McpToolCall" {
			if item.Server == "" || item.Tool == "" {
				partial = true
				continue
			}
			event.Kind, event.Name, event.ResourceID, event.Action = "mcp", item.Server, "mcp:codex:"+item.Server, item.Tool
			event.Detail = "Codex tool completion event; does not prove business acceptance"
			var result struct {
				IsError *bool `json:"isError"`
			}
			if json.Unmarshal(item.Result, &result) == nil && result.IsError != nil && item.Status == "completed" {
				if *result.IsError {
					event.Status = "failed"
				} else {
					event.Status = "succeeded"
				}
			}
			if item.Status == "failed" {
				event.Status = "failed"
			}
			if item.Status == "declined" {
				event.Status = "declined"
			}
			if item.Status == "cancelled" || item.Status == "canceled" {
				event.Status = "cancelled"
			}
			events = append(events, event)
			continue
		}
		if len(item.Command) == 0 {
			partial = true
			continue
		}
		event.Kind, event.Name, event.Action = "cli", filepath.Base(item.Command[0]), "execute"
		event.Detail = "Command execution receipt; commands inside scripts not expanded"
		commands := [][]string{item.Command}
		if len(item.Command) >= 3 && slices.Contains([]string{"sh", "bash", "zsh"}, event.Name) && strings.Contains(item.Command[1], "c") {
			commands = simpleEvidenceCommands(item.Command[2])
			if len(commands) == 1 && len(commands[0]) > 0 {
				event.Name = filepath.Base(commands[0][0])
			} else {
				event.Detail = "Compound shell command receipt; internal CLIs not individually verified"
			}
		}
		event.ResourceID = "cli:codex-name:" + event.Name
		if len(commands) == 1 && len(commands[0]) > 0 && filepath.IsAbs(commands[0][0]) {
			event.ResourceID = aiID("cli", filepath.Clean(commands[0][0]))
		} else if len(commands) != 1 && filepath.IsAbs(item.Command[0]) {
			event.ResourceID = aiID("cli", filepath.Clean(item.Command[0]))
		} else {
			event.Detail += "; executable path not recorded; not merged with current inventory sources"
		}
		if item.Status == "completed" && item.ExitCode != nil {
			if *item.ExitCode == 0 {
				event.Status = "succeeded"
			} else {
				event.Status = "failed"
			}
		}
		if item.Status == "failed" {
			event.Status = "failed"
		}
		if item.Status == "declined" {
			event.Status = "declined"
		}
		if item.Status == "cancelled" || item.Status == "canceled" {
			event.Status = "cancelled"
		}
		if len(commands) == 1 && len(commands[0]) == 2 && slices.Contains([]string{"--version", "-V", "version", "--help", "-h"}, commands[0][1]) {
			event.Purpose = "probe"
		}
		events = append(events, event)
		if event.Status != "succeeded" {
			continue
		}
		for _, command := range commands {
			if len(command) < 2 {
				continue
			}
			name, args := filepath.Base(command[0]), command[1:]
			if name == "rtk" && len(args) >= 2 && args[0] == "read" {
				name, args = "cat", args[1:]
			}
			if name != "cat" {
				continue
			}
			for _, arg := range args {
				if len(events) >= eventLimit {
					partial = true
					break
				}
				if strings.HasPrefix(arg, "-") || filepath.Base(arg) != "SKILL.md" {
					continue
				}
				location := arg
				if !filepath.IsAbs(location) {
					location = filepath.Join(project, location)
				}
				location = filepath.Clean(location)
				skill := event
				skill.ID, skill.Kind, skill.ResourceID = event.ID+":"+skillID(location), "skill", skillID(location)
				skill.Name, skill.Action, skill.Purpose = filepath.Base(filepath.Dir(location)), "read", "instruction"
				skill.Detail = "Successful read command: " + location + "; content completeness, historical version, and compliance not verified"
				events = append(events, skill)
			}
		}
	}
	if scanner.Err() != nil || reader.N <= 0 || !sawItems {
		partial = true
	}
	if !matched {
		session = ""
	}
	return events, session, version, limit - reader.N, partial
}

// simpleEvidenceCommands accepts only literal commands joined by &&. It never
// evaluates shell text, expansions, substitutions, pipelines or redirects.
func simpleEvidenceCommands(text string) [][]string {
	commands := [][]string{}
	args := []string{}
	var word strings.Builder
	quote := rune(0)
	flush := func() {
		if word.Len() > 0 {
			args = append(args, word.String())
			word.Reset()
		}
	}
	runes := []rune(text)
	for i := 0; i < len(runes); i++ {
		r := runes[i]
		if r == '$' || r == '`' || r == '\n' || r == '\r' {
			return nil
		}
		if quote != 0 {
			if r == quote {
				quote = 0
			} else if r == '\\' && quote == '"' {
				return nil
			} else {
				word.WriteRune(r)
			}
			continue
		}
		switch r {
		case '\'', '"':
			quote = r
		case ' ', '\t':
			flush()
		case '&':
			if i+1 >= len(runes) || runes[i+1] != '&' {
				return nil
			}
			flush()
			if len(args) == 0 {
				return nil
			}
			commands = append(commands, args)
			args = nil
			i++
		case ';', '|', '<', '>', '\\', '*', '?', '~', '(', ')', '{', '}', '#':
			return nil
		default:
			word.WriteRune(r)
		}
	}
	if quote != 0 {
		return nil
	}
	flush()
	if len(args) == 0 {
		return nil
	}
	return append(commands, args)
}

func finishAIUsage(report *AIUsageReport) {
	seen := map[string]int{}
	events := []AIUsageEvent{}
	for _, event := range report.Events {
		if timestamp, err := time.Parse(time.RFC3339Nano, event.Timestamp); err == nil {
			event.Timestamp = timestamp.UTC().Format("2006-01-02T15:04:05.000000000Z")
		}
		if index, exists := seen[event.ID]; !exists {
			seen[event.ID] = len(events)
			events = append(events, event)
		} else if event.Timestamp > events[index].Timestamp || event.Timestamp == events[index].Timestamp && event.Status != "unknown" || events[index].Status == "unknown" && event.Status != "unknown" {
			events[index] = event
		}
	}
	slices.SortStableFunc(events, func(a, b AIUsageEvent) int { return strings.Compare(b.Timestamp, a.Timestamp) })
	if len(events) > 4000 {
		events = events[:4000]
		report.Issues = append(report.Issues, "Only the latest 4000 evidence records are displayed and summarized")
	}
	// Leave room for summaries and coverage within the control client's 4 MiB limit.
	budget := 3 * 1024 * 1024
	resourceSizes := map[string]bool{}
	for i, event := range events {
		encoded, err := json.Marshal(event)
		size := len(encoded) + 1
		if !resourceSizes[event.ResourceID] {
			summary, summaryErr := json.Marshal(AIUsageSummary{ResourceID: event.ResourceID, Kind: event.Kind, Name: event.Name, LastUsed: event.Timestamp})
			if summaryErr != nil {
				err = summaryErr
			}
			size += len(summary) + 128
			resourceSizes[event.ResourceID] = true
		}
		if err != nil || size > budget {
			events = events[:i]
			report.Issues = append(report.Issues, "Usage evidence reached the 3 MiB response limit; statistics incomplete")
			break
		}
		budget -= size
	}
	report.Events = events
	report.Summaries = []AIUsageSummary{}
	indices := map[string]int{}
	sessions := map[string]map[string]bool{}
	for _, event := range events {
		index, ok := indices[event.ResourceID]
		if !ok {
			index = len(report.Summaries)
			indices[event.ResourceID] = index
			report.Summaries = append(report.Summaries, AIUsageSummary{ResourceID: event.ResourceID, Kind: event.Kind, Name: event.Name, LastUsed: event.Timestamp})
			sessions[event.ResourceID] = map[string]bool{}
		}
		summary := &report.Summaries[index]
		if event.Session != "" {
			sessions[event.ResourceID][event.Session] = true
		}
		summary.Sessions = len(sessions[event.ResourceID])
		if event.Actor == "repsync" {
			summary.AppRuns++
			continue
		}
		if event.Status == "declined" {
			summary.Declined++
			continue
		}
		summary.Calls++
		if event.Purpose == "probe" {
			summary.Probes++
		}
		switch event.Status {
		case "succeeded":
			summary.Succeeded++
		case "failed":
			summary.Failed++
		case "cancelled":
			summary.Cancelled++
		default:
			summary.Unknown++
		}
	}
	// Stable ordering makes repeated scans comparable without changing counts.
	slices.SortStableFunc(report.Summaries, func(a, b AIUsageSummary) int { return strings.Compare(a.Kind+":"+a.Name, b.Kind+":"+b.Name) })
}
