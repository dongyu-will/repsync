//go:build integration

package workspace_test

import (
	"path/filepath"
	"strings"
	"testing"

	"repsync/internal/workspace"
)

func TestProjectReviewAcknowledgementsPersistAndReopenOnChange(t *testing.T) {
	f := newGitFixture(t)
	file := filepath.Join(f.repo, "draft.md")
	reviewWrite(t, file, "first draft")
	r := review(t, f)
	find := func(r workspace.ProjectReview, kind string) workspace.ReviewIssue {
		t.Helper()
		for _, i := range r.Issues {
			if i.Kind == kind {
				return i
			}
		}
		t.Fatalf("missing issue %s: %+v", kind, r.Issues)
		return workspace.ReviewIssue{}
	}
	i := find(r, "untracked")
	if _, err := f.service.SetReviewIssueState(f.repo, r.Version, i.Key, "retained", ""); err == nil {
		t.Fatal("reason not required")
	}
	if _, err := f.service.SetReviewIssueState(f.repo, r.Version, i.Key, "retained", "keep draft for later"); err != nil {
		t.Fatal(err)
	}
	r = review(t, f)
	if len(r.Acknowledged) != 1 || r.Acknowledged[0].Acknowledgement.Reason != "keep draft for later" {
		t.Fatal("acknowledgement missing")
	}
	fresh := workspace.NewService(f.config)
	t.Cleanup(fresh.Shutdown)
	persisted, err := fresh.ReviewProject(f.repo)
	if err != nil || len(persisted.Acknowledged) != 1 {
		t.Fatal("acknowledgement not persisted")
	}
	oldVersion := r.Version
	reviewWrite(t, file, "changed draft")
	r = review(t, f)
	if len(r.Acknowledged) != 0 || find(r, "untracked").Key == i.Key {
		t.Fatal("changed draft stayed hidden")
	}
	i = find(r, "untracked")
	if _, err := f.service.SetReviewIssueState(f.repo, oldVersion, i.Key, "later", "later"); err == nil {
		t.Fatal("stale report accepted")
	}
	if _, err := f.service.SetReviewIssueState(f.repo, r.Version, i.Key, "later", "review next time"); err != nil {
		t.Fatal(err)
	}
	r = review(t, f)
	if _, err := f.service.SetReviewIssueState(f.repo, r.Version, i.Key, "reopened", ""); err != nil {
		t.Fatal(err)
	}
	if len(review(t, f).Acknowledged) != 0 {
		t.Fatal("reopen failed")
	}
}

func TestProjectReviewStashRetentionDoesNotPermitCleanup(t *testing.T) {
	f := newGitFixture(t)
	reviewWrite(t, filepath.Join(f.repo, ".gitignore"), "stash changes\n")
	f.run("stash", "push", "-m", "AI saved work")
	r := review(t, f)
	var stash workspace.ReviewIssue
	for _, i := range r.Issues {
		if strings.HasPrefix(i.Kind, "stash_") {
			stash = i
		}
	}
	if stash.Key == "" {
		t.Fatal("stash reminder missing")
	}
	if _, err := f.service.SetReviewIssueState(f.repo, r.Version, stash.Key, "retained", "intentional stash"); err != nil {
		t.Fatal(err)
	}
	reviewWrite(t, filepath.Join(f.repo, "unrelated.md"), "other task")
	r = review(t, f)
	if len(r.Acknowledged) != 1 || r.Acknowledged[0].Key != stash.Key {
		t.Fatal("unrelated edits reopened retained stash")
	}
	view := f.view()
	if !strings.Contains(strings.Join(view.Warnings, " "), "stash") {
		t.Fatalf("retained stash bypassed cleanup guard: %+v", view)
	}
	if len(r.Snapshot.Stashes) != 1 {
		t.Fatal("retaining reminder removed actual stash")
	}
}
