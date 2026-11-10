//go:build integration

package workspace_test

import (
	"os"
	"path/filepath"
	"repsync/internal/workspace"
	"strings"
	"testing"
)

func resourceFixture(t *testing.T) (*workspace.Service, string, string) {
	t.Helper()
	s := workspace.NewService(filepath.Join(t.TempDir(), "projects.json"))
	project, err := s.Add(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	source := filepath.Join(t.TempDir(), "shared-check.sh")
	writeSyncFile(t, source, "#!/bin/sh\necho version-one\n")
	if err := os.Chmod(source, 0755); err != nil {
		t.Fatal(err)
	}
	if _, err := s.ConnectResource("check", source, "shared-check.sh"); err != nil {
		t.Fatal(err)
	}
	writeSyncFile(t, filepath.Join(project.Path, ".repsync.json"), `{"services":[],"pipelines":[],"resources":["check"]}`)
	return s, project.Path, source
}

func TestResourceUpdateAdoptionAndProtectedRestore(t *testing.T) {
	t.Parallel()
	s, path, source := resourceFixture(t)
	target := filepath.Join(path, "shared-check.sh")
	writeSyncFile(t, filepath.Join(path, "AGENTS.md"), "# Project rules\nKeep these.\n")
	preview, err := s.PreviewResource(path, "check")
	if err != nil || preview.Conflict || preview.Target != "shared-check.sh" || !strings.Contains(preview.Diff, "shared-check.sh") {
		t.Fatalf("preview: %+v %v", preview, err)
	}
	if _, err := os.Stat(target); !os.IsNotExist(err) {
		t.Fatal("preview wrote target")
	}
	first, err := s.ApplyUpdate(preview.Token)
	if err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(target)
	if err != nil || info.Mode().Perm() != 0755 {
		t.Fatal("script mode lost")
	}
	status, err := s.ResourceStatus(path)
	if err != nil || status[0].Status != "current" {
		t.Fatalf("adoption: %+v %v", status, err)
	}
	originalHistory, err := s.UpdateHistory(path)
	if err != nil || len(originalHistory) != 0 {
		t.Fatal("resource appeared as an AGENTS update")
	}
	writeSyncFile(t, source, "#!/bin/sh\necho version-two\n")
	status, err = s.ResourceStatus(path)
	if err != nil || status[0].Status != "outdated" {
		t.Fatalf("outdated: %+v %v", status, err)
	}
	secondPreview, err := s.PreviewResource(path, "check")
	if err != nil || secondPreview.Conflict {
		t.Fatal(err)
	}
	second, err := s.ApplyUpdate(secondPreview.Token)
	if err != nil {
		t.Fatal(err)
	}
	writeSyncFile(t, target, "# Local project change\n")
	if _, err := s.RestoreUpdate(second.ID); err == nil {
		t.Fatal("restore overwrote local change")
	}
	conflict, err := s.PreviewResource(path, "check")
	if err != nil || !conflict.Conflict {
		t.Fatal("local difference not identified")
	}
	if _, err := s.ApplyUpdate(conflict.Token); err == nil {
		t.Fatal("resource update overwrote project difference")
	}
	writeSyncFile(t, target, readSyncFile(t, source))
	if err := os.Chmod(target, 0755); err != nil {
		t.Fatal(err)
	}
	if _, err := s.RestoreUpdate(second.ID); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(readSyncFile(t, target), "version-one") {
		t.Fatal("did not restore previous resource")
	}
	status, err = s.ResourceStatus(path)
	if err != nil || status[0].Status != "outdated" {
		t.Fatalf("restored resource cannot be updated again: %+v %v", status, err)
	}
	history, err := s.ResourceHistory(path)
	if err != nil || len(history) != 2 || history[0].Target != "shared-check.sh" {
		t.Fatalf("resource history: %+v %v", history, err)
	}
	if _, err := s.RestoreUpdate(first.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(target); !os.IsNotExist(err) {
		t.Fatal("restore did not remove originally absent target")
	}
	if readSyncFile(t, filepath.Join(path, "AGENTS.md")) != "# Project rules\nKeep these.\n" {
		t.Fatal("resource touched project rules")
	}
}

func TestResourceRejectsStaleSourceAdoptionAndSymlinks(t *testing.T) {
	t.Parallel()
	s, path, source := resourceFixture(t)
	preview, err := s.PreviewResource(path, "check")
	if err != nil {
		t.Fatal(err)
	}
	writeSyncFile(t, source, "# Changed source\n")
	if _, err := s.ApplyUpdate(preview.Token); err == nil {
		t.Fatal("applied stale resource source")
	}
	preview, err = s.PreviewResource(path, "check")
	if err != nil {
		t.Fatal(err)
	}
	writeSyncFile(t, filepath.Join(path, ".repsync.json"), `{"resources":[]}`)
	if _, err := s.ApplyUpdate(preview.Token); err == nil {
		t.Fatal("applied no longer adopted resource")
	}
	writeSyncFile(t, filepath.Join(path, ".repsync.json"), `{"resources":["check"]}`)
	if err := os.Symlink(source, filepath.Join(path, "shared-check.sh")); err != nil {
		t.Fatal(err)
	}
	if _, err := s.PreviewResource(path, "check"); err == nil {
		t.Fatal("accepted target symlink")
	}
	if _, err := s.ConnectResource("bad", source, "../outside"); err == nil {
		t.Fatal("accepted outside target")
	}
	if _, err := s.ConnectResource("bad", source, ".repsync.json"); err == nil {
		t.Fatal("accepted project config target")
	}
	for _, target := range []string{".GIT/config", "AGENTS.MD", ".REPSYNC.JSON"} {
		if _, err := s.ConnectResource("bad", source, target); err == nil {
			t.Fatalf("accepted case alias of reserved target: %s", target)
		}
	}
	parent := filepath.Join(path, "linked")
	if err := os.Symlink(t.TempDir(), parent); err != nil {
		t.Fatal(err)
	}
	if _, err := s.ConnectResource("nested", source, "linked/file.sh"); err != nil {
		t.Fatal(err)
	}
	writeSyncFile(t, filepath.Join(path, ".repsync.json"), `{"resources":["nested"]}`)
	if _, err := s.PreviewResource(path, "nested"); err == nil {
		t.Fatal("accepted symlink parent")
	}
}
