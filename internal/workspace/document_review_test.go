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

func TestDocumentReviewScanPreviewApply(t *testing.T) {
	if _, err := exec.LookPath("rg"); err != nil {
		t.Skip("ripgrep not installed")
	}
	f := newGitFixture(t)
	write := func(name, text string) {
		t.Helper()
		reviewWrite(t, filepath.Join(f.repo, name), text)
	}
	if err := os.MkdirAll(filepath.Join(f.repo, "docs", "archive"), 0700); err != nil {
		t.Fatal(err)
	}
	const readme = "# Setup\r\n\r\n本地使用MySQL启动。\r\n\r\n## Other\r\n保留本节。\r\n"
	write("README.md", readme)
	write("docs/export.md", "# Exports\n\nUse the legacy export module at /legacy-export.\n\nOther ways to export.\n")
	write("docs/archive/history.md", "MySQL was previously used.\n")
	write("backend.ts", "export function openMysql() { return 'mysql' }\nexport function boot() { return openMysql() }\n")
	write("legacy-export.ts", "export function legacyExport() { return 'csv' }\n")
	write("recovery.ts", "function openMysql() { return 'archive' }\nexport function restore() { return openMysql() }\n")
	if err := os.Symlink(filepath.Join(f.repo, "README.md"), filepath.Join(f.repo, "docs", "linked.md")); err != nil {
		t.Fatal(err)
	}
	f.run("add", ".")
	f.run("commit", "-m", "old database and export")
	codegraph, _ := exec.LookPath("codegraph")
	if codegraph != "" {
		cmd := exec.Command(codegraph, "init", "--yes", ".")
		cmd.Dir = f.repo
		cmd.Env = append(os.Environ(), "CODEGRAPH_TELEMETRY=0", "CODEGRAPH_NO_DAEMON=1")
		if output, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("initialize temporary CodeGraph index: %v %s", err, output)
		}
	}
	write("backend.ts", "export function openPostgres() { return 'postgres' }\nexport function boot() { return openPostgres() }\n")
	if err := os.Remove(filepath.Join(f.repo, "legacy-export.ts")); err != nil {
		t.Fatal(err)
	}
	report, err := f.service.ScanDocumentReview(f.repo, "worktree", "")
	if err != nil {
		t.Fatal(err)
	}
	if len(report.Findings) != 2 {
		t.Fatalf("want two candidates, got %+v", report)
	}
	mysql, oldExport := report.Findings[0], report.Findings[1]
	if mysql.Path != "README.md" || mysql.StartLine != 3 || !strings.Contains(mysql.Suggestion, "PostgreSQL") ||
		len(mysql.Evidence) == 0 || mysql.Evidence[0].Path != "backend.ts" {
		t.Fatalf("missing migration evidence or suggestion: %+v", mysql)
	}
	if oldExport.Path != "docs/export.md" || !slices.Contains(oldExport.Terms, "legacy export") {
		t.Fatalf("missing removed module: %+v", oldExport)
	}
	if codegraph != "" {
		if !slices.ContainsFunc(mysql.Evidence, func(e workspace.DocumentReviewEvidence) bool {
			return e.Source == "CodeGraph" && e.Path == "recovery.ts" && strings.Contains(e.Detail, "restore")
		}) || !slices.ContainsFunc(oldExport.Evidence, func(e workspace.DocumentReviewEvidence) bool {
			return e.Source == "CodeGraph" && e.Path == "" && strings.Contains(e.Detail, "legacyExport")
		}) {
			t.Fatalf("missing CodeGraph evidence for remaining and removed symbols: %+v", report)
		}
	}
	edit := func(finding workspace.DocumentFinding, replacement string) workspace.DocumentReviewEdit {
		return workspace.DocumentReviewEdit{
			Scope: "worktree", Basis: report.Basis, Path: finding.Path,
			StartLine: finding.StartLine, EndLine: finding.EndLine, Version: finding.Version,
			Replacement: replacement,
		}
	}
	draft := edit(mysql, "本地使用 PostgreSQL 启动。")
	preview, err := f.service.PreviewDocumentReview(f.repo, draft)
	if err != nil || preview.Before != "本地使用MySQL启动。\r\n" || preview.After != "本地使用 PostgreSQL 启动。\r\n" {
		t.Fatalf("preview: %+v %v", preview, err)
	}
	if original, err := f.service.ReadDocument(f.repo, "README.md"); err != nil || original != readme {
		t.Fatalf("preview changed the file: %q %v", original, err)
	}
	changed := draft
	changed.Replacement = "a different replacement"
	if err := f.service.ApplyDocumentReview(f.repo, changed, preview.Token); err == nil {
		t.Fatal("applied a different replacement than the preview")
	}
	if err := f.service.ApplyDocumentReview(f.repo, draft, preview.Token); err != nil {
		t.Fatal(err)
	}
	if actual, err := f.service.ReadDocument(f.repo, "README.md"); err != nil ||
		actual != strings.Replace(readme, "本地使用MySQL启动。", "本地使用 PostgreSQL 启动。", 1) {
		t.Fatalf("unexpected write: %q %v", actual, err)
	}
	draft = edit(oldExport, "")
	preview, err = f.service.PreviewDocumentReview(f.repo, draft)
	if err != nil || preview.After != "" {
		t.Fatalf("deletion preview: %+v %v", preview, err)
	}
	write("docs/export.md", oldExport.Original+"\nOther editor added this.\n")
	if err := f.service.ApplyDocumentReview(f.repo, draft, preview.Token); err == nil {
		t.Fatal("overwrote a concurrent documentation edit")
	}
	report, err = f.service.ScanDocumentReview(f.repo, "worktree", "")
	if err != nil || len(report.Findings) != 1 {
		t.Fatalf("rescan: %+v %v", report, err)
	}
	draft = edit(report.Findings[0], "")
	preview, err = f.service.PreviewDocumentReview(f.repo, draft)
	if err != nil {
		t.Fatal(err)
	}
	write("backend.ts", "export function openPostgres() { return 'postgresql' }\n")
	if err := f.service.ApplyDocumentReview(f.repo, draft, preview.Token); err == nil {
		t.Fatal("applied an edit after the source evidence changed")
	}
	f.run("add", "backend.ts", "legacy-export.ts")
	f.run("commit", "-m", "migrate database and retire export")
	report, err = f.service.ScanDocumentReview(f.repo, "commit", "")
	if err != nil || len(report.Findings) != 1 {
		t.Fatalf("last commit: %+v %v", report, err)
	}
	draft = edit(report.Findings[0], "")
	draft.Scope = "commit"
	preview, err = f.service.PreviewDocumentReview(f.repo, draft)
	if err != nil {
		t.Fatal(err)
	}
	if err := f.service.ApplyDocumentReview(f.repo, draft, preview.Token); err != nil {
		t.Fatal(err)
	}
	content, err := os.ReadFile(filepath.Join(f.repo, "docs", "export.md"))
	if err != nil || string(content) != "\nOther editor added this.\n" {
		t.Fatalf("deletion changed adjacent content: %q %v", content, err)
	}
}
