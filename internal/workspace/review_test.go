//go:build integration

package workspace_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"repsync/internal/workspace"
)

func review(t *testing.T, f *gitFixture) workspace.ProjectReview {
	t.Helper()
	r, err := f.service.ReviewProject(f.repo)
	if err != nil {
		t.Fatal(err)
	}
	return r
}
func reviewWrite(t *testing.T, path, text string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(text), 0600); err != nil {
		t.Fatal(err)
	}
}

func TestProjectReviewGitFactsChecksAndDelivery(t *testing.T) {
	f := newGitFixture(t)
	remote := filepath.Join(f.root, "remote.git")
	f.run("init", "--bare", remote)
	f.run("remote", "add", "origin", remote)
	f.run("push", "-u", "origin", "main")
	file := filepath.Join(f.repo, ".gitignore")
	reviewWrite(t, file, "cache/\nfirst\n")
	f.run("add", ".gitignore")
	f.run("commit", "-m", "local only")
	reviewWrite(t, file, "cache/\nstaged\n")
	f.run("add", ".gitignore")
	reviewWrite(t, file, "cache/\nunstaged\n")
	reviewWrite(t, filepath.Join(f.repo, "new notes.md"), "preserve")
	r := review(t, f)
	if !r.Snapshot.UpstreamKnown || r.Snapshot.Ahead != 1 || r.Snapshot.Behind != 0 || r.Snapshot.Staged != 1 || r.Snapshot.Unstaged != 1 || r.Snapshot.Untracked != 1 {
		t.Fatalf("facts: %+v", r.Snapshot)
	}
	for _, stage := range r.Delivery {
		if stage.Stage == "deploy" && stage.Status != "pending" && stage.Status != "unknown" {
			t.Fatal("invented deployment")
		}
	}
	_, err := f.service.RecordReviewCheck(f.repo, r.Version, workspace.ReviewCheckInput{Command: "go vet ./...", Scope: "Go packages", Status: "success", Evidence: "external log"})
	if err != nil {
		t.Fatal(err)
	}
	r = review(t, f)
	if len(r.Checks) != 1 || r.Checks[0].Current != "current" || r.Checks[0].Source != "external" {
		t.Fatalf("check: %+v", r.Checks)
	}
	before := r.Snapshot.Fingerprint
	f.run("add", ".")
	f.run("commit", "-m", "save reviewed contents")
	r = review(t, f)
	if r.Snapshot.Fingerprint != before || r.Checks[0].Current != "current" {
		t.Fatal("staging or commit invalidated unchanged file content")
	}
	if err := os.Remove(filepath.Join(f.repo, "new notes.md")); err != nil {
		t.Fatal(err)
	}
	deleted := review(t, f).Snapshot.Fingerprint
	f.run("add", ".")
	f.run("commit", "-m", "commit deletion")
	if review(t, f).Snapshot.Fingerprint != deleted {
		t.Fatal("committing a deletion invalidated unchanged contents")
	}
	r = review(t, f)
	_, err = f.service.RecordReviewEvidence(f.repo, r.Version, workspace.ReviewEvidenceInput{Stage: "deploy", Status: "success", Note: "confirmed preview deployment commit and page"})
	if err != nil {
		t.Fatal(err)
	}
	r = review(t, f)
	for _, stage := range r.Delivery {
		if stage.Stage == "deploy" && stage.Status != "recorded" {
			t.Fatalf("manual delivery missing: %+v", stage)
		}
	}
	stale := r.Version
	reviewWrite(t, file, "changed after check\n")
	if _, err := f.service.RecordReviewCheck(f.repo, stale, workspace.ReviewCheckInput{Command: "old", Scope: "all", Status: "success", Evidence: "old"}); err == nil {
		t.Fatal("stale report accepted")
	}
	r = review(t, f)
	if r.Checks[0].Current != "stale" {
		t.Fatal("changed content still current")
	}
	for _, stage := range r.Delivery {
		if stage.Stage == "deploy" && stage.Status != "stale" {
			t.Fatal("old deployment record treated as current")
		}
	}
}

func TestProjectReviewResourcesAreDeclaredAndSecretFree(t *testing.T) {
	f := newGitFixture(t)
	t.Setenv("REPSYNC_REVIEW_SECRET", "private-secret-must-not-be-returned")
	config := filepath.Join(f.repo, ".repsync.json")
	marker := filepath.Join(f.repo, "must-not-run")
	reviewWrite(t, config, `{"services":[],"pipelines":[{"id":"ci","name":"CI","kind":"ci","steps":[["touch","must-not-run"]]}],"extension":{"keep":true}}`)
	if err := os.Chmod(config, 0640); err != nil {
		t.Fatal(err)
	}
	reviewWrite(t, filepath.Join(f.repo, ".gitignore"), "cache/\n.env.local\n")
	reviewWrite(t, filepath.Join(f.repo, ".env.local"), "PASSWORD=private-file-content")
	r := review(t, f)
	definitions := []workspace.ReviewResource{{ID: "env", Name: "Environment", Kind: "environment", Target: "REPSYNC_REVIEW_SECRET"}, {ID: "file", Name: "Env file", Kind: "file", Target: ".env.local"}, {ID: "database", Name: "Database backup", Kind: "manual", Guide: "Follow project restore guide"}}
	if err := f.service.SaveReviewResources(f.repo, r.ConfigVersion, definitions); err != nil {
		t.Fatal(err)
	}
	if err := f.service.SaveReviewResources(f.repo, r.ConfigVersion, definitions); err == nil {
		t.Fatal("stale configuration overwrote file")
	}
	r = review(t, f)
	if len(r.Resources) != 3 || r.Resources[0].State.Observed != "set" || r.Resources[1].State.Tracking != "ignored" || r.Resources[2].Status != "pending" {
		t.Fatalf("resource facts: %+v", r.Resources)
	}
	data, err := json.Marshal(r)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(data), "private-secret") || strings.Contains(string(data), "private-file-content") {
		t.Fatal("exposed secret contents")
	}
	if _, err := os.Stat(marker); !os.IsNotExist(err) {
		t.Fatal("review executed project command")
	}
	data, err = os.ReadFile(config)
	if err != nil || !strings.Contains(string(data), `"extension"`) || !strings.Contains(string(data), `"pipelines"`) {
		t.Fatal("lost config sections")
	}
	info, err := os.Stat(config)
	if err != nil || info.Mode().Perm() != 0640 {
		t.Fatal("changed permissions")
	}
	_, err = f.service.RecordReviewEvidence(f.repo, r.Version, workspace.ReviewEvidenceInput{Stage: "resource", Target: "file", Status: "success", Note: "backup file checked"})
	if err != nil {
		t.Fatal(err)
	}
	r = review(t, f)
	if r.Resources[1].Status != "success" {
		t.Fatal("lost resource confirmation")
	}
	reviewWrite(t, filepath.Join(f.repo, ".env.local"), "PASSWORD=changed-private-content-longer")
	r = review(t, f)
	if r.Resources[1].Status != "stale" {
		t.Fatal("changed ignored resource still confirmed")
	}
	if err := os.Remove(filepath.Join(f.repo, ".env.local")); err != nil {
		t.Fatal(err)
	}
	r = review(t, f)
	if _, err := f.service.RecordReviewEvidence(f.repo, r.Version, workspace.ReviewEvidenceInput{Stage: "resource", Target: "file", Status: "success", Note: "not actually present"}); err == nil {
		t.Fatal("missing resource was confirmed as passing")
	}
	if err := f.service.SaveReviewResources(f.repo, r.ConfigVersion, []workspace.ReviewResource{{ID: "escape", Name: "Outside", Kind: "file", Target: "../outside"}}); err == nil {
		t.Fatal("accepted outside resource")
	}
}

func TestProjectReviewTaskHandoverPreservesBeforeAndAfter(t *testing.T) {
	f := newGitFixture(t)
	file := filepath.Join(f.repo, ".gitignore")
	reviewWrite(t, file, "existing stash\n")
	f.run("stash", "push", "-m", "before task")
	r := review(t, f)
	task, err := f.service.StartReviewTask(f.repo, r.Version, "Implement feature")
	if err != nil {
		t.Fatal(err)
	}
	reviewWrite(t, file, "new stash\n")
	f.run("stash", "push", "-m", "during task")
	reviewWrite(t, filepath.Join(f.repo, "draft.md"), "untracked draft")
	r = review(t, f)
	finished, err := f.service.FinishReviewTask(f.repo, task.ID, r.Version, workspace.ReviewFinishInput{Summary: "feature added", Checks: "targeted checks recorded", Blockers: "review draft and stash"})
	if err != nil {
		t.Fatal(err)
	}
	if len(finished.NewStashes) != 1 || !strings.Contains(finished.NewStashes[0].Subject, "during task") || len(finished.NewUntracked) != 1 || finished.NewUntracked[0] != "draft.md" {
		t.Fatalf("wrong task delta: %+v", finished)
	}
	fresh := workspace.NewService(f.config)
	text, err := fresh.ReviewHandover(f.repo, task.ID)
	if err != nil || !strings.Contains(text, "feature added") || !strings.Contains(text, "review draft and stash") {
		t.Fatalf("handover persistence: %s %v", text, err)
	}
	if f.run("stash", "list", "--format=%H") == "" {
		t.Fatal("task records modified stash")
	}
	if _, err := f.service.FinishReviewTask(f.repo, task.ID, review(t, f).Version, workspace.ReviewFinishInput{Summary: "again"}); err == nil {
		t.Fatal("closed task overwritten")
	}
}

func TestProjectReviewPipelineTracksActualContents(t *testing.T) {
	f := newGitFixture(t)
	writePipeline(t, f.repo, [][]string{{"/bin/sh", "-c", "echo CHECKED"}}, 10)
	config, err := f.service.Pipelines(f.repo)
	if err != nil {
		t.Fatal(err)
	}
	run, err := f.service.StartPipeline(f.repo, "ci", config.Version)
	if err != nil {
		t.Fatal(err)
	}
	output := waitPipeline(t, f.service, f.repo, run.ID)
	if output.Run.Status != "success" || output.Run.Fingerprint == "" || output.Run.EndFingerprint != output.Run.Fingerprint {
		t.Fatalf("pipeline capture: %+v", output.Run)
	}
	r := review(t, f)
	if len(r.Checks) != 1 || r.Checks[0].Source != "pipeline" || r.Checks[0].Current != "current" {
		t.Fatalf("pipeline review: %+v", r.Checks)
	}
	reviewWrite(t, filepath.Join(f.repo, "new.md"), "after pipeline")
	if review(t, f).Checks[0].Current != "stale" {
		t.Fatal("untracked changes did not invalidate comparison")
	}
	writePipeline(t, f.repo, [][]string{{"/bin/sh", "-c", "echo changed > during.md"}}, 10)
	config, err = f.service.Pipelines(f.repo)
	if err != nil {
		t.Fatal(err)
	}
	run, err = f.service.StartPipeline(f.repo, "ci", config.Version)
	if err != nil {
		t.Fatal(err)
	}
	output = waitPipeline(t, f.service, f.repo, run.ID)
	if output.Run.Status != "success" || output.Run.Fingerprint == output.Run.EndFingerprint {
		t.Fatal("changes during a passing command were not captured")
	}
	if review(t, f).Checks[0].Current != "stale" {
		t.Fatal("mutating check marked current")
	}
}
