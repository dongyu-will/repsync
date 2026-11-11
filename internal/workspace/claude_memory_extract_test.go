//go:build integration

package workspace

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestClaudeMemorySessionsAndExtraction(t *testing.T) {
	root := t.TempDir()
	service := NewService(filepath.Join(root, "state", "projects.json"))
	project, err := service.Add(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	other := t.TempDir()
	config := filepath.Join(root, "claude")
	t.Setenv("CLAUDE_CONFIG_DIR", config)
	const original = "共享规范由项目覆盖，数据库使用 PostgreSQL。"
	const id = "shared-id"
	var log strings.Builder
	parent := ""
	appendEntry := func(kind, cwd, uuid string, content any, stop string, flags map[string]any) {
		t.Helper()
		entry := map[string]any{"type": kind, "cwd": cwd, "uuid": uuid, "sessionId": id,
			"parentUuid": parent, "timestamp": "2026-11-11T08:00:00Z", "message": map[string]any{"role": kind, "content": content, "stop_reason": stop}}
		parent = uuid
		for key, value := range flags {
			entry[key] = value
		}
		data, err := json.Marshal(entry)
		if err != nil {
			t.Fatal(err)
		}
		log.Write(data)
		log.WriteByte('\n')
	}
	appendEntry("user", project.Path, "one", original, "", nil)
	appendEntry("user", project.Path, "tool", []map[string]string{{"type": "tool_result", "content": "untrusted output"}, {"type": "text", "text": "tool annotation"}}, "", nil)
	appendEntry("user", project.Path, "meta", "injected context", "", map[string]any{"isMeta": true})
	appendEntry("assistant", project.Path, "reply", "not user evidence", "end_turn", nil)
	appendEntry("user", other, "elsewhere", "other project", "", nil)
	appendEntry("assistant", other, "reply2", "done", "end_turn", nil)
	appendEntry("user", project.Path, "interrupted", "interrupted request", "", nil)
	appendEntry("user", project.Path, "interrupt", "[Request interrupted by user]", "", nil)
	appendEntry("assistant", project.Path, "reply3", "done", "end_turn", nil)
	appendEntry("user", project.Path, "summary", "generated summary", "", map[string]any{"isCompactSummary": true})
	appendEntry("assistant", project.Path, "reply4", "done", "end_turn", nil)
	appendEntry("user", project.Path, "unfinished", "unfinished request", "", nil)
	appendEntry("assistant", project.Path, "orphan", "unrelated branch reply", "end_turn", map[string]any{"parentUuid": "another-branch"})
	file := "project/session.jsonl"
	aiFixture(t, filepath.Join(config, "projects", file), log.String())
	aiFixture(t, filepath.Join(config, "projects", "project/session/subagents/agent.jsonl"), log.String())
	sessions, err := service.ListMemorySessions(project.Path, "claude")
	if err != nil || sessions.Partial || len(sessions.Sessions) != 1 || sessions.Sessions[0].Provider != "claude" {
		t.Fatalf("session listing: %+v %v", sessions, err)
	}
	_, messages, partial, err := readClaudeMemoryMessages(filepath.Join(config, "projects"), project.Path, file, id)
	if err != nil || partial || len(messages) != 1 || messages[0].Line != 1 || messages[0].Text != original {
		t.Fatalf("authored messages: %+v %v %v", messages, partial, err)
	}
	if _, _, _, err := readClaudeMemoryMessages(filepath.Join(config, "projects"), project.Path, file, "changed-id"); err == nil {
		t.Fatal("accepted a changed session identity")
	}
	if _, _, _, err := readClaudeMemoryMessages(filepath.Join(config, "projects"), other, file, id); err == nil {
		t.Fatal("accepted a mismatched project session")
	}
	bin := filepath.Join(root, "bin")
	if err := os.Mkdir(bin, 0700); err != nil {
		t.Fatal(err)
	}
	proposal := `{"items":[{"line":1,"quote":"共享规范由项目覆盖","statement":"共享规范由项目覆盖","environment":"","keywords":[]}]}`
	claudeCLI := "#!/bin/sh\ncat >/dev/null\n" + `printf '%s' '{"subtype":"success","is_error":false,"structured_output":` + proposal + `}'` + "\n"
	codexCLI := "#!/bin/sh\nwhile [ \"$#\" -gt 0 ]; do\n  if [ \"$1\" = --output-last-message ]; then shift; output=\"$1\"; fi\n  shift\ndone\ncat >/dev/null\nprintf '%s' '" + proposal + "' > \"$output\"\n"
	for binary, script := range map[string]string{"claude": claudeCLI, "codex": codexCLI} {
		if err := os.WriteFile(filepath.Join(bin, binary), []byte(script), 0700); err != nil {
			t.Fatal(err)
		}
	}
	t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))
	result, err := service.ExtractProjectMemories(project.Path, "claude", file, id, "claude")
	if err != nil || result.Messages != 1 || len(result.Added) != 1 || result.Added[0].Status != "pending" ||
		result.Added[0].Session.Provider != "claude" || result.Added[0].Session.Extractor != "claude" ||
		!strings.HasPrefix(result.Added[0].Source, "Claude Code session") {
		t.Fatalf("Claude extraction: %+v %v", result, err)
	}
	result, err = service.ExtractProjectMemories(project.Path, "claude", file, id, "codex")
	if err != nil || result.Existing != 1 || len(result.Added) != 0 {
		t.Fatalf("cross-tool repeated extraction: %+v %v", result, err)
	}
	// The same source id and line from a different tool must remain distinct.
	result, err = service.saveMemoryProposals(project.Path, MemorySession{ID: id, Provider: "codex"}, "codex.jsonl", "claude",
		messages, []memoryProposal{{Line: 1, Quote: "共享规范由项目覆盖", Statement: "共享规范由项目覆盖"}}, MemoryExtraction{})
	if err != nil || len(result.Added) != 1 {
		t.Fatalf("cross-source identity collision: %+v %v", result, err)
	}
	if err := os.WriteFile(filepath.Join(bin, "claude"), []byte("#!/bin/sh\nprintf '%s' '{\"subtype\":\"success\",\"is_error\":true,\"structured_output\":{\"items\":[]}}'\n"), 0700); err != nil {
		t.Fatal(err)
	}
	if _, err := runMemoryExtraction("claude", messages); err == nil {
		t.Fatal("accepted an unsuccessful CLI result")
	}
	if _, err := service.ListMemorySessions(project.Path, "unknown"); err == nil {
		t.Fatal("accepted an unsupported source")
	}
	if err := os.Symlink(filepath.Join(config, "projects", file), filepath.Join(config, "projects/project/link.jsonl")); err != nil {
		t.Fatal(err)
	}
	if _, _, _, err := readClaudeMemoryMessages(filepath.Join(config, "projects"), project.Path, "project/link.jsonl", id); err == nil {
		t.Fatal("accepted a symlink transcript")
	}
}
