//go:build integration

package workspace_test

import (
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"repsync/internal/workspace"
)

func TestRegistrationAndReadOnlyInspection(t *testing.T) {
	root := t.TempDir()
	repo := filepath.Join(root, "project with spaces")
	if err := os.Mkdir(repo, 0700); err != nil {
		t.Fatal(err)
	}
	write := func(name, content string) {
		t.Helper()
		if err := os.WriteFile(filepath.Join(repo, name), []byte(content), 0600); err != nil {
			t.Fatal(err)
		}
	}
	runGit := func(args ...string) string {
		t.Helper()
		cmd := exec.Command("git", append([]string{"-C", repo}, args...)...)
		cmd.Env = append(os.Environ(), "GIT_CONFIG_GLOBAL=/dev/null", "GIT_CONFIG_SYSTEM=/dev/null")
		out, err := cmd.CombinedOutput()
		if err != nil {
			t.Fatalf("git %v: %v %s", args, err, out)
		}
		return string(out)
	}
	runGit("init", "-b", "main")
	write("AGENTS.md", "# Local rules\n")
	write("package.json", `{"scripts":{"lint":"echo check"}}`)
	runGit("add", "AGENTS.md", "package.json")
	runGit("-c", "user.name=Test", "-c", "user.email=test@example.invalid", "commit", "-m", "fixture")
	write("AGENTS.md", "# Local rules\nPreserve this change.\n")
	write("untracked.txt", "Keep this file.")
	before := runGit("status", "--porcelain=v1")
	service := workspace.NewService(filepath.Join(root, "config", "projects.json"))
	if _, err := service.Inspect(repo); err == nil {
		t.Fatal("unregistered project accepted")
	}
	if _, err := service.Add("relative"); err == nil {
		t.Fatal("relative path accepted")
	}
	p, err := service.Add(repo)
	if err != nil {
		t.Fatal(err)
	}
	// A symlink to the same directory must not create a second registration.
	link := filepath.Join(root, "alias")
	if err := os.Symlink(repo, link); err != nil {
		t.Fatal(err)
	}
	if _, err := service.Add(link); err != nil {
		t.Fatal(err)
	}
	projects, err := workspace.NewService(filepath.Join(root, "config", "projects.json")).List()
	if err != nil || len(projects) != 1 {
		t.Fatalf("persisted registrations: %v %v", projects, err)
	}
	inspection, err := service.Inspect(p.Path)
	if err != nil {
		t.Fatal(err)
	}
	if !inspection.Git || !inspection.StatusKnown || inspection.Branch != "main" || !strings.Contains(inspection.Status, "untracked.txt") || !slices.Equal(inspection.Documents, []string{"AGENTS.md"}) || inspection.Scripts["lint"] != "echo check" {
		t.Fatalf("incorrect inspection: %+v", inspection)
	}
	content, err := service.ReadDocument(p.Path, "AGENTS.md")
	if err != nil || !strings.Contains(content, "Preserve this change.") {
		t.Fatalf("incorrect document: %q %v", content, err)
	}
	if after := runGit("status", "--porcelain=v1"); after != before {
		t.Fatalf("inspection changed worktree: %q -> %q", before, after)
	}
	data, err := os.ReadFile(filepath.Join(repo, "untracked.txt"))
	if err != nil || string(data) != "Keep this file." {
		t.Fatal("untracked content changed")
	}
	if err := os.RemoveAll(repo); err != nil {
		t.Fatal(err)
	}
	if _, err := service.Inspect(p.Path); err == nil {
		t.Fatal("missing project accepted")
	}
}

func TestRootMarkdownDocuments(t *testing.T) {
	root := t.TempDir()
	project := filepath.Join(root, "project")
	if err := os.MkdirAll(filepath.Join(project, "nested.md"), 0700); err != nil {
		t.Fatal(err)
	}
	files := map[string]string{
		"PRODUCT.md":         "# Product\n",
		"README.MD":          "",
		"notes.markdown":     "# Notes\n",
		"large.md":           strings.Repeat("x", 256*1024+1),
		"nested.md/child.md": "Nested document",
		"package.json":       "{}",
	}
	for name, content := range files {
		if err := os.WriteFile(filepath.Join(project, name), []byte(content), 0600); err != nil {
			t.Fatal(err)
		}
	}
	outside := filepath.Join(root, "outside.md")
	if err := os.WriteFile(outside, []byte("Outside project"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, filepath.Join(project, "external.md")); err != nil {
		t.Fatal(err)
	}
	service := workspace.NewService(filepath.Join(root, "projects.json"))
	p, err := service.Add(project)
	if err != nil {
		t.Fatal(err)
	}
	inspection, err := service.Inspect(p.Path)
	if err != nil || !slices.Equal(inspection.Documents, []string{"PRODUCT.md", "README.MD", "external.md", "large.md", "notes.markdown"}) {
		t.Fatalf("root document list: %v %v", inspection.Documents, err)
	}
	for _, name := range []string{"PRODUCT.md", "README.MD", "notes.markdown"} {
		content, err := service.ReadDocument(p.Path, name)
		if err != nil || content != files[name] {
			t.Fatalf("read %s: %q %v", name, content, err)
		}
	}
	for _, name := range []string{"../outside.md", outside, "external.md", "package.json", "nested.md", "nested.md/child.md", "missing.md", "large.md"} {
		t.Run(name, func(t *testing.T) {
			content, err := service.ReadDocument(p.Path, name)
			if err == nil || content != "" {
				t.Fatalf("unexpected document access: %q %v", content, err)
			}
		})
	}
	if _, err := service.ReadDocument(root, "outside.md"); err == nil {
		t.Fatal("unregistered document read accepted")
	}
}

func TestCorruptRegistrationIsPreserved(t *testing.T) {
	root := t.TempDir()
	registry := filepath.Join(root, "projects.json")
	if err := os.WriteFile(registry, []byte("invalid json"), 0600); err != nil {
		t.Fatal(err)
	}
	service := workspace.NewService(registry)
	if _, err := service.Add(root); err == nil {
		t.Fatal("corrupt registry silently replaced")
	}
	data, err := os.ReadFile(registry)
	if err != nil || string(data) != "invalid json" {
		t.Fatal("corrupt registry was overwritten")
	}
}

func TestProjectRemovalAndRelocationPreserveFiles(t *testing.T) {
	t.Parallel()
	f := newSyncFixture(t, "# Project rules\nKeep release gates.\n", true)
	pending := f.preview(t)
	record := f.apply(t)
	target := filepath.Join(f.project, "AGENTS.md")
	content := readSyncFile(t, target)
	moved := filepath.Join(filepath.Dir(f.project), "moved project")
	if err := os.Rename(f.project, moved); err != nil {
		t.Fatal(err)
	}
	next, err := f.service.RelocateProject(f.project, moved)
	if err != nil || next.Path != moved {
		t.Fatalf("repair missing directory: %+v %v", next, err)
	}
	if readSyncFile(t, filepath.Join(moved, "AGENTS.md")) != content {
		t.Fatal("relocate modified project")
	}
	if _, err := f.service.ApplyUpdate(pending.Token); err == nil {
		t.Fatal("old preview survived relocation")
	}
	if _, err := f.service.RestoreUpdate(record.ID); err == nil {
		t.Fatal("old backup followed another directory")
	}
	other, err := f.service.Add(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f.service.RelocateProject(next.Path, other.Path); err == nil {
		t.Fatal("merged registered projects")
	}
	if err := f.service.RemoveProject(next.Path); err != nil {
		t.Fatal(err)
	}
	if readSyncFile(t, filepath.Join(moved, "AGENTS.md")) != content {
		t.Fatal("remove deleted project")
	}
	if _, err := f.service.Add(moved); err != nil {
		t.Fatal(err)
	}
	// A missing directory can still be unregistered.
	if err := os.Rename(moved, f.project); err != nil {
		t.Fatal(err)
	}
	if err := f.service.RemoveProject(moved); err != nil {
		t.Fatal(err)
	}
	if _, err := f.service.Add(f.project); err != nil {
		t.Fatal(err)
	}
	if _, err := f.service.RestoreUpdate(record.ID); err != nil {
		t.Fatal("original backup lost:", err)
	}
}

func TestProjectRegistrationChangesRejectRunningTasks(t *testing.T) {
	t.Parallel()
	s := workspace.NewService(filepath.Join(t.TempDir(), "projects.json"))
	t.Cleanup(s.Shutdown)
	path := batchProject(t, s, workspace.LocalServiceDefinition{ID: "worker", Name: "Worker", Command: []string{"/bin/sleep", "30"}})
	if _, err := s.StartProjects([]string{path}, false); err != nil {
		t.Fatal(err)
	}
	if err := s.RemoveProject(path); err == nil {
		t.Fatal("unregistered active service")
	}
	if _, err := s.RelocateProject(path, t.TempDir()); err == nil {
		t.Fatal("relocated active service")
	}
	if _, err := s.StopProjects([]string{path}); err != nil {
		t.Fatal(err)
	}
	writeSyncFile(t, filepath.Join(path, ".repsync.json"), `{"pipelines":[{"id":"build","name":"Build","kind":"cd","steps":[["/bin/sleep","30"]],"timeoutSeconds":60}]}`)
	config, err := s.Pipelines(path)
	if err != nil {
		t.Fatal(err)
	}
	run, err := s.StartPipeline(path, "build", config.Version)
	if err != nil {
		t.Fatal(err)
	}
	if err := s.RemoveProject(path); err == nil {
		t.Fatal("unregistered active pipeline")
	}
	if err := s.StopPipeline(path, run.ID); err != nil {
		t.Fatal(err)
	}
}
