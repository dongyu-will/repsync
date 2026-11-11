//go:build integration

package workspace

import (
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"
)

func TestCodexMemoryExtractionPreservesAuthoredEvidence(t *testing.T) {
	root := t.TempDir()
	service := NewService(filepath.Join(root, "state", "projects.json"))
	project, err := service.Add(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	other, err := service.Add(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	const stamp = "2026-11-10T08:00:00Z"
	const original = "生产环境已经切到 PostgreSQL。旧 MySQL 启动指南可以删掉。"
	var log strings.Builder
	appendLog := func(kind string, payload any) {
		t.Helper()
		data, err := json.Marshal(map[string]any{"type": kind, "timestamp": stamp, "payload": payload})
		if err != nil {
			t.Fatal(err)
		}
		log.Write(data)
		log.WriteByte('\n')
	}
	event := func(kind, turn string) {
		appendLog("event_msg", map[string]string{"type": kind, "turn_id": turn})
	}
	start := func(turn, cwd string) {
		event("task_started", turn)
		appendLog("turn_context", map[string]string{"turn_id": turn, "cwd": cwd})
	}
	message := func(turn, role, kind, text string) {
		appendLog("response_item", map[string]any{
			"type": "message", "role": role, "content": []map[string]string{{"type": "input_text", "text": text}},
			"internal_chat_message_metadata_passthrough": map[string]any{
				"turn_id": turn, "content_item_kinds": []string{kind},
			},
		})
	}
	appendLog("session_meta", map[string]string{"id": "thread-1", "cwd": project.Path, "timestamp": stamp, "source": "vscode"})
	start("one", project.Path)
	message("one", "user", "agents_md.instructions", "Injected project rules")
	message("one", "user", "user.text", original)
	message("one", "assistant", "user.text", "Fabricated completed deployment")
	event("task_complete", "one")
	start("legacy", project.Path)
	appendLog("event_msg", map[string]string{"type": "user_message", "message": "保留真实的旧版用户消息。"})
	event("task_complete", "legacy")
	start("elsewhere", other.Path)
	message("elsewhere", "user", "user.text", "Other project")
	event("task_complete", "elsewhere")
	start("unfinished", project.Path)
	message("unfinished", "user", "user.text", "Unfinished turn")
	file := "2026/11/10/session.jsonl"
	aiFixture(t, filepath.Join(root, file), log.String())
	session, messages, partial, err := readCodexMemoryMessages(root, project.Path, file, "thread-1")
	if err != nil || partial || len(messages) != 2 || messages[0].Text != original ||
		messages[0].Line != 5 || messages[1].Text != "保留真实的旧版用户消息。" {
		t.Fatalf("unexpected messages: %+v %v %v", messages, partial, err)
	}
	if _, _, _, err := readCodexMemoryMessages(root, other.Path, file, "thread-1"); err == nil {
		t.Fatal("accepted a session belonging to another project")
	}
	proposals := []codexMemoryProposal{
		{Line: 5, Quote: "生产环境已经切到 PostgreSQL。", Statement: "生产环境已切换到 PostgreSQL", Environment: "生产", Keywords: []string{"MySQL"}},
		{Line: 4, Quote: "Injected project rules", Statement: "Injected project rules"},
		{Line: 5, Quote: "生产已完成 PostgreSQL 切换", Statement: "Paraphrased quote"},
	}
	extraction := CodexMemoryExtraction{Added: []ProjectMemory{}}
	sourceFile := filepath.Join(root, file)
	got, err := service.saveCodexMemoryProposals(project.Path, session, sourceFile, messages, proposals, extraction)
	if err != nil || len(got.Added) != 1 || got.Rejected != 2 {
		t.Fatalf("extraction: %+v %v", got, err)
	}
	candidate := got.Added[0]
	if candidate.Status != "pending" || candidate.Origin != "ai" || candidate.Session == nil ||
		candidate.Session.SessionID != "thread-1" || candidate.Session.Quote != proposals[0].Quote ||
		candidate.Session.Line != 5 || candidate.Session.File != sourceFile || candidate.Session.Timestamp != stamp {
		t.Fatalf("source or status lost: %+v", candidate)
	}
	if _, err := service.SetProjectMemoryState(project.Path, candidate.ID, candidate.Version, "archive"); err != nil {
		t.Fatal(err)
	}
	got, err = service.saveCodexMemoryProposals(project.Path, session, sourceFile, messages, proposals, extraction)
	if err != nil || len(got.Added) != 0 || got.Existing != 1 {
		t.Fatalf("repeated extraction resurrected a record: %+v %v", got, err)
	}
}
