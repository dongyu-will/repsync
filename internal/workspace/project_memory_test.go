//go:build integration

package workspace_test

import (
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"repsync/internal/workspace"
)

func TestProjectMemoryDocumentEvidence(t *testing.T) {
	if _, err := exec.LookPath("rg"); err != nil {
		t.Skip("ripgrep not installed")
	}
	f := newGitFixture(t)
	readme := filepath.Join(f.repo, "README.md")
	reviewWrite(t, readme, "# Setup\n\nStart MySQL before the service.\n")
	f.run("add", "README.md")
	f.run("commit", "-m", "database guide")
	input := workspace.ProjectMemoryInput{
		Statement: "PostgreSQL 已完成切换", Source: "部署记录 #42", Environment: "生产",
		Keywords: []string{"MySQL"},
	}
	noSource := input
	noSource.Source = ""
	if _, err := f.service.ProposeProjectMemory(f.repo, noSource); err == nil {
		t.Fatal("accepted a memory without a source")
	}
	item, err := f.service.ProposeProjectMemory(f.repo, input)
	if err != nil || item.Status != "pending" || item.Source != input.Source {
		t.Fatalf("proposal: %+v %v", item, err)
	}
	pending := item
	scan := func() workspace.DocumentReview {
		t.Helper()
		report, err := f.service.ScanDocumentReview(f.repo, "worktree", "")
		if err != nil {
			t.Fatal(err)
		}
		return report
	}
	set := func(item workspace.ProjectMemory, action string) workspace.ProjectMemory {
		t.Helper()
		next, err := f.service.SetProjectMemoryState(f.repo, item.ID, item.Version, action)
		if err != nil {
			t.Fatal(err)
		}
		return next
	}
	if report := scan(); len(report.Findings) != 0 {
		t.Fatalf("pending proposal used by scan: %+v", report)
	}
	item = set(set(item, "archive"), "restore")
	if item.Status != "pending" {
		t.Fatalf("undo unexpectedly confirmed a pending record: %+v", item)
	}
	if _, err := f.service.SetProjectMemoryState(f.repo, pending.ID, pending.Version, "confirm"); err == nil {
		t.Fatal("accepted a stale confirmation")
	}
	item = set(item, "confirm")
	report := scan()
	if len(report.Findings) != 1 {
		t.Fatalf("confirmed keyword did not match clean worktree: %+v", report)
	}
	finding := report.Findings[0]
	if !slices.ContainsFunc(finding.Evidence, func(e workspace.DocumentReviewEvidence) bool {
		return e.MemoryID == item.ID && e.Path == input.Source && strings.Contains(e.Detail, input.Environment) &&
			strings.Contains(e.Detail, input.Statement)
	}) {
		t.Fatalf("missing cited memory: %+v", finding.Evidence)
	}
	edit := workspace.DocumentReviewEdit{
		Scope: "worktree", Basis: report.Basis, Path: finding.Path, StartLine: finding.StartLine,
		EndLine: finding.EndLine, Version: finding.Version, Replacement: "Start PostgreSQL before the service.",
	}
	preview, err := f.service.PreviewDocumentReview(f.repo, edit)
	if err != nil {
		t.Fatal(err)
	}
	item = set(item, "archive")
	if err := f.service.ApplyDocumentReview(f.repo, edit, preview.Token); err == nil {
		t.Fatal("applied a preview using archived evidence")
	}
	if report := scan(); len(report.Findings) != 0 {
		t.Fatalf("archived memory used by scan: %+v", report)
	}
	item = set(item, "restore")
	if item.Status != "confirmed" || item.ConfirmedAt == "" || item.Source != input.Source {
		t.Fatalf("restore lost previous status or provenance: %+v", item)
	}
	reopened := workspace.NewService(f.config)
	confirmed, err := reopened.ConfirmedProjectMemories(f.repo)
	if err != nil || len(confirmed) != 1 || confirmed[0].ID != item.ID {
		t.Fatalf("reopening lost confirmed memory: %+v %v", confirmed, err)
	}
	other, err := f.service.Add(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	confirmed, err = f.service.ConfirmedProjectMemories(other.Path)
	if err != nil || len(confirmed) != 0 {
		t.Fatalf("cross-project memory leak: %+v %v", confirmed, err)
	}
}
