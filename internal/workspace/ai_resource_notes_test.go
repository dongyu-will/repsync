//go:build integration

package workspace

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestAIResourceNotesLifecycle(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("CODEX_HOME", filepath.Join(home, ".codex"))
	bin := filepath.Join(home, "bin")
	aiFixture(t, filepath.Join(bin, "codex"), "#!/bin/sh\nprintf '[]'\n")
	if err := os.Chmod(filepath.Join(bin, "codex"), 0700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))
	service := NewService(filepath.Join(t.TempDir(), "projects.json"))
	project, err := service.Add(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	source := filepath.Join(home, ".agents/skills/writer/SKILL.md")
	aiFixture(t, source, "---\nname: writer\ndescription: Write docs\n---\nOriginal instructions")
	other := filepath.Join(home, ".codex/skills/another-writer/SKILL.md")
	aiFixture(t, other, "---\nname: writer\n---\nOther source")
	find := func(scope, location string) AIResource {
		t.Helper()
		location, err = filepath.EvalSymlinks(location)
		if err != nil {
			t.Fatal(err)
		}
		report, err := service.AIResources(scope)
		if err != nil {
			t.Fatal(err)
		}
		for _, resource := range report.Resources {
			if resource.Location == location {
				return resource
			}
		}
		t.Fatalf("resource missing: %s", location)
		return AIResource{}
	}
	resource := find("", source)
	input := AIResourceNoteInput{AIResourceNoteData: AIResourceNoteData{
		ResourceID: resource.ID, ResourceRevision: resource.Revision,
		InstalledVersion: "1", LatestVersion: "2", UpdateStatus: "available",
		CheckedAt: time.Now().UTC().Format(time.RFC3339), Summary: "新版增加文档审查",
		Changes: "新增审查；当前安装版还不支持。", Manual: "对 AI 说：整理当前文档。",
		Sources: []string{"https://example.invalid/releases/2", source},
	}}
	for _, test := range []struct {
		name   string
		change func(*AIResourceNoteInput)
	}{
		{name: "missing version evidence", change: func(in *AIResourceNoteInput) { in.LatestVersion = "" }},
		{name: "invalid status", change: func(in *AIResourceNoteInput) { in.UpdateStatus = "success" }},
		{name: "future check", change: func(in *AIResourceNoteInput) {
			in.CheckedAt = time.Now().Add(time.Hour).Format(time.RFC3339)
		}},
		{name: "oversized manual", change: func(in *AIResourceNoteInput) { in.Manual = strings.Repeat("x", 64*1024) }},
	} {
		t.Run(test.name, func(t *testing.T) {
			invalid := input
			test.change(&invalid)
			if _, err := service.RecordAIResourceNote("", invalid); err == nil {
				t.Fatal("invalid update report accepted")
			}
		})
	}
	note, err := service.RecordAIResourceNote("", input)
	if err != nil || note.Version == "" {
		t.Fatalf("record: %+v %v", note, err)
	}
	if _, err := service.RecordAIResourceNote("", input); err == nil {
		t.Fatal("concurrent report replacement was accepted")
	}
	if got := find(project.Path, source); got.Note == nil || got.Note.Stale || got.Note.Manual != input.Manual {
		t.Fatalf("project did not inherit global manual: %+v", got)
	}
	if got := find("", other); got.Note != nil {
		t.Fatal("same-name source inherited unrelated report")
	}
	input.ExpectedVersion = note.Version
	input.Summary = "项目专属说明"
	if _, err := service.RecordAIResourceNote(project.Path, input); err != nil {
		t.Fatal("cannot override inherited global note:", err)
	}
	if got := find("", source); got.Note == nil || got.Note.Summary == input.Summary {
		t.Fatal("project report overwrote global report")
	}
	aiFixture(t, source, "---\nname: writer\n---\nNew instructions")
	if got := find(project.Path, source); got.Note == nil || !got.Note.Stale {
		t.Fatalf("changed skill still had current manual: %+v", got)
	}
	if _, err := service.RecordAIResourceNote("", input); err == nil ||
		!strings.Contains(err.Error(), "resource changed") {
		t.Fatalf("stale resource snapshot accepted: %v", err)
	}
	before, err := os.ReadFile(source)
	if err != nil {
		t.Fatal(err)
	}
	input.ResourceRevision = find("", source).Revision
	input.UpdateStatus, input.Summary = "failed", "上游读取失败；没有执行升级"
	if _, err := service.RecordAIResourceNote("", input); err != nil {
		t.Fatal(err)
	}
	after, err := os.ReadFile(source)
	if err != nil || string(before) != string(after) {
		t.Fatal("recording findings changed the installed resource")
	}
	// A new service instance reads the same persisted failure, not a success fallback.
	restarted := NewService(service.path)
	report, err := restarted.AIResources("")
	if err != nil {
		t.Fatal(err)
	}
	for _, got := range report.Resources {
		if got.ID == resource.ID && (got.Note == nil || got.Note.UpdateStatus != "failed" || got.Note.Stale) {
			t.Fatalf("failure state did not survive restart: %+v", got)
		}
	}
}

func TestAIResourceNotesMCPConfigurationRevision(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("CODEX_HOME", filepath.Join(home, ".codex"))
	bin := filepath.Join(home, "bin")
	command := filepath.Join(bin, "codex")
	aiFixture(t, command, "#!/bin/sh\ncat \"$HOME/mcp-list.json\"\n")
	if err := os.Chmod(command, 0700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))
	service := NewService(filepath.Join(t.TempDir(), "projects.json"))
	read := func(configuration string) AIResource {
		t.Helper()
		aiFixture(t, filepath.Join(home, "mcp-list.json"), configuration)
		report, err := service.AIResources("")
		if err != nil {
			t.Fatal(err)
		}
		encoded, err := json.Marshal(report)
		leaked := strings.Contains(string(encoded), "private-token") || strings.Contains(string(encoded), "example.invalid")
		if err != nil || leaked {
			t.Fatal("MCP snapshot leaked connection details")
		}
		for _, resource := range report.Resources {
			if resource.Kind == "mcp" {
				return resource
			}
		}
		t.Fatalf("MCP missing: %+v", report.Issues)
		return AIResource{}
	}
	first := read(`[{"name":"docs","enabled":true,"transport":{
		"type":"http","url":"https://example.invalid/one","token":"private-token"}}]`)
	reordered := read(`[{"enabled":true,"name":"docs","transport":{
		"token":"private-token","url":"https://example.invalid/one","type":"http"}}]`)
	changed := read(`[{"name":"docs","enabled":true,"transport":{
		"type":"http","url":"https://example.invalid/two","token":"private-token"}}]`)
	if first.Revision == "" || first.Revision != reordered.Revision || first.Revision == changed.Revision {
		t.Fatal("MCP revision did not track configuration identity")
	}
}

func TestAIResourceNotesMCPScopesAndCorruption(t *testing.T) {
	service := NewService(filepath.Join(t.TempDir(), "projects.json"))
	note := AIResourceNote{AIResourceNoteData: AIResourceNoteData{
		ResourceID: "mcp:codex:docs", ResourceRevision: "global-endpoint",
		UpdateStatus: "unknown", Summary: "远程版本未知", Manual: "读取官方文档",
		CheckedAt: time.Now().UTC().Format(time.RFC3339), Sources: []string{},
	}}
	if err := saveJSON(service.aiResourceNotePath("", note.ResourceID), note); err != nil {
		t.Fatal(err)
	}
	resource := AIResource{ID: note.ResourceID, Kind: "mcp", Revision: "project-endpoint"}
	report := AIResourceReport{Scope: "/project", Resources: []AIResource{resource}}
	service.attachAIResourceNotes(&report)
	if report.Resources[0].Note != nil {
		t.Fatal("different project MCP inherited global server manual")
	}
	report.Resources[0].Revision = note.ResourceRevision
	service.attachAIResourceNotes(&report)
	if report.Resources[0].Note == nil {
		t.Fatal("matching MCP did not inherit global report")
	}
	aiFixture(t, service.aiResourceNotePath("/project", resource.ID), "invalid JSON")
	service.attachAIResourceNotes(&report)
	if len(report.Issues) == 0 {
		t.Fatal("corrupt record was silently hidden")
	}
}
