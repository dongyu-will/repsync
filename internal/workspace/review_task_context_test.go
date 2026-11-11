//go:build integration

package workspace_test

import (
	"path/filepath"
	"strings"
	"testing"

	"repsync/internal/workspace"
)

func TestProjectReviewTaskContextWarnsAboutSharedDatabase(t *testing.T) {
	f := newGitFixture(t)
	input := workspace.ReviewTaskContext{Title: "API change", Owner: "AI session A", Areas: []string{"backend", "database"}, Paths: []string{"internal/api"}, Environment: "staging", Target: "shared-db"}
	first, err := f.service.StartReviewTaskWithContext(f.repo, review(t, f).Version, input)
	if err != nil {
		t.Fatal(err)
	}
	reviewWrite(t, filepath.Join(f.repo, "draft.md"), "other work")
	// A separate registered project may still address the same database.
	otherPath := f.tree("feature")
	if _, err := f.service.Add(otherPath); err != nil {
		t.Fatal(err)
	}
	other, err := f.service.ReviewProject(otherPath)
	if err != nil {
		t.Fatal(err)
	}
	input.Title, input.Owner, input.Paths = "Migration", "AI session B", []string{"migrations"}
	if _, err := f.service.StartReviewTaskWithContext(otherPath, other.Version, input); err != nil {
		t.Fatal(err)
	}
	r := review(t, f)
	if !strings.Contains(strings.Join(r.TaskWarnings, " "), "Same database environment and target") {
		t.Fatalf("shared database unnoticed: %+v", r.TaskWarnings)
	}
	done, err := f.service.FinishReviewTask(f.repo, first.ID, r.Version, workspace.ReviewFinishInput{Summary: "done", Checks: "API only", Blockers: "coordinate migration"})
	if err != nil {
		t.Fatal(err)
	}
	for _, text := range []string{"AI session A", "shared-db", "staging", "internal/api"} {
		if !strings.Contains(done.Handover, text) {
			t.Fatalf("handover lost %s", text)
		}
	}
	if len(review(t, f).TaskWarnings) != 0 {
		t.Fatal("closed task still considered concurrent")
	}
	input.Paths = []string{"../escape"}
	if _, err := f.service.StartReviewTaskWithContext(f.repo, review(t, f).Version, input); err == nil {
		t.Fatal("outside task path accepted")
	}
}
