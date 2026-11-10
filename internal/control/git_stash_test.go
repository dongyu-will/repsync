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

func TestControlStashCLIUsesReviewedObject(t *testing.T) {
	root := t.TempDir()
	repo := filepath.Join(root, "repo")
	if err := os.Mkdir(repo, 0700); err != nil {
		t.Fatal(err)
	}
	git := func(args ...string) string {
		t.Helper()
		cmd := exec.Command("git", append([]string{"-C", repo}, args...)...)
		cmd.Env = append(os.Environ(), "GIT_CONFIG_GLOBAL=/dev/null", "GIT_CONFIG_SYSTEM=/dev/null", "GIT_AUTHOR_NAME=Fixture", "GIT_AUTHOR_EMAIL=fixture@example.invalid", "GIT_COMMITTER_NAME=Fixture", "GIT_COMMITTER_EMAIL=fixture@example.invalid")
		out, err := cmd.CombinedOutput()
		if err != nil {
			t.Fatalf("git %v: %v %s", args, err, out)
		}
		return strings.TrimSpace(string(out))
	}
	git("init", "-b", "main")
	file := filepath.Join(repo, "note.md")
	if err := os.WriteFile(file, []byte("original\n"), 0600); err != nil {
		t.Fatal(err)
	}
	git("add", "note.md")
	git("commit", "-m", "initial")
	if err := os.WriteFile(file, []byte("stash draft\n"), 0600); err != nil {
		t.Fatal(err)
	}
	git("stash", "push", "-m", "before push")
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
	response, code := cli(t, socket, "git-stashes", p.Path)
	if code != 0 || !response.OK {
		t.Fatalf("list: %+v", response)
	}
	var list workspace.GitStashList
	if err := json.Unmarshal(response.Data, &list); err != nil || len(list.Entries) != 1 {
		t.Fatalf("list data: %+v %v", list, err)
	}
	oid := list.Entries[0].OID
	response, code = cli(t, socket, "git-stash-show", p.Path, oid)
	var preview workspace.GitStashPreview
	if err := json.Unmarshal(response.Data, &preview); err != nil || code != 0 || !strings.Contains(preview.Patch, "stash draft") {
		t.Fatalf("show: %+v %v", response, err)
	}
	if response, code := cli(t, socket, "git-stash-apply", p.Path, oid, "stale"); code != 1 || response.OK {
		t.Fatalf("stale version accepted: %+v", response)
	}
	response, code = cli(t, socket, "git-stash-apply", p.Path, oid, preview.Version)
	var applied workspace.GitStashApplyResult
	if err := json.Unmarshal(response.Data, &applied); err != nil || code != 0 || !applied.Applied {
		t.Fatalf("apply: %+v %v", response, err)
	}
	if got, err := os.ReadFile(file); err != nil || string(got) != "stash draft\n" {
		t.Fatal("CLI did not restore files")
	}
	if git("rev-parse", "refs/stash") != oid {
		t.Fatal("CLI dropped stash")
	}
	if response, code := cli(t, socket, "git-stash-apply", p.Path, oid, preview.Version); code != 1 || response.OK {
		t.Fatalf("dirty tree accepted: %+v", response)
	}
}
