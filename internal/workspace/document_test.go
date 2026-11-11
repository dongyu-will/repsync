//go:build integration

package workspace_test

import (
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"repsync/internal/workspace"
)

func TestSaveDocument(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	project := filepath.Join(root, "project")
	if err := os.Mkdir(project, 0700); err != nil {
		t.Fatal(err)
	}
	file := filepath.Join(project, "notes.markdown")
	original := "# Original\r\n本地修改\r\n"
	if err := os.WriteFile(file, []byte(original), 0640); err != nil {
		t.Fatal(err)
	}
	s := workspace.NewService(filepath.Join(root, "state", "projects.json"))
	p, err := s.Add(project)
	if err != nil {
		t.Fatal(err)
	}
	updated := "# Updated\n\n| A | B |\n|---|---|\n| 1 | 2 |\n"
	if err := s.SaveDocument(p.Path, "notes.markdown", original, updated); err != nil {
		t.Fatal(err)
	}
	got, err := s.ReadDocument(p.Path, "notes.markdown")
	if err != nil || got != updated {
		t.Fatalf("saved content: %q %v", got, err)
	}
	info, err := os.Stat(file)
	if err != nil || info.Mode().Perm() != 0640 {
		t.Fatalf("permissions changed: %v %v", info, err)
	}
	if err := s.SaveDocument(p.Path, "notes.markdown", original, "stale"); err == nil {
		t.Fatal("stale editor overwrote file")
	}
	if got, _ := os.ReadFile(file); string(got) != updated {
		t.Fatal("conflict changed file")
	}
	if err := s.SaveDocument(p.Path, "notes.markdown", updated, ""); err != nil {
		t.Fatal(err)
	}
	if got, _ := os.ReadFile(file); len(got) != 0 {
		t.Fatal("cannot save empty file")
	}
	if err := os.Remove(file); err != nil {
		t.Fatal(err)
	}
	if err := s.SaveDocument(p.Path, "notes.markdown", "", "recreated"); err == nil {
		t.Fatal("recreated externally deleted file")
	}
}

func TestSaveDocumentRejectsUnsafeTargets(t *testing.T) {
	t.Parallel()
	project := t.TempDir()
	s := workspace.NewService(filepath.Join(t.TempDir(), "projects.json"))
	p, err := s.Add(project)
	if err != nil {
		t.Fatal(err)
	}
	outside := filepath.Join(t.TempDir(), "outside.md")
	if err := os.WriteFile(outside, []byte("outside"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(project, "read-only.md"), []byte("keep"), 0400); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(project, "document.md"), []byte("keep"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink("document.md", filepath.Join(project, "internal.md")); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, filepath.Join(project, "external.md")); err != nil {
		t.Fatal(err)
	}
	for _, test := range []struct{ name, target, content string }{
		{"parent path", "../outside.md", "new"}, {"absolute", outside, "new"},
		{"non markdown", "package.json", "new"}, {"read only", "read-only.md", "new"},
		{"internal symlink", "internal.md", "new"}, {"external symlink", "external.md", "new"},
		{"large content", "document.md", strings.Repeat("x", 256*1024+1)},
		{"invalid UTF8", "document.md", string([]byte{0xff})},
	} {
		t.Run(test.name, func(t *testing.T) {
			if err := s.SaveDocument(p.Path, test.target, "keep", test.content); err == nil {
				t.Fatal("unsafe save accepted")
			}
		})
	}
	if err := s.SaveDocument(filepath.Dir(outside), "outside.md", "outside", "new"); err == nil {
		t.Fatal("unregistered project accepted")
	}
	if got, _ := os.ReadFile(outside); string(got) != "outside" {
		t.Fatal("outside content changed")
	}
	if got, _ := os.ReadFile(filepath.Join(project, "document.md")); string(got) != "keep" {
		t.Fatal("protected content changed")
	}
	entries, err := os.ReadDir(project)
	if err != nil {
		t.Fatal(err)
	}
	for _, entry := range entries {
		if strings.HasPrefix(entry.Name(), ".repsync-document-") {
			t.Fatal("temporary file leaked")
		}
	}
}

func TestSaveDocumentConcurrentEditors(t *testing.T) {
	t.Parallel()
	project := t.TempDir()
	if err := os.WriteFile(filepath.Join(project, "README.MD"), []byte("original"), 0600); err != nil {
		t.Fatal(err)
	}
	s := workspace.NewService(filepath.Join(t.TempDir(), "projects.json"))
	p, err := s.Add(project)
	if err != nil {
		t.Fatal(err)
	}
	var wg sync.WaitGroup
	results := make(chan error, 2)
	for _, content := range []string{"first", "second"} {
		wg.Go(func() { results <- s.SaveDocument(p.Path, "README.MD", "original", content) })
	}
	wg.Wait()
	close(results)
	success := 0
	for err := range results {
		if err == nil {
			success++
		}
	}
	if success != 1 {
		t.Fatalf("expected one successful save, got %d", success)
	}
}
