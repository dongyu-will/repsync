//go:build integration

package control_test

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"repsync/internal/control"
	"repsync/internal/workspace"
)

func TestControlGitPolicyPreservesVersionAndProjectFiles(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	repo := filepath.Join(root, "repo")
	if err := os.Mkdir(repo, 0700); err != nil {
		t.Fatal(err)
	}
	for _, args := range [][]string{{"init", "-b", "main"}, {"-c", "core.hooksPath=/dev/null", "commit", "--allow-empty", "-m", "initial"}} {
		cmd := exec.Command("git", append([]string{"-C", repo}, args...)...)
		cmd.Env = append(os.Environ(), "GIT_CONFIG_GLOBAL=/dev/null", "GIT_CONFIG_SYSTEM=/dev/null", "GIT_AUTHOR_NAME=Fixture", "GIT_AUTHOR_EMAIL=fixture@example.invalid", "GIT_COMMITTER_NAME=Fixture", "GIT_COMMITTER_EMAIL=fixture@example.invalid")
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v %s", args, err, out)
		}
	}
	const config = "{\"services\":[]}\n"
	if err := os.WriteFile(filepath.Join(repo, ".repsync.json"), []byte(config), 0600); err != nil {
		t.Fatal(err)
	}
	service := workspace.NewService(filepath.Join(root, "state", "projects.json"))
	project, err := service.Add(repo)
	if err != nil {
		t.Fatal(err)
	}
	socket := shortSocket(t)
	server, err := control.Listen(socket, service)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { service.Shutdown(); server.Close() })
	read := func() workspace.GitOverview {
		t.Helper()
		response, code := cli(t, socket, "git-workspace", project.Path)
		var view workspace.GitOverview
		if err := json.Unmarshal(response.Data, &view); err != nil || code != 0 {
			t.Fatalf("read policy: %+v %v", response, err)
		}
		return view
	}
	before := read()
	file := filepath.Join(root, "policy.json")
	write := func(content string) {
		t.Helper()
		if err := os.WriteFile(file, []byte(content), 0600); err != nil {
			t.Fatal(err)
		}
	}
	write(`{"enabled":false,"baseBranch":"main","retentionDays":14}`)
	if response, code := cli(t, socket, "git-policy", project.Path, before.PolicyVersion, file); code != 0 || !response.OK {
		t.Fatalf("save policy: %+v", response)
	}
	after := read()
	if after.Policy.RetentionDays != 14 || after.Policy.Enabled || after.PolicyVersion == before.PolicyVersion || len(after.History) != len(before.History) {
		t.Fatalf("policy was not saved or cleanup unexpectedly ran: %+v", after)
	}
	for _, test := range []struct {
		name, content, version string
		code                   int
	}{
		{"stale version", `{"enabled":false,"baseBranch":"main","retentionDays":21}`, before.PolicyVersion, 1},
		{"invalid retention", `{"enabled":false,"baseBranch":"main","retentionDays":0}`, after.PolicyVersion, 1},
		{"missing branch", `{"enabled":false,"baseBranch":"absent","retentionDays":14}`, after.PolicyVersion, 1},
		{"unknown field", `{"enabled":false,"baseBranch":"main","retentionDays":14,"extra":true}`, after.PolicyVersion, 2},
	} {
		t.Run(test.name, func(t *testing.T) {
			write(test.content)
			if response, code := cli(t, socket, "git-policy", project.Path, test.version, file); code != test.code || response.OK {
				t.Fatalf("invalid policy accepted: %+v, code %d", response, code)
			}
			if current := read(); current.PolicyVersion != after.PolicyVersion {
				t.Fatal("failed request changed the policy")
			}
		})
	}
	if response, code := cli(t, socket, "git-policy", project.Path); code != 2 || response.OK {
		t.Fatalf("missing arguments accepted: %+v", response)
	}
	if content, err := os.ReadFile(filepath.Join(repo, ".repsync.json")); err != nil || string(content) != config {
		t.Fatal("policy command changed project configuration")
	}
}
