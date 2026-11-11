//go:build integration

package workspace_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestGitStashSharedPreviewApplyAndCleanupProtection(t *testing.T) {
	f := newGitFixture(t)
	tree := f.tree("topic-stash")
	f.age()
	old := f.view()
	tracked := filepath.Join(f.repo, ".gitignore")
	if err := os.WriteFile(tracked, []byte("cache/\nstashed-rule\n"), 0600); err != nil {
		t.Fatal(err)
	}
	f.run("add", ".gitignore")
	if err := os.WriteFile(filepath.Join(f.repo, "draft.md"), []byte("untracked draft"), 0600); err != nil {
		t.Fatal(err)
	}
	f.run("stash", "push", "--include-untracked", "-m", "AI before push")
	list, err := f.service.GitStashes(f.repo)
	if err != nil || len(list.Entries) != 1 {
		t.Fatalf("list: %+v %v", list, err)
	}
	if !strings.Contains(list.Entries[0].Subject, "AI before push") {
		t.Fatal("lost stash description")
	}
	inspection, err := f.service.Inspect(f.repo)
	if err != nil || !inspection.StashKnown || inspection.StashCount != 1 || inspection.Status != "" {
		t.Fatalf("notice on clean tree: %+v %v", inspection, err)
	}
	if _, err := f.service.Add(tree); err != nil {
		t.Fatal(err)
	}
	shared, err := f.service.GitStashes(tree)
	if err != nil || len(shared.Entries) != 1 || shared.Entries[0].OID != list.Entries[0].OID {
		t.Fatalf("worktree stash not shared: %+v %v", shared, err)
	}
	if _, err := f.service.CleanupGitWorkspace(f.repo, old.Version); err == nil {
		t.Fatal("stale cleanup ignored new stash")
	}
	for _, w := range f.view().Worktrees {
		if w.Eligible {
			t.Fatalf("stash did not protect worktree: %+v", w)
		}
	}
	for _, b := range f.view().Branches {
		if b.Eligible {
			t.Fatalf("stash did not protect branch: %+v", b)
		}
	}
	preview, err := f.service.GitStashChanges(f.repo, list.Entries[0].OID)
	if err != nil || !strings.Contains(preview.Patch, "stashed-rule") || !strings.Contains(preview.Patch, "draft.md") {
		t.Fatalf("patch missing staged/untracked: %+v %v", preview, err)
	}
	result, err := f.service.ApplyGitStash(f.repo, preview.Stash.OID, preview.Version)
	if err != nil || !result.Applied {
		t.Fatalf("apply: %+v %v", result, err)
	}
	if got, err := os.ReadFile(filepath.Join(f.repo, "draft.md")); err != nil || string(got) != "untracked draft" {
		t.Fatal("untracked draft not restored")
	}
	if !strings.Contains(f.run("diff", "--cached"), "stashed-rule") {
		t.Fatal("staged changes not restored")
	}
	if f.run("rev-parse", "refs/stash") != preview.Stash.OID {
		t.Fatal("stash was dropped")
	}
	if _, err := f.service.ApplyGitStash(f.repo, preview.Stash.OID, preview.Version); err == nil {
		t.Fatal("applied over dirty tree")
	}
}

func TestGitStashRestoreRequiresRegisteredWorktreeRoot(t *testing.T) {
	f := newGitFixture(t)
	if err := os.WriteFile(filepath.Join(f.repo, ".gitignore"), []byte("stash rule\n"), 0600); err != nil {
		t.Fatal(err)
	}
	f.run("stash", "push", "-m", "root changes")
	sub := filepath.Join(f.repo, "subproject")
	if err := os.Mkdir(sub, 0700); err != nil {
		t.Fatal(err)
	}
	if _, err := f.service.Add(sub); err != nil {
		t.Fatal(err)
	}
	list, err := f.service.GitStashes(sub)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f.service.ApplyGitStash(sub, list.Entries[0].OID, list.Version); err == nil {
		t.Fatal("stash applied outside registered subproject")
	}
	if got, err := os.ReadFile(filepath.Join(f.repo, ".gitignore")); err != nil || string(got) != "cache/\n" {
		t.Fatal("parent repository changed")
	}
}

func TestGitStashUsesObjectIdentityAndRejectsStalePreview(t *testing.T) {
	f := newGitFixture(t)
	file := filepath.Join(f.repo, ".gitignore")
	var firstOID, oldVersion string
	for _, text := range []string{"first", "second"} {
		if err := os.WriteFile(file, []byte("cache/\n"+text+"\n"), 0600); err != nil {
			t.Fatal(err)
		}
		f.run("stash", "push", "-m", text)
		if text == "first" {
			list, err := f.service.GitStashes(f.repo)
			if err != nil {
				t.Fatal(err)
			}
			first := list.Entries[0]
			preview, err := f.service.GitStashChanges(f.repo, first.OID)
			if err != nil {
				t.Fatal(err)
			}
			firstOID, oldVersion = first.OID, preview.Version
		}
	}
	if _, err := f.service.ApplyGitStash(f.repo, firstOID, oldVersion); err == nil {
		t.Fatal("stale stash index accepted")
	}
	preview, err := f.service.GitStashChanges(f.repo, firstOID)
	if err != nil {
		t.Fatal(err)
	}
	if preview.Stash.Selector != "stash@{1}" {
		t.Fatalf("wrong stash: %+v", preview.Stash)
	}
	result, err := f.service.ApplyGitStash(f.repo, firstOID, preview.Version)
	if err != nil || !result.Applied {
		t.Fatalf("apply old stash: %+v %v", result, err)
	}
	got, err := os.ReadFile(file)
	if err != nil || !strings.Contains(string(got), "first") || strings.Contains(string(got), "second") {
		t.Fatal("restored wrong stash")
	}
	if f.run("stash", "list", "--format=%H") == firstOID {
		t.Fatal("newer stash disappeared")
	}
	if _, err := f.service.GitStashChanges(f.repo, "--all"); err == nil {
		t.Fatal("unlisted object accepted")
	}
}

func TestGitStashConflictsAndIgnoredFilesRetainOriginal(t *testing.T) {
	t.Run("merge conflict", func(t *testing.T) {
		f := newGitFixture(t)
		file := filepath.Join(f.repo, ".gitignore")
		if err := os.WriteFile(file, []byte("stash version\n"), 0600); err != nil {
			t.Fatal(err)
		}
		f.run("stash", "push", "-m", "conflict")
		if err := os.WriteFile(file, []byte("current version\n"), 0600); err != nil {
			t.Fatal(err)
		}
		f.run("add", ".gitignore")
		f.run("commit", "-m", "current")
		list, err := f.service.GitStashes(f.repo)
		if err != nil {
			t.Fatal(err)
		}
		result, err := f.service.ApplyGitStash(f.repo, list.Entries[0].OID, list.Version)
		if err != nil || result.Applied || !strings.Contains(result.Message, "Original stash preserved") {
			t.Fatalf("conflict: %+v %v", result, err)
		}
		if f.run("rev-parse", "refs/stash") != list.Entries[0].OID {
			t.Fatal("lost conflicting stash")
		}
	})
	t.Run("ignored file collision", func(t *testing.T) {
		f := newGitFixture(t)
		dir := filepath.Join(f.repo, "cache")
		file := filepath.Join(dir, "keep")
		if err := os.Mkdir(dir, 0700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(file, []byte("stashed ignored"), 0600); err != nil {
			t.Fatal(err)
		}
		f.run("stash", "push", "--all", "-m", "ignored")
		if err := os.Mkdir(dir, 0700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(file, []byte("current ignored"), 0600); err != nil {
			t.Fatal(err)
		}
		list, err := f.service.GitStashes(f.repo)
		if err != nil {
			t.Fatal(err)
		}
		result, err := f.service.ApplyGitStash(f.repo, list.Entries[0].OID, list.Version)
		if err != nil || result.Applied {
			t.Fatalf("ignored collision: %+v %v", result, err)
		}
		if got, err := os.ReadFile(file); err != nil || string(got) != "current ignored" {
			t.Fatal("overwrote ignored file")
		}
		if f.run("rev-parse", "refs/stash") != list.Entries[0].OID {
			t.Fatal("lost ignored stash")
		}
	})
}
