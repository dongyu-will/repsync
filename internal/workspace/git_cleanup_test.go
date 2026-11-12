//go:build integration

package workspace_test

import (
	"os"
	"testing"

	"repsync/internal/workspace"
)

func TestGitCleanupSelectionOnlyRemovesReviewedTargets(t *testing.T) {
	f := newGitFixture(t)
	selected := f.tree("topic-selected")
	kept := f.tree("topic-kept")
	f.run("branch", "topic-unused")
	f.age()
	view := f.view()
	if view.Policy.Enabled {
		t.Fatal("manual cleanup should not require automatic cleanup")
	}
	record, err := f.service.CleanupGitSelection(f.repo, view.Version, []workspace.GitCleanupTarget{
		{Kind: "worktree", Target: selected},
		{Kind: "branch", Target: "topic-selected"},
	})
	if err != nil || len(record.Items) != 2 || record.Automatic {
		t.Fatalf("selected cleanup: %+v %v", record, err)
	}
	for _, item := range record.Items {
		if item.Status != "removed" {
			t.Fatalf("selected item not removed: %+v", item)
		}
	}
	if _, err := os.Stat(selected); !os.IsNotExist(err) {
		t.Fatalf("selected worktree remains: %v", err)
	}
	if _, err := os.Stat(kept); err != nil {
		t.Fatalf("unselected worktree changed: %v", err)
	}
	if f.run("rev-parse", "topic-kept") != f.run("rev-parse", "topic-unused") {
		t.Fatal("unselected branch changed")
	}
	if _, err := f.service.RestoreGitCleanup(f.repo, record.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(selected); err != nil {
		t.Fatalf("selected cleanup could not be restored: %v", err)
	}
}

func TestGitCleanupSelectionWorktreeLeavesItsBranch(t *testing.T) {
	f := newGitFixture(t)
	path := f.tree("topic-selected")
	f.age()
	view := f.view()
	record, err := f.service.CleanupGitSelection(f.repo, view.Version, []workspace.GitCleanupTarget{
		{Kind: "worktree", Target: path},
	})
	if err != nil || len(record.Items) != 1 || record.Items[0].Status != "removed" {
		t.Fatalf("single worktree cleanup: %+v %v", record, err)
	}
	if f.run("rev-parse", "topic-selected") != f.run("rev-parse", "main") {
		t.Fatal("unselected linked branch was changed")
	}
}

func TestGitCleanupSelectionRejectsUnsafeOrIncompleteRequests(t *testing.T) {
	for _, name := range []string{"empty", "unknown", "protected", "duplicate", "linked worktree omitted", "stale preview", "within retention"} {
		t.Run(name, func(t *testing.T) {
			f := newGitFixture(t)
			path := f.tree("topic-selected")
			if name != "within retention" {
				f.age()
			}
			view := f.view()
			target := workspace.GitCleanupTarget{Kind: "worktree", Target: path}
			targets := []workspace.GitCleanupTarget{target}
			switch name {
			case "empty":
				targets = []workspace.GitCleanupTarget{}
			case "unknown":
				targets = append(targets, workspace.GitCleanupTarget{Kind: "branch", Target: "missing"})
			case "protected":
				targets = append(targets, workspace.GitCleanupTarget{Kind: "branch", Target: "main"})
			case "duplicate":
				targets = append(targets, target)
			case "linked worktree omitted":
				targets = []workspace.GitCleanupTarget{{Kind: "branch", Target: "topic-selected"}}
			case "stale preview":
				f.run("branch", "created-after-preview")
			}
			if _, err := f.service.CleanupGitSelection(f.repo, view.Version, targets); err == nil {
				t.Fatal("unsafe or incomplete selection was accepted")
			}
			if _, err := os.Stat(path); err != nil {
				t.Fatalf("rejected selection removed a worktree: %v", err)
			}
			if f.run("rev-parse", "topic-selected") != f.run("rev-parse", "main") {
				t.Fatal("rejected selection changed branch refs")
			}
		})
	}
}
