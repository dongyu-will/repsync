//go:build integration

package workspace_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"repsync/internal/workspace"
)

type syncFixture struct {
	service                 *workspace.Service
	config, source, project string
}

func writeSyncFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(content), 0600); err != nil {
		t.Fatal(err)
	}
}

func readSyncFile(t *testing.T, path string) string {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
}

func newSyncFixture(t *testing.T, original string, exists bool) syncFixture {
	t.Helper()
	root := t.TempDir()
	fixture := syncFixture{config: filepath.Join(root, "config", "projects.json"), source: filepath.Join(root, "common.md"), project: filepath.Join(root, "project with spaces")}
	if err := os.Mkdir(fixture.project, 0700); err != nil {
		t.Fatal(err)
	}
	fixture.service = workspace.NewService(fixture.config)
	project, err := fixture.service.Add(fixture.project)
	if err != nil {
		t.Fatal(err)
	}
	fixture.project = project.Path
	if exists {
		writeSyncFile(t, filepath.Join(fixture.project, "AGENTS.md"), original)
	}
	writeSyncFile(t, fixture.source, "# Shared rules\nUse the existing project checks.\n")
	source, err := fixture.service.ConnectSharedSource(fixture.source)
	if err != nil {
		t.Fatal(err)
	}
	fixture.source = source.Path
	return fixture
}

func (f syncFixture) preview(t *testing.T) workspace.UpdatePreview {
	t.Helper()
	preview, err := f.service.PreviewUpdate(f.project)
	if err != nil {
		t.Fatal(err)
	}
	return preview
}

func (f syncFixture) apply(t *testing.T) workspace.UpdateHistory {
	t.Helper()
	record, err := f.service.ApplyUpdate(f.preview(t).Token)
	if err != nil {
		t.Fatal(err)
	}
	return record
}

func TestRuleUpdatePreservesProjectContentAndRestores(t *testing.T) {
	original := "# Project overrides\r\nKeep custom release rules.\r\n"
	f := newSyncFixture(t, original, true)
	agents := filepath.Join(f.project, "AGENTS.md")
	product := filepath.Join(f.project, "PRODUCT.md")
	writeSyncFile(t, product, "Keep project-specific product requirements.")
	if err := os.Chmod(agents, 0640); err != nil {
		t.Fatal(err)
	}
	preview := f.preview(t)
	if !preview.Changed || !strings.Contains(preview.Diff, "+<!-- repsync:shared:start -->") || strings.Contains(preview.Diff, "repsync-diff-") {
		t.Fatalf("incorrect diff: %s", preview.Diff)
	}
	if got := readSyncFile(t, agents); got != original {
		t.Fatal("preview modified project")
	}
	first, err := f.service.ApplyUpdate(preview.Token)
	if err != nil {
		t.Fatal(err)
	}
	firstContent := readSyncFile(t, agents)
	if !strings.HasSuffix(firstContent, original) || strings.Count(firstContent, "repsync:shared:start") != 1 {
		t.Fatal("project overrides changed")
	}
	info, err := os.Stat(agents)
	if err != nil || info.Mode().Perm() != 0640 {
		t.Fatal("permissions changed")
	}
	if got := readSyncFile(t, product); got != "Keep project-specific product requirements." {
		t.Fatal("product document changed")
	}
	if _, err := f.service.ApplyUpdate(preview.Token); err == nil {
		t.Fatal("replayed plan accepted")
	}
	if unchanged := f.preview(t); unchanged.Changed || unchanged.Diff != "" {
		t.Fatal("identical source produced changes")
	}
	// An edited managed block remains reviewable, while all project-owned text stays untouched.
	writeSyncFile(t, f.source, "# Shared rules v2\nPreserve project-specific checks.\n")
	second := f.apply(t)
	secondContent := readSyncFile(t, agents)
	if !strings.HasSuffix(secondContent, original) || strings.Count(secondContent, "repsync:shared:start") != 1 || strings.Contains(secondContent, "Use the existing") {
		t.Fatal("managed block was not replaced")
	}
	reloaded := workspace.NewService(f.config)
	source, err := reloaded.SharedSource()
	if err != nil || source.Path != f.source || !strings.Contains(source.Content, "v2") {
		t.Fatalf("source persistence: %+v %v", source, err)
	}
	history, err := reloaded.UpdateHistory(f.project)
	if err != nil || len(history) != 2 || history[0].ID != second.ID || history[0].State != "applied" || history[1].State != "changed" {
		t.Fatalf("history: %+v %v", history, err)
	}
	if _, err := reloaded.RestoreUpdate(second.ID); err != nil {
		t.Fatal(err)
	}
	if got := readSyncFile(t, agents); got != firstContent {
		t.Fatal("second restore did not restore exact original bytes")
	}
	if _, err := reloaded.RestoreUpdate(first.ID); err != nil {
		t.Fatal(err)
	}
	if got := readSyncFile(t, agents); got != original {
		t.Fatal("first restore did not restore original")
	}
}

func TestRuleUpdateRejectsStaleInputs(t *testing.T) {
	for _, change := range []string{"source", "target", "permissions", "source selection"} {
		t.Run(change, func(t *testing.T) {
			f := newSyncFixture(t, "# Local\n", true)
			preview := f.preview(t)
			agents := filepath.Join(f.project, "AGENTS.md")
			switch change {
			case "source":
				writeSyncFile(t, f.source, "Changed public rules\n")
			case "target":
				writeSyncFile(t, agents, "New local changes\n")
			case "permissions":
				if err := os.Chmod(agents, 0644); err != nil {
					t.Fatal(err)
				}
			case "source selection":
				if _, err := f.service.ConnectSharedSource(f.source); err != nil {
					t.Fatal(err)
				}
			}
			before := readSyncFile(t, agents)
			if _, err := f.service.ApplyUpdate(preview.Token); err == nil {
				t.Fatal("stale preview applied")
			}
			if got := readSyncFile(t, agents); got != before {
				t.Fatal("existing changes overwritten")
			}
		})
	}
}

func TestRuleUpdateCreationAndRestoreProtection(t *testing.T) {
	f := newSyncFixture(t, "", false)
	agents := filepath.Join(f.project, "AGENTS.md")
	stale := f.preview(t)
	writeSyncFile(t, agents, "Created after preview\n")
	if _, err := f.service.ApplyUpdate(stale.Token); err == nil {
		t.Fatal("new file overwritten")
	}
	if err := os.Remove(agents); err != nil {
		t.Fatal(err)
	}
	record := f.apply(t)
	applied := readSyncFile(t, agents)
	writeSyncFile(t, agents, applied+"Later local work\n")
	if _, err := f.service.RestoreUpdate(record.ID); err == nil {
		t.Fatal("later local work discarded")
	}
	history, err := f.service.UpdateHistory(f.project)
	if err != nil || len(history) != 1 || history[0].State != "changed" {
		t.Fatalf("changed history: %+v %v", history, err)
	}
	writeSyncFile(t, agents, applied)
	if _, err := f.service.RestoreUpdate(record.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(agents); !os.IsNotExist(err) {
		t.Fatal("new file was not removed on restore")
	}
}

func TestRuleUpdateRejectsInvalidTargetsAndSources(t *testing.T) {
	for _, content := range []string{
		"<!-- repsync:shared:start -->\nBroken", "<!-- repsync:shared:end -->\n<!-- repsync:shared:start -->",
		"<!-- repsync:shared:start --><!-- repsync:shared:end --><!-- repsync:shared:end -->",
		strings.Repeat("x", 256*1024), string([]byte{0xff}),
	} {
		f := newSyncFixture(t, content, true)
		if _, err := f.service.PreviewUpdate(f.project); err == nil {
			t.Fatal("unsafe target accepted")
		}
	}
	f := newSyncFixture(t, "# Local", true)
	if _, err := f.service.PreviewUpdate(filepath.Dir(f.project)); err == nil {
		t.Fatal("unregistered project accepted")
	}
	if _, err := f.service.ApplyUpdate("made-up-token"); err == nil {
		t.Fatal("unrecognized token accepted")
	}
	if _, err := f.service.RestoreUpdate("../outside"); err == nil {
		t.Fatal("invalid receipt accepted")
	}
	for _, content := range []string{"", "<!-- repsync:shared:start -->", strings.Repeat("x", 256*1024+1), string([]byte{0xff})} {
		writeSyncFile(t, f.source, content)
		if _, err := f.service.ConnectSharedSource(f.source); err == nil {
			t.Fatal("invalid source accepted")
		}
	}
	writeSyncFile(t, f.source, "Shared rules")
	agents := filepath.Join(f.project, "AGENTS.md")
	if err := os.Remove(agents); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(f.source, agents); err != nil {
		t.Fatal(err)
	}
	if _, err := f.service.PreviewUpdate(f.project); err == nil {
		t.Fatal("symlink target accepted")
	}
	if got := readSyncFile(t, f.source); got != "Shared rules" {
		t.Fatal("source changed")
	}
}

func TestRuleUpdatePreparedRecordSurvivesRestart(t *testing.T) {
	f := newSyncFixture(t, "Original", true)
	record := f.apply(t)
	receipt := filepath.Join(filepath.Dir(f.config), "sync", "history", record.ID+".json")
	data := map[string]any{}
	if err := json.Unmarshal([]byte(readSyncFile(t, receipt)), &data); err != nil {
		t.Fatal(err)
	}
	data["state"] = "prepared"
	raw, err := json.Marshal(data)
	if err != nil {
		t.Fatal(err)
	}
	writeSyncFile(t, receipt, string(raw))
	reloaded := workspace.NewService(f.config)
	history, err := reloaded.UpdateHistory(f.project)
	if err != nil || len(history) != 1 || history[0].State != "applied" {
		t.Fatalf("interrupted completion recovery: %+v %v", history, err)
	}
	if _, err := reloaded.RestoreUpdate(record.ID); err != nil {
		t.Fatal(err)
	}
	if got := readSyncFile(t, filepath.Join(f.project, "AGENTS.md")); got != "Original" {
		t.Fatal("backup was not restorable")
	}
}

func TestConcurrentRuleApplicationsDoNotOverwriteEachOther(t *testing.T) {
	f := newSyncFixture(t, "Original", true)
	first, second := f.preview(t), f.preview(t)
	results := make(chan error, 2)
	var workers sync.WaitGroup
	for _, preview := range []workspace.UpdatePreview{first, second} {
		workers.Go(func() { _, err := f.service.ApplyUpdate(preview.Token); results <- err })
	}
	workers.Wait()
	close(results)
	successes := 0
	for err := range results {
		if err == nil {
			successes++
		}
	}
	if successes != 1 {
		t.Fatalf("expected one application, got %d", successes)
	}
}
