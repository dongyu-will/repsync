package workspace

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"time"
)

type GitCleanupPolicy struct {
	Enabled       bool   `json:"enabled"`
	BaseBranch    string `json:"baseBranch"`
	RetentionDays int    `json:"retentionDays"`
}

type GitBranch struct {
	Name      string   `json:"name"`
	Commit    string   `json:"commit"`
	Upstream  string   `json:"upstream"`
	UpdatedAt string   `json:"updatedAt"`
	Merged    bool     `json:"merged"`
	Worktrees []string `json:"worktrees"`
	Eligible  bool     `json:"eligible"`
	Reasons   []string `json:"reasons"`
}

type GitWorktree struct {
	Path        string   `json:"path"`
	Branch      string   `json:"branch"`
	Commit      string   `json:"commit"`
	Primary     bool     `json:"primary"`
	Locked      bool     `json:"locked"`
	Missing     bool     `json:"missing"`
	Dirty       bool     `json:"dirty"`
	StatusKnown bool     `json:"statusKnown"`
	Eligible    bool     `json:"eligible"`
	Reasons     []string `json:"reasons"`
}

type GitCleanupItem struct {
	Kind      string             `json:"kind"`
	Target    string             `json:"target"`
	Branch    string             `json:"branch"`
	Commit    string             `json:"commit"`
	BackupRef string             `json:"backupRef"`
	Status    string             `json:"status"`
	Error     string             `json:"error"`
	Upstream  string             `json:"upstream"`
	Config    []GitBranchSetting `json:"config"`
}

type GitBranchSetting struct {
	Key   string `json:"key"`
	Value string `json:"value"`
}

type GitCleanupRecord struct {
	ID         string           `json:"id"`
	CreatedAt  string           `json:"createdAt"`
	Automatic  bool             `json:"automatic"`
	RestoredAt string           `json:"restoredAt"`
	Items      []GitCleanupItem `json:"items"`
}

type GitOverview struct {
	Git           bool               `json:"git"`
	Worktrees     []GitWorktree      `json:"worktrees"`
	Branches      []GitBranch        `json:"branches"`
	Policy        GitCleanupPolicy   `json:"policy"`
	PolicyVersion string             `json:"policyVersion"`
	Version       string             `json:"version"`
	History       []GitCleanupRecord `json:"history"`
	Warnings      []string           `json:"warnings"`
	Stashes       GitStashList       `json:"stashes"`
	StashError    string             `json:"stashError"`
}

type gitObserved struct {
	Commit string    `json:"commit"`
	Since  time.Time `json:"since"`
}

type gitWorkspaceState struct {
	Policy GitCleanupPolicy       `json:"policy"`
	Seen   map[string]gitObserved `json:"seen"`
}

func gitDigest(value any) string {
	data, _ := json.Marshal(value)
	return fmt.Sprintf("%x", sha256.Sum256(data))
}

func (s *Service) gitState(path string) (string, gitWorkspaceState, error) {
	common, err := git(path, "rev-parse", "--path-format=absolute", "--git-common-dir")
	if err != nil {
		return "", gitWorkspaceState{}, err
	}
	canonical, err := filepath.EvalSymlinks(strings.TrimSpace(common))
	if err != nil {
		return "", gitWorkspaceState{}, err
	}
	dir := filepath.Join(filepath.Dir(s.path), "git", gitDigest(canonical))
	state := gitWorkspaceState{Policy: GitCleanupPolicy{RetentionDays: 7}, Seen: map[string]gitObserved{}}
	data, err := os.ReadFile(filepath.Join(dir, "state.json"))
	if err == nil {
		err = json.Unmarshal(data, &state)
	} else if errors.Is(err, os.ErrNotExist) {
		err = nil
		ref, _ := git(path, "symbolic-ref", "--short", "refs/remotes/origin/HEAD")
		candidates := []string{strings.TrimPrefix(strings.TrimSpace(ref), "origin/"), "main", "master"}
		for _, candidate := range candidates {
			if candidate == "" {
				continue
			}
			if _, e := git(path, "show-ref", "--verify", "refs/heads/"+candidate); e == nil {
				state.Policy.BaseBranch = candidate
				break
			}
		}
	}
	if state.Seen == nil {
		state.Seen = map[string]gitObserved{}
	}
	if err == nil && (state.Policy.RetentionDays < 1 || state.Policy.RetentionDays > 365) {
		err = errors.New("invalid retention period in cleanup settings; check stopped")
	}
	return dir, state, err
}

// GitWorkspace lists local refs and linked worktrees without fetching or switching branches.
func (s *Service) GitWorkspace(path string) (GitOverview, error) {
	s.gitMu.Lock()
	defer s.gitMu.Unlock()
	return s.gitOverview(path)
}

func (s *Service) gitOverview(path string) (GitOverview, error) {
	deadline := time.Now().Add(25 * time.Second)
	view := GitOverview{Worktrees: []GitWorktree{}, Branches: []GitBranch{}, History: []GitCleanupRecord{}, Warnings: []string{}}
	if _, err := s.registeredProject(path); err != nil {
		return view, err
	}
	if _, err := git(path, "rev-parse", "--git-dir"); err != nil {
		if info, statErr := os.Stat(path); statErr != nil || !info.IsDir() {
			return view, errors.New("project folder inaccessible")
		}
		return view, nil
	}
	view.Git = true
	stashes, stashErr := readGitStashes(path)
	view.Stashes = stashes
	if stashErr != nil {
		view.StashError = stashErr.Error()
	}
	dir, state, err := s.gitState(path)
	if err != nil {
		return view, err
	}
	view.Policy, view.PolicyVersion = state.Policy, gitDigest(state.Policy)
	base := "refs/heads/" + state.Policy.BaseBranch
	baseOID, baseErr := git(path, "rev-parse", "--verify", base+"^{commit}")
	if baseErr != nil {
		view.Warnings = append(view.Warnings, "Select a valid local base branch before cleanup")
	}
	raw, err := git(path, "worktree", "list", "--porcelain", "-z")
	if err != nil {
		return view, err
	}
	for _, block := range strings.Split(raw, "\x00\x00") {
		if len(view.Worktrees) >= 64 || time.Now().After(deadline) {
			return view, errors.New("worktree scan exceeded 64 entries or 25 seconds; no cleanup performed")
		}
		if block == "" {
			continue
		}
		w := GitWorktree{Primary: len(view.Worktrees) == 0, Reasons: []string{}}
		for _, field := range strings.Split(block, "\x00") {
			key, value, _ := strings.Cut(field, " ")
			switch key {
			case "worktree":
				w.Path = value
			case "HEAD":
				w.Commit = value
			case "branch":
				w.Branch = strings.TrimPrefix(value, "refs/heads/")
			case "locked":
				w.Locked = true
			case "prunable":
				w.Missing = true
			}
		}
		if w.Path == "" {
			return view, errors.New("could not fully parse worktree list")
		}
		if w.Primary {
			w.Reasons = append(w.Reasons, "Main worktree")
		}
		if w.Locked {
			w.Reasons = append(w.Reasons, "Locked")
		}
		if w.Missing {
			w.Reasons = append(w.Reasons, "Folder missing. Registration kept for repair.")
		}
		// Codex owns the lifecycle and active-chat state of its own worktrees.
		if strings.Contains(filepath.ToSlash(w.Path), "/.codex/worktrees/") {
			w.Reasons = append(w.Reasons, "Managed by Codex; archive or remove through its owning chat")
		}
		projects, e := s.List()
		if e != nil {
			return view, e
		}
		for _, p := range projects {
			if pathsOverlap(w.Path, p.Path) {
				w.Reasons = append(w.Reasons, "Registered or contains a registered project")
				break
			}
		}
		status, e := git(w.Path, "status", "--porcelain=v1", "--untracked-files=all", "--ignored=matching")
		w.StatusKnown, w.Dirty = e == nil, status != ""
		if e != nil {
			w.Reasons = append(w.Reasons, "Cannot read full file state")
		} else if w.Dirty {
			w.Reasons = append(w.Reasons, "Modified, untracked, or ignored files")
		}
		if w.Branch == "" {
			w.Reasons = append(w.Reasons, "Detached HEAD; manual action required")
		}
		if len(w.Reasons) == 0 {
			if e := worktreeIdle(w.Path); e != nil {
				w.Reasons = append(w.Reasons, e.Error())
			}
		}
		view.Worktrees = append(view.Worktrees, w)
	}
	raw, err = git(path, "for-each-ref", "--sort=refname", "--format=%(refname:strip=2)%00%(objectname)%00%(upstream:short)%00%(committerdate:iso8601-strict)", "refs/heads/")
	if err != nil || strings.Contains(raw, "[output exceeds 1 MiB") {
		return view, errors.New("could not read all local branches")
	}
	now := time.Now().UTC()
	seen := map[string]gitObserved{}
	for _, line := range strings.Split(strings.TrimSpace(raw), "\n") {
		if len(view.Branches) >= 256 || time.Now().After(deadline) {
			return view, errors.New("branch scan exceeded 256 entries or 25 seconds; no cleanup performed")
		}
		if line == "" {
			continue
		}
		fields := strings.Split(line, "\x00")
		if len(fields) != 4 {
			return view, errors.New("could not fully parse local branches")
		}
		b := GitBranch{Name: fields[0], Commit: fields[1], Upstream: fields[2], UpdatedAt: fields[3], Worktrees: []string{}, Reasons: []string{}}
		b.Merged = baseErr == nil && gitAncestor(path, b.Commit, base)
		if !b.Merged {
			b.Reasons = append(b.Reasons, "Not merged into the base branch")
		}
		if protectedGitBranch(b.Name, state.Policy.BaseBranch) {
			b.Reasons = append(b.Reasons, "Protected branch")
		}
		key := "branch:" + b.Name
		observed := state.Seen[key]
		if observed.Commit != b.Commit || observed.Since.IsZero() {
			observed = gitObserved{Commit: b.Commit, Since: now}
		}
		seen[key] = observed
		last := observed.Since
		changed, e := gitRefChanged(path, "refs/heads/"+b.Name)
		if e != nil {
			b.Reasons = append(b.Reasons, "Cannot confirm the branch's latest change time")
		} else if changed.After(last) {
			last = changed
		}
		if last.After(now.Add(-time.Duration(state.Policy.RetentionDays) * 24 * time.Hour)) {
			b.Reasons = append(b.Reasons, "Within retention (since first observation or latest change)")
		}
		for i := range view.Worktrees {
			w := &view.Worktrees[i]
			if w.Branch != b.Name {
				continue
			}
			b.Worktrees = append(b.Worktrees, w.Path)
			// Worktree checkout/creation may be newer than the branch's last commit.
			wk := "worktree:" + w.Path
			wo := state.Seen[wk]
			if wo.Commit != w.Commit || wo.Since.IsZero() {
				wo = gitObserved{Commit: w.Commit, Since: now}
			}
			seen[wk] = wo
			w.Reasons = append(w.Reasons, b.Reasons...)
			checkout, e := gitRefChanged(w.Path, "HEAD")
			if e != nil {
				w.Reasons = append(w.Reasons, "Cannot confirm the worktree's latest checkout time")
			} else if checkout.After(wo.Since) {
				wo.Since = checkout
			}
			if wo.Since.After(now.Add(-time.Duration(state.Policy.RetentionDays) * 24 * time.Hour)) {
				w.Reasons = append(w.Reasons, "Worktree is within retention")
			}
			w.Eligible = len(w.Reasons) == 0
			if !w.Eligible {
				b.Reasons = append(b.Reasons, "Linked worktree must be kept")
			}
		}
		b.Eligible = len(b.Reasons) == 0
		view.Branches = append(view.Branches, b)
	}
	stashProtection := ""
	if view.StashError != "" {
		stashProtection = "Cannot verify stash state; cleanup paused"
	} else if len(view.Stashes.Entries) > 0 {
		stashProtection = fmt.Sprintf("%d pending stashes; cleanup paused", len(view.Stashes.Entries))
	}
	if stashProtection != "" {
		view.Warnings = append(view.Warnings, stashProtection)
		for i := range view.Worktrees {
			view.Worktrees[i].Eligible = false
			view.Worktrees[i].Reasons = append(view.Worktrees[i].Reasons, stashProtection)
		}
		for i := range view.Branches {
			view.Branches[i].Eligible = false
			view.Branches[i].Reasons = append(view.Branches[i].Reasons, stashProtection)
		}
	}
	state.Seen = seen
	if err := saveJSON(filepath.Join(dir, "state.json"), state); err != nil {
		return view, err
	}
	entries, err := os.ReadDir(filepath.Join(dir, "history"))
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return view, err
	}
	slices.Reverse(entries)
	for _, entry := range entries {
		if len(view.History) >= 20 {
			break
		}
		var record GitCleanupRecord
		data, e := os.ReadFile(filepath.Join(dir, "history", entry.Name()))
		if e != nil || json.Unmarshal(data, &record) != nil {
			return view, errors.New("failed to read cleanup history")
		}
		view.History = append(view.History, record)
	}
	view.Version = gitDigest(struct {
		Base       string
		Worktrees  []GitWorktree
		Branches   []GitBranch
		Policy     GitCleanupPolicy
		Stashes    GitStashList
		StashError string
	}{strings.TrimSpace(baseOID), view.Worktrees, view.Branches, view.Policy, view.Stashes, view.StashError})
	return view, nil
}

func pathsOverlap(a, b string) bool {
	a, b = filepath.Clean(a), filepath.Clean(b)
	return a == b || strings.HasPrefix(a, b+string(filepath.Separator)) || strings.HasPrefix(b, a+string(filepath.Separator))
}

func protectedGitBranch(name, base string) bool {
	return slices.Contains([]string{base, "main", "master", "develop", "development", "production", "staging", "release", "HEAD"}, name) || strings.HasPrefix(name, "release/") || strings.HasPrefix(name, "hotfix/")
}

func gitAncestor(path, commit, base string) bool {
	_, err := git(path, "merge-base", "--is-ancestor", commit, base)
	return err == nil
}

func gitRefChanged(path, ref string) (time.Time, error) {
	log, err := git(path, "reflog", "show", "-1", "--format=%gD", "--date=unix", ref)
	_, stamp, ok := strings.Cut(log, "@{")
	if err != nil || !ok {
		return time.Time{}, errors.New("reflog unavailable")
	}
	n, err := strconv.ParseInt(strings.TrimSuffix(strings.TrimSpace(stamp), "}"), 10, 64)
	return time.Unix(n, 0), err
}

func worktreeIdle(path string) error {
	gitDir, err := git(path, "rev-parse", "--path-format=absolute", "--git-dir")
	if err != nil {
		return errors.New("cannot check Git operation state")
	}
	if _, err := os.Lstat(filepath.Join(strings.TrimSpace(gitDir), "config.worktree")); !errors.Is(err, os.ErrNotExist) {
		return errors.New("worktree-specific configuration exists or cannot be checked; manual retention required")
	}
	if err := gitOperationIdle(path); err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, "lsof", "-nP", "-t", "+D", path)
	var out, stderr limitedBuffer
	cmd.Stdout, cmd.Stderr = &out, &stderr
	err = cmd.Run()
	if ctx.Err() != nil || stderr.Len() > 0 || out.overflow {
		return errors.New("cannot confirm the worktree is unused by processes")
	}
	if out.Len() > 0 {
		return errors.New("a process is using the worktree")
	}
	var exit *exec.ExitError
	if errors.As(err, &exit) && exit.ExitCode() == 1 {
		return nil
	}
	if err != nil {
		return errors.New("cannot verify the worktree is unused (lsof required)")
	}
	return nil
}

func gitOperationIdle(path string) error {
	gitDir, err := git(path, "rev-parse", "--path-format=absolute", "--git-dir")
	if err != nil {
		return errors.New("cannot check Git operation state")
	}
	for _, name := range []string{"index.lock", "HEAD.lock", "MERGE_HEAD", "CHERRY_PICK_HEAD", "REVERT_HEAD", "rebase-merge", "rebase-apply", "sequencer"} {
		if _, err := os.Lstat(filepath.Join(strings.TrimSpace(gitDir), name)); !errors.Is(err, os.ErrNotExist) {
			return errors.New("Git operation in progress or state unavailable")
		}
	}
	return nil
}

// SaveGitCleanupPolicy changes app-owned settings, never the project's config file.
func (s *Service) SaveGitCleanupPolicy(path, version string, policy GitCleanupPolicy) error {
	s.gitMu.Lock()
	defer s.gitMu.Unlock()
	if _, err := s.registeredProject(path); err != nil {
		return err
	}
	dir, state, err := s.gitState(path)
	if err != nil {
		return err
	}
	if gitDigest(state.Policy) != version {
		return errors.New("cleanup settings changed; refresh and try again")
	}
	if policy.RetentionDays < 1 || policy.RetentionDays > 365 {
		return errors.New("retention must be 1–365 days")
	}
	if _, err := git(path, "show-ref", "--verify", "refs/heads/"+policy.BaseBranch); err != nil {
		return errors.New("select an existing local base branch")
	}
	state.Policy = policy
	return saveJSON(filepath.Join(dir, "state.json"), state)
}
