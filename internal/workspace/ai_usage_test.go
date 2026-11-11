//go:build integration

package workspace

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"
)

func TestAIUsageCodexEvidenceScopeDeduplicationAndPrivacy(t *testing.T) {
	t.Parallel()
	directory, project := t.TempDir(), t.TempDir()
	now := time.Now().UTC()
	stamp := now.Add(-time.Hour).Format(time.RFC3339Nano)
	var lines []string
	add := func(kind string, payload any) {
		data, err := json.Marshal(map[string]any{"type": kind, "timestamp": stamp, "payload": payload})
		if err != nil {
			t.Fatal(err)
		}
		lines = append(lines, string(data))
	}
	add("session_meta", map[string]any{"id": "session-1", "cwd": project, "cli_version": "fixture-version", "base_instructions": "SECRET_CHAT_BODY"})
	command := func(id, text string, exit int) {
		add("event_msg", map[string]any{"type": "item_completed", "item": map[string]any{"type": "CommandExecution", "id": id, "command": []string{"/bin/sh", "-c", text}, "cwd": "file://" + project, "status": "completed", "exit_code": exit, "aggregated_output": "SECRET_COMMAND_OUTPUT"}})
	}
	skill := filepath.Join(project, ".agents/skills/writing/SKILL.md")
	command("read-1", "cat '"+skill+"'", 0)
	command("read-1", "cat '"+skill+"'", 0)
	command("read-2", "cat '"+skill+"'", 0)
	command("read-failed", "cat '"+skill+"'", 1)
	command("not-read", "echo 'cat "+skill+"'", 0)
	command("probe", "gh --version", 0)
	mcp := func(id, status string, isError bool) {
		add("event_msg", map[string]any{"type": "item_completed", "item": map[string]any{"type": "McpToolCall", "id": id, "server": "example", "tool": "read", "status": status, "arguments": "SECRET_TOKEN", "result": map[string]any{"isError": isError, "content": "SECRET_MCP_BODY"}}})
	}
	mcp("mcp-good", "completed", false)
	mcp("mcp-bad", "completed", true)
	mcp("mcp-declined", "declined", false)
	mcp("mcp-cancelled", "cancelled", false)
	add("turn_context", map[string]any{"cwd": project + "-different"})
	mcp("outside", "completed", false)
	path := filepath.Join(directory, "rollout.jsonl")
	aiFixture(t, path, strings.Join(lines, "\n")+"\n")
	// Importing the same evidence twice must not inflate calls or sessions.
	aiFixture(t, filepath.Join(directory, "duplicate.jsonl"), strings.Join(lines, "\n")+"\n")
	report := scanCodexUsage(directory, project, now)
	find := func(id string) AIUsageSummary {
		for _, summary := range report.Summaries {
			if summary.ResourceID == id {
				return summary
			}
		}
		t.Fatalf("summary missing %s: %+v", id, report.Summaries)
		return AIUsageSummary{}
	}
	read := find(skillID(skill))
	if read.Sessions != 1 || read.Calls != 2 {
		t.Fatalf("skill reads were inflated or failed reads counted: %+v", read)
	}
	tool := find("mcp:codex:example")
	if tool.Calls != 3 || tool.Succeeded != 1 || tool.Failed != 1 || tool.Declined != 1 || tool.Cancelled != 1 {
		t.Fatalf("MCP states conflated: %+v", tool)
	}
	probe := find("cli:codex-name:gh")
	if probe.Probes != 1 {
		t.Fatalf("probe hidden: %+v", probe)
	}
	data, err := json.Marshal(report)
	if err != nil {
		t.Fatal(err)
	}
	for _, secret := range []string{"SECRET_CHAT_BODY", "SECRET_COMMAND_OUTPUT", "SECRET_TOKEN", "SECRET_MCP_BODY", "outside"} {
		if strings.Contains(string(data), secret) {
			t.Fatalf("unrelated or private content leaked: %s", secret)
		}
	}
	for _, event := range report.Events {
		if event.Source == "" || event.Line < 1 || event.Session != "session-1" {
			t.Fatalf("provenance missing: %+v", event)
		}
	}
}

func TestAIUsageMissingMalformedAndLegacyRecordsStayUnknown(t *testing.T) {
	t.Parallel()
	missing := scanCodexUsage(filepath.Join(t.TempDir(), "missing"), "", time.Now())
	if missing.Coverage[0].Status != "unavailable" || len(missing.Events) != 0 {
		t.Fatalf("missing source: %+v", missing)
	}
	directory := t.TempDir()
	aiFixture(t, filepath.Join(directory, "legacy.jsonl"), `{"type":"session_meta","payload":{"id":"legacy","cwd":"/project"}}`+"\n"+`{"type":"response_item","payload":{"type":"message","content":"no execution evidence"}}`+"\n{broken\n")
	report := scanCodexUsage(directory, "/project", time.Now())
	if report.Coverage[0].PartialFiles != 1 || len(report.Issues) == 0 || len(report.Events) != 0 {
		t.Fatalf("unsupported records reported as complete: %+v", report)
	}
}

func TestAIUsageLiteralShellEvidenceRejectsGuesses(t *testing.T) {
	t.Parallel()
	for _, text := range []string{"cat /a/SKILL.md | head", "cat $(echo /a/SKILL.md)", "cat /a/SKILL.md; true", "cat /a/*.md", "cat /a/SKILL.md || true", "cat /a/SKILL.md > /tmp/result"} {
		if simpleEvidenceCommands(text) != nil {
			t.Errorf("unsafe inference from %q", text)
		}
	}
	if commands := simpleEvidenceCommands("cat '/a directory/SKILL.md' && gh --version"); len(commands) != 2 || !slices.Equal(commands[0], []string{"cat", "/a directory/SKILL.md"}) {
		t.Fatalf("literal read evidence lost: %+v", commands)
	}
}

func TestAIUsageBoundsOversizedRecords(t *testing.T) {
	t.Parallel()
	directory := t.TempDir()
	aiFixture(t, filepath.Join(directory, "oversized.jsonl"), strings.Repeat("x", 2*1024*1024+1))
	report := scanCodexUsage(directory, "", time.Now())
	if report.Coverage[0].PartialFiles != 1 || len(report.Events) != 0 {
		t.Fatal("oversized JSONL was not bounded")
	}
	if entries, err := os.ReadDir(directory); err != nil || len(entries) != 1 {
		t.Fatal("scan changed input directory")
	}
}

func TestAIUsageUpdatesCompletionAndKeepsExecutableSourcesSeparate(t *testing.T) {
	t.Parallel()
	report := AIUsageReport{Events: []AIUsageEvent{
		{ID: "same", ResourceID: "mcp:codex:test", Kind: "mcp", Name: "test", Status: "unknown", Session: "one", Timestamp: "2026-11-10T01:00:00Z"},
		{ID: "same", ResourceID: "mcp:codex:test", Kind: "mcp", Name: "test", Status: "succeeded", Session: "one", Timestamp: "2026-11-10T01:01:00Z"},
		{ID: "first", ResourceID: aiID("cli", "/first/gh"), Kind: "cli", Name: "gh", Status: "succeeded", Timestamp: "2026-11-10T01:00:00Z"},
		{ID: "second", ResourceID: aiID("cli", "/second/gh"), Kind: "cli", Name: "gh", Status: "succeeded", Timestamp: "2026-11-10T01:00:00.1Z"},
	}}
	finishAIUsage(&report)
	if len(report.Events) != 3 || len(report.Summaries) != 3 {
		t.Fatalf("different sources conflated: %+v", report.Summaries)
	}
	if report.Events[0].ID != "same" || report.Events[1].ID != "second" {
		t.Fatal("timestamps with different fractional precision sorted incorrectly")
	}
	for _, summary := range report.Summaries {
		if summary.Kind == "mcp" && (summary.Calls != 1 || summary.Succeeded != 1 || summary.Unknown != 0) {
			t.Fatalf("completion update lost: %+v", summary)
		}
	}
}

func TestAIUsageFitsControlResponseBudget(t *testing.T) {
	t.Parallel()
	report := AIUsageReport{Events: []AIUsageEvent{}, Issues: []string{}, Coverage: []AIUsageCoverage{}}
	for i := range 4000 {
		report.Events = append(report.Events, AIUsageEvent{ID: fmt.Sprintf("event-%d", i), ResourceID: "cli:fixture", Kind: "cli", Name: "fixture", Status: "succeeded", Source: "/source/" + strings.Repeat("long-path/", 200), Timestamp: "2026-11-10T01:00:00Z"})
	}
	finishAIUsage(&report)
	encoded, err := json.Marshal(report)
	if err != nil {
		t.Fatal(err)
	}
	if len(encoded) >= 4*1024*1024 || len(report.Events) == 0 || len(report.Issues) == 0 {
		t.Fatalf("response budget not preserved: %d bytes, %d events, %v", len(encoded), len(report.Events), report.Issues)
	}
}
