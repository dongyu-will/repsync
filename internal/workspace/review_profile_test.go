//go:build integration

package workspace_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"repsync/internal/workspace"
)

func TestProjectReviewProfilePreservesConfiguration(t *testing.T) {
	f := newGitFixture(t)
	config := filepath.Join(f.repo, ".repsync.json")
	reviewWrite(t, config, `{"services":[],"pipelines":[],"extension":{"keep":true}}`)
	if err := os.Chmod(config, 0640); err != nil {
		t.Fatal(err)
	}
	before := review(t, f)
	if before.ProfileConfigured {
		t.Fatal("missing profile was assumed configured")
	}
	profile := workspace.ReviewProfile{Kind: "backend", Areas: []string{"backend", "database"}, Requirements: []string{"backend-api", "database-integration", "database-migrations", "database-consistency", "database-recovery"}, Stages: []string{"push"}}
	if err := f.service.SaveReviewProfile(f.repo, before.ConfigVersion, profile); err != nil {
		t.Fatal(err)
	}
	if err := f.service.SaveReviewProfile(f.repo, before.ConfigVersion, profile); err == nil {
		t.Fatal("stale profile overwrote configuration")
	}
	r := review(t, f)
	if !r.ProfileConfigured || len(r.Coverage) != 5 {
		t.Fatalf("profile: %+v", r.Profile)
	}
	for _, stage := range r.Delivery {
		if stage.Stage == "deploy" && stage.Status != "not_applicable" {
			t.Fatal("inapplicable deploy required")
		}
	}
	data, _ := os.ReadFile(config)
	info, _ := os.Stat(config)
	if !strings.Contains(string(data), `"extension"`) || !strings.Contains(string(data), `"services"`) || info.Mode().Perm() != 0640 {
		t.Fatal("configuration or permissions lost")
	}
	profile.Requirements = append(profile.Requirements, "frontend-static")
	if err := f.service.SaveReviewProfile(f.repo, r.ConfigVersion, profile); err == nil {
		t.Fatal("requirement outside project area accepted")
	}
}

func TestProjectReviewCoverageSeparatesBackendAndDatabase(t *testing.T) {
	f := newGitFixture(t)
	profile := workspace.ReviewProfile{Kind: "web", Areas: []string{"frontend", "backend", "database"}, Requirements: []string{"frontend-static", "backend-api", "database-integration", "database-migrations", "database-consistency", "database-recovery"}, Stages: []string{}}
	if err := f.service.SaveReviewProfile(f.repo, review(t, f).ConfigVersion, profile); err != nil {
		t.Fatal(err)
	}
	record := func(input workspace.ReviewCheckInput) {
		t.Helper()
		if _, err := f.service.RecordReviewCheck(f.repo, review(t, f).Version, input); err != nil {
			t.Fatal(err)
		}
	}
	record(workspace.ReviewCheckInput{Command: "frontend check", Scope: "frontend only", Status: "success", Evidence: "external frontend log", Coverage: []workspace.ReviewCoverage{{Area: "frontend", Method: "static"}}})
	record(workspace.ReviewCheckInput{Command: "SQL lint", Scope: "SQL syntax only", Status: "success", Evidence: "external SQL log", Coverage: []workspace.ReviewCoverage{{Area: "database", Method: "static"}}, Environment: "local", Target: "fixture-db"})
	r := review(t, f)
	for _, item := range r.Coverage {
		want := "pending"
		if item.Requirement.ID == "frontend-static" {
			want = "recorded"
		}
		if item.Status != want {
			t.Fatalf("%s incorrectly covered: %+v", item.Requirement.ID, item)
		}
	}
	input := workspace.ReviewCheckInput{Command: "database checks", Scope: "fixture database", Status: "success", Evidence: "external integration log", Coverage: []workspace.ReviewCoverage{{Area: "database", Method: "integration"}}}
	if _, err := f.service.RecordReviewCheck(f.repo, r.Version, input); err == nil {
		t.Fatal("database evidence without environment accepted")
	}
	input.Environment, input.Target = "local", "postgres://secret@host/db"
	if _, err := f.service.RecordReviewCheck(f.repo, r.Version, input); err == nil {
		t.Fatal("connection string accepted as target alias")
	}
	input.Target = "fixture-db"
	record(input)
	r = review(t, f)
	if r.Coverage[2].Status != "recorded" || r.Coverage[2].Environment != "local" || r.Coverage[2].Target != "fixture-db" {
		t.Fatalf("database assertion: %+v", r.Coverage)
	}
	// A new failure supersedes a success, even when recorded within the same second.
	input.Status = "failure"
	record(input)
	r = review(t, f)
	if r.Coverage[2].Status != "failed" {
		t.Fatalf("new failure hidden: %+v", r.Coverage[2])
	}
	for _, item := range r.Coverage[3:] {
		if item.Status != "pending" {
			t.Fatal("database connection treated as migration/recovery proof")
		}
	}
	reviewWrite(t, filepath.Join(f.repo, "changed.sql"), "changed after checks")
	r = review(t, f)
	if r.Coverage[2].Status != "stale" {
		t.Fatal("changed contents still verified")
	}
}

func TestProjectReviewPipelineCoverageIsExplicit(t *testing.T) {
	f := newGitFixture(t)
	reviewWrite(t, filepath.Join(f.repo, ".repsync.json"), `{"reviewProfile":{"kind":"backend","areas":["backend","database"],"requirements":["backend-static","database-integration"],"stages":[]},"pipelines":[{"id":"ci","name":"Static fixture","kind":"ci","steps":[["/bin/echo","fixture only"]],"coverage":[{"area":"backend","method":"static"}],"reviewEnvironment":"local","reviewTarget":"fixture-api"}]}`)
	config, err := f.service.Pipelines(f.repo)
	if err != nil {
		t.Fatal(err)
	}
	run, err := f.service.StartPipeline(f.repo, "ci", config.Version)
	if err != nil {
		t.Fatal(err)
	}
	if result := waitPipeline(t, f.service, f.repo, run.ID); result.Run.Status != "success" {
		t.Fatal("fixture failed")
	}
	r := review(t, f)
	if r.Coverage[0].Status != "passed" || r.Coverage[0].Source != "pipeline" || r.Coverage[0].Target != "fixture-api" {
		t.Fatalf("pipeline metadata lost: %+v", r.Coverage)
	}
	if r.Coverage[1].Status != "pending" {
		t.Fatal("passing pipeline treated as database coverage")
	}
}
