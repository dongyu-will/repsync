//go:build integration

package workspace_test

import (
	"path/filepath"
	"repsync/internal/workspace"
	"strings"
	"testing"
)

func TestSharedAdoptionTracksContentAndPreservesDifferences(t *testing.T) {
	t.Parallel()
	original := "# Project overrides\nUse the project release gate.\n"
	f := newSyncFixture(t, original, true)
	state := func(want string) workspace.SharedAdoption {
		t.Helper()
		items, err := f.service.SharedAdoptions()
		if err != nil || len(items) != 1 || items[0].Status != want {
			t.Fatalf("adoption %s: %+v %v", want, items, err)
		}
		return items[0]
	}
	if !state("not_adopted").HasProjectContent {
		t.Fatal("project differences missing")
	}
	preview := f.preview(t)
	if preview.ProjectContent != original {
		t.Fatal("preview lost project differences")
	}
	record := f.apply(t)
	current := state("current")
	if current.AdoptedVersion != record.Version {
		t.Fatal("missing adopted version")
	}
	writeSyncFile(t, f.source, "# Updated shared rules\nRun scoped checks.\n")
	if state("outdated").AdoptedVersion != record.Version {
		t.Fatal("lost adoption evidence")
	}
	target := filepath.Join(f.project, "AGENTS.md")
	before := readSyncFile(t, target)
	writeSyncFile(t, target, strings.Replace(before, "Use the existing project checks.", "Locally modified common block.", 1))
	state("modified")
	if !strings.Contains(f.preview(t).ProjectContent, original) {
		t.Fatal("project rules not preserved in plan")
	}
	// No implicit write occurs when querying status.
	if strings.Contains(readSyncFile(t, target), "Updated shared rules") {
		t.Fatal("status changed project")
	}
	writeSyncFile(t, target, "<!-- repsync:shared:start -->\nbroken")
	if state("error").Error == "" {
		t.Fatal("malformed markers not explained")
	}
	writeSyncFile(t, target, before)
	if _, err := f.service.RestoreUpdate(record.ID); err != nil {
		t.Fatal(err)
	}
	state("not_adopted")
}
