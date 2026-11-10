//go:build integration

package workspace_test

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"repsync/internal/workspace"
)

type gitFixture struct {
	t                  *testing.T
	root, repo, config string
	service            *workspace.Service
}

func newGitFixture(t *testing.T) *gitFixture {
	t.Helper()
	f := &gitFixture{t: t, root: t.TempDir()}
	var err error
	f.root, err = filepath.EvalSymlinks(f.root)
	if err != nil {
		t.Fatal(err)
	}
	f.repo = filepath.Join(f.root, "repo with spaces")
	f.config = filepath.Join(f.root, "state", "projects.json")
	if err := os.Mkdir(f.repo, 0700); err != nil {
		t.Fatal(err)
	}
	f.run("init", "-b", "main")
	if err := os.WriteFile(filepath.Join(f.repo, ".gitignore"), []byte("cache/\n"), 0600); err != nil {
		t.Fatal(err)
	}
	f.run("add", ".gitignore")
	f.run("commit", "-m", "initial")
	f.service = workspace.NewService(f.config)
	if _, err := f.service.Add(f.repo); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(f.service.Shutdown)
	return f
}

func (f *gitFixture) run(args ...string) string {
	f.t.Helper()
	return f.at(f.repo, args...)
}
func (f *gitFixture) at(path string, args ...string) string {
	f.t.Helper()
	cmd := exec.Command("git", append([]string{"-C", path}, args...)...)
	cmd.Env = append(os.Environ(), "GIT_CONFIG_GLOBAL=/dev/null", "GIT_CONFIG_SYSTEM=/dev/null", "GIT_AUTHOR_NAME=Fixture", "GIT_AUTHOR_EMAIL=fixture@example.invalid", "GIT_COMMITTER_NAME=Fixture", "GIT_COMMITTER_EMAIL=fixture@example.invalid", "GIT_AUTHOR_DATE=2020-01-01T00:00:00Z", "GIT_COMMITTER_DATE=2020-01-01T00:00:00Z")
	out, err := cmd.CombinedOutput()
	if err != nil {
		f.t.Fatalf("git %v: %v %s", args, err, out)
	}
	return strings.TrimSpace(string(out))
}
func (f *gitFixture) tree(branch string) string {
	f.t.Helper()
	path := filepath.Join(f.root, branch+" tree")
	f.run("worktree", "add", "-b", branch, path)
	return path
}
func (f *gitFixture) view() workspace.GitOverview {
	f.t.Helper()
	v, err := f.service.GitWorkspace(f.repo)
	if err != nil {
		f.t.Fatal(err)
	}
	return v
}
func (f *gitFixture) age() {
	f.t.Helper()
	f.view()
	files, err := filepath.Glob(filepath.Join(filepath.Dir(f.config), "git", "*", "state.json"))
	if err != nil || len(files) != 1 {
		f.t.Fatalf("state: %v %v", files, err)
	}
	data, err := os.ReadFile(files[0])
	if err != nil {
		f.t.Fatal(err)
	}
	var state map[string]json.RawMessage
	if err := json.Unmarshal(data, &state); err != nil {
		f.t.Fatal(err)
	}
	var seen map[string]map[string]any
	if err := json.Unmarshal(state["seen"], &seen); err != nil {
		f.t.Fatal(err)
	}
	for _, entry := range seen {
		entry["since"] = time.Now().Add(-30 * 24 * time.Hour).UTC().Format(time.RFC3339)
	}
	state["seen"], _ = json.Marshal(seen)
	data, _ = json.Marshal(state)
	if err := os.WriteFile(files[0], data, 0600); err != nil {
		f.t.Fatal(err)
	}
}

func TestGitWorkspaceCleanupAndRestore(t *testing.T) {
	f := newGitFixture(t)
	clean := f.tree("topic-clean")
	f.run("config", "branch.topic-clean.description", "Preserve branch description")
	dirty := f.tree("topic-dirty")
	tracked := f.tree("topic-tracked")
	if err := os.WriteFile(filepath.Join(tracked, ".gitignore"), []byte("cache/\nchanged\n"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dirty, "keep.txt"), []byte("untracked"), 0600); err != nil {
		t.Fatal(err)
	}
	ignored := f.tree("topic-ignored")
	if err := os.Mkdir(filepath.Join(ignored, "cache"), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(ignored, "cache", "keep"), []byte("ignored"), 0600); err != nil {
		t.Fatal(err)
	}
	locked := f.tree("topic-locked")
	f.run("worktree", "lock", locked)
	registered := f.tree("topic-registered")
	if _, err := f.service.Add(registered); err != nil {
		t.Fatal(err)
	}
	unmerged := f.tree("topic-unmerged")
	if err := os.WriteFile(filepath.Join(unmerged, "new.txt"), []byte("unmerged"), 0600); err != nil {
		t.Fatal(err)
	}
	f.at(unmerged, "add", "new.txt")
	f.at(unmerged, "commit", "-m", "not merged")
	f.run("branch", "topic-unused")
	f.age()
	v := f.view()
	for _, w := range v.Worktrees {
		if w.Eligible != (w.Path == clean) {
			t.Fatalf("eligibility: %+v", w)
		}
	}
	r, err := f.service.CleanupGitWorkspace(f.repo, v.Version)
	if err != nil {
		t.Fatal(err)
	}
	if len(r.Items) != 3 {
		t.Fatalf("unexpected candidates: %+v", r)
	}
	for _, item := range r.Items {
		if item.Status != "removed" {
			t.Fatalf("cleanup failed: %+v", item)
		}
	}
	if _, err := os.Stat(clean); !os.IsNotExist(err) {
		t.Fatalf("clean tree still exists: %v", err)
	}
	for _, path := range []string{dirty, tracked, ignored, locked, registered, unmerged} {
		if _, err := os.Stat(path); err != nil {
			t.Fatal(err)
		}
	}
	if f.run("show", "main:.gitignore") != "cache/" {
		t.Fatal("main changed")
	}
	f.service = workspace.NewService(f.config) // persisted recovery survives a restart
	if _, err := f.service.RestoreGitCleanup(f.repo, r.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(clean, ".gitignore")); err != nil {
		t.Fatal(err)
	}
	if f.run("config", "--get", "branch.topic-clean.description") != "Preserve branch description" {
		t.Fatal("lost branch settings")
	}
	if f.run("rev-parse", "topic-unused") != f.run("rev-parse", "main") {
		t.Fatal("branch not restored")
	}
	for _, b := range f.view().Branches {
		if b.Eligible {
			t.Fatalf("restored branch immediately eligible: %+v", b)
		}
	}
}

func TestGitWorkspaceStalePreviewAndActiveProcess(t *testing.T) {
	f := newGitFixture(t)
	tree := f.tree("topic-active")
	f.age()
	v := f.view()
	if !slices.ContainsFunc(v.Worktrees, func(w workspace.GitWorktree) bool { return w.Path == tree && w.Eligible }) {
		t.Fatalf("not eligible: %+v", v)
	}
	cmd := exec.Command("sleep", "30")
	cmd.Dir = tree
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = cmd.Process.Kill(); _ = cmd.Wait() })
	if _, err := f.service.CleanupGitWorkspace(f.repo, v.Version); err == nil {
		t.Fatal("stale preview deleted active tree")
	}
	for _, w := range f.view().Worktrees {
		if w.Path == tree && (w.Eligible || !strings.Contains(strings.Join(w.Reasons, ""), "进程")) {
			t.Fatalf("active tree not protected: %+v", w)
		}
	}
	_ = cmd.Process.Kill()
	_ = cmd.Wait()
	v = f.view()
	if err := os.WriteFile(filepath.Join(tree, "late.txt"), []byte("preserve"), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := f.service.CleanupGitWorkspace(f.repo, v.Version); err == nil {
		t.Fatal("stale preview deleted changed tree")
	}
	if got, err := os.ReadFile(filepath.Join(tree, "late.txt")); err != nil || string(got) != "preserve" {
		t.Fatal("lost file")
	}
}

func TestGitWorkspaceAutomaticCleanupOptInAndProtection(t *testing.T) {
	f := newGitFixture(t)
	tree := f.tree("topic-auto")
	f.run("branch", "release/stable")
	f.age()
	if records, err := f.service.RunAutomaticGitCleanup(); err != nil || len(records) != 0 {
		t.Fatalf("auto ran without opt in: %v %v", records, err)
	}
	v := f.view()
	policy := v.Policy
	policy.Enabled = true
	if err := f.service.SaveGitCleanupPolicy(f.repo, v.PolicyVersion, policy); err != nil {
		t.Fatal(err)
	}
	if err := f.service.SaveGitCleanupPolicy(f.repo, v.PolicyVersion, policy); err == nil {
		t.Fatal("stale policy accepted")
	}
	records, err := f.service.RunAutomaticGitCleanup()
	if err != nil || len(records) != 1 || !records[0].Automatic {
		t.Fatalf("auto: %+v %v", records, err)
	}
	if _, err := os.Stat(tree); !os.IsNotExist(err) {
		t.Fatal("automatic cleanup did not remove candidate")
	}
	f.run("rev-parse", "release/stable")
	if len(f.view().History) != 1 {
		t.Fatal("missing automatic history")
	}
	if _, err := f.service.GitWorkspace(filepath.Dir(f.repo)); err == nil {
		t.Fatal("unregistered path accepted")
	}
}

func TestGitWorkspaceRecentCheckoutAndCodexOwnership(t *testing.T) {
	f := newGitFixture(t)
	recent := f.tree("topic-recent")
	detached := filepath.Join(f.root, "detached")
	f.run("worktree", "add", "--detach", detached, "main")
	codex := filepath.Join(f.root, ".codex", "worktrees", "task", "repo")
	f.run("worktree", "add", "-b", "topic-codex", codex)
	f.age()
	cmd := exec.Command("git", "-C", recent, "reset", "--hard", "HEAD")
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("checkout: %v %s", err, out)
	}
	view := f.view()
	for _, w := range view.Worktrees {
		if w.Eligible {
			t.Fatalf("protected tree eligible: %+v", w)
		}
		if w.Path == recent && !strings.Contains(strings.Join(w.Reasons, ""), "未满保留期") {
			t.Fatalf("recent checkout not detected: %+v", w)
		}
		if w.Path == codex && !strings.Contains(strings.Join(w.Reasons, ""), "Codex") {
			t.Fatalf("Codex lifecycle not protected: %+v", w)
		}
	}
	if result, err := f.service.CleanupGitWorkspace(f.repo, view.Version); err != nil || len(result.Items) != 0 {
		t.Fatalf("unexpected removal: %+v %v", result, err)
	}
}

func TestGitWorkspaceRestoreDoesNotOverwriteAndTimerStops(t *testing.T) {
	f := newGitFixture(t)
	tree := f.tree("topic-restore")
	f.age()
	view := f.view()
	record, err := f.service.CleanupGitWorkspace(f.repo, view.Version)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(tree, 0700); err != nil {
		t.Fatal(err)
	}
	file := filepath.Join(tree, "keep.txt")
	if err := os.WriteFile(file, []byte("preserve"), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := f.service.RestoreGitCleanup(f.repo, record.ID); err == nil {
		t.Fatal("restored over an existing path")
	}
	if got, err := os.ReadFile(file); err != nil || string(got) != "preserve" {
		t.Fatal("existing content changed")
	}
	if err := os.Remove(file); err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(tree); err != nil {
		t.Fatal(err)
	}
	if _, err := f.service.RestoreGitCleanup(f.repo, record.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := f.service.RestoreGitCleanup(f.repo, record.ID); err == nil {
		t.Fatal("repeated restore accepted")
	}
	f.service.StartGitCleanup()
	f.service.StartGitCleanup()
	f.service.Shutdown()
}
