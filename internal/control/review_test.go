//go:build integration

package control_test

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"repsync/internal/control"
	"repsync/internal/workspace"
)

func TestControlManualReviewAndHandover(t *testing.T) {
	root := t.TempDir()
	repo := filepath.Join(root, "repo")
	if err := os.Mkdir(repo, 0700); err != nil {
		t.Fatal(err)
	}
	git := func(args ...string) {
		t.Helper()
		cmd := exec.Command("git", append([]string{"-C", repo}, args...)...)
		cmd.Env = append(os.Environ(), "GIT_CONFIG_GLOBAL=/dev/null", "GIT_CONFIG_SYSTEM=/dev/null", "GIT_AUTHOR_NAME=Fixture", "GIT_AUTHOR_EMAIL=fixture@example.invalid", "GIT_COMMITTER_NAME=Fixture", "GIT_COMMITTER_EMAIL=fixture@example.invalid")
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v %s", args, err, out)
		}
	}
	git("init", "-b", "main")
	if err := os.WriteFile(filepath.Join(repo, "note.md"), []byte("original"), 0600); err != nil {
		t.Fatal(err)
	}
	git("add", ".")
	git("commit", "-m", "initial")
	service := workspace.NewService(filepath.Join(root, "state", "projects.json"))
	p, err := service.Add(repo)
	if err != nil {
		t.Fatal(err)
	}
	socket := shortSocket(t)
	server, err := control.Listen(socket, service)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { service.Shutdown(); server.Close() })
	getReview := func() workspace.ProjectReview {
		t.Helper()
		response, code := cli(t, socket, "review", p.Path)
		var report workspace.ProjectReview
		if err := json.Unmarshal(response.Data, &report); err != nil || code != 0 {
			t.Fatalf("review: %+v %v", response, err)
		}
		return report
	}
	report := getReview()
	profileFile := filepath.Join(root, "profile.json")
	if err := os.WriteFile(profileFile, []byte(`{"kind":"backend","areas":["backend","database"],"requirements":["backend-api","database-integration","database-migrations","database-recovery"],"stages":["push","deploy"]}`), 0600); err != nil {
		t.Fatal(err)
	}
	if response, code := cli(t, socket, "review-profile", p.Path, report.ConfigVersion, profileFile); code != 0 || !response.OK {
		t.Fatalf("profile CLI: %+v", response)
	}
	report = getReview()
	if !report.ProfileConfigured || len(report.Coverage) != 4 {
		t.Fatal("CLI profile missing")
	}
	issueFile := filepath.Join(root, "issue.json")
	issue := report.Issues[0]
	data, _ := json.Marshal(map[string]string{"key": issue.Key, "state": "retained", "reason": "intentional pending item"})
	if err := os.WriteFile(issueFile, data, 0600); err != nil {
		t.Fatal(err)
	}
	if response, code := cli(t, socket, "review-issue", p.Path, report.Version, issueFile); code != 0 || !response.OK {
		t.Fatalf("issue CLI: %+v", response)
	}
	report = getReview()
	if len(report.Acknowledged) != 1 {
		t.Fatal("CLI acknowledgement missing")
	}
	contextFile := filepath.Join(root, "context.json")
	if err := os.WriteFile(contextFile, []byte(`{"title":"Database review","owner":"CLI session","areas":["database"],"paths":["migrations"],"environment":"local","target":"fixture-db"}`), 0600); err != nil {
		t.Fatal(err)
	}
	if response, code := cli(t, socket, "review-task-context", p.Path, report.Version, contextFile); code != 0 || !response.OK {
		t.Fatalf("context CLI: %+v", response)
	}
	report = getReview()
	if len(report.Tasks) != 1 || report.Tasks[0].Owner != "CLI session" || report.Tasks[0].Target != "fixture-db" {
		t.Fatal("CLI task context missing")
	}
	response, code := cli(t, socket, "review-task-start", p.Path, report.Version, "CLI task")
	var task workspace.ReviewTask
	if err := json.Unmarshal(response.Data, &task); err != nil || code != 0 {
		t.Fatalf("start task: %+v %v", response, err)
	}
	input := filepath.Join(root, "check.json")
	if err := os.WriteFile(input, []byte(`{"command":"npm run check","status":"success","scope":"frontend","evidence":"external log"}`), 0600); err != nil {
		t.Fatal(err)
	}
	report = getReview()
	response, code = cli(t, socket, "review-check", p.Path, report.Version, input)
	if code != 0 || !response.OK {
		t.Fatalf("record check: %+v", response)
	}
	if response, code := cli(t, socket, "review-evidence", p.Path, "stale", input); code != 2 || response.OK {
		t.Fatal("invalid input was accepted")
	}
	report = getReview()
	if len(report.Checks) != 1 || report.Checks[0].Source != "external" {
		t.Fatal("external provenance missing")
	}
	input = filepath.Join(root, "finish.json")
	if err := os.WriteFile(input, []byte(`{"summary":"implemented","checks":"recorded","blockers":"not deployed"}`), 0600); err != nil {
		t.Fatal(err)
	}
	response, code = cli(t, socket, "review-task-finish", p.Path, task.ID, report.Version, input)
	if code != 0 || !response.OK {
		t.Fatalf("finish: %+v", response)
	}
	response, code = cli(t, socket, "review-handover", p.Path, task.ID)
	var handover string
	if err := json.Unmarshal(response.Data, &handover); err != nil || code != 0 || !strings.Contains(handover, "not deployed") {
		t.Fatalf("handover: %+v %v", response, err)
	}
}
