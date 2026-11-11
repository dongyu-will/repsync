package workspace

import (
	"context"
	"crypto/rand"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"time"
)

// CleanupGitWorkspace applies exactly the reviewed inventory, rechecking it first.
func (s *Service) CleanupGitWorkspace(path, version string) (GitCleanupRecord, error) {
	s.gitMu.Lock()
	defer s.gitMu.Unlock()
	return s.cleanupGitWorkspace(path, version, false)
}

func (s *Service) cleanupGitWorkspace(path, version string, automatic bool) (GitCleanupRecord, error) {
	record := GitCleanupRecord{Items: []GitCleanupItem{}}
	view, err := s.gitOverview(path)
	if err != nil {
		return record, err
	}
	if !view.Git {
		return record, errors.New("current folder is not a Git repository")
	}
	if view.Version != version {
		return record, errors.New("worktrees, branches, or cleanup settings changed; preview again")
	}
	dir, _, err := s.gitState(path)
	if err != nil {
		return record, err
	}
	record.ID = fmt.Sprintf("%019d-%s", time.Now().UnixMilli(), rand.Text())
	record.CreatedAt, record.Automatic = time.Now().UTC().Format(time.RFC3339), automatic
	for _, w := range view.Worktrees {
		if w.Eligible {
			record.Items = append(record.Items, GitCleanupItem{Kind: "worktree", Target: w.Path, Branch: w.Branch, Commit: w.Commit, Status: "pending"})
		}
	}
	for _, b := range view.Branches {
		if b.Eligible {
			record.Items = append(record.Items, GitCleanupItem{Kind: "branch", Target: b.Name, Branch: b.Name, Commit: b.Commit, Upstream: b.Upstream, Status: "pending"})
		}
	}
	if len(record.Items) == 0 {
		return record, nil
	}
	// Freeze in-app task starts and registrations while removing verified paths.
	s.taskMu.Lock()
	defer s.taskMu.Unlock()
	if s.closing {
		return record, errors.New("the app is shutting down")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	projects, err := s.load()
	if err != nil {
		return record, err
	}
	if !slices.ContainsFunc(projects, func(p Project) bool { return p.Path == path }) {
		return record, errors.New("project registration removed")
	}
	file := filepath.Join(dir, "history", record.ID+".json")
	stashes, err := readGitStashes(path)
	if err != nil || len(stashes.Entries) > 0 {
		return record, errors.New("stash state changed or pending stashes remain; check again")
	}
	for i := range record.Items {
		item := &record.Items[i]
		item.Config, err = gitBranchSettings(path, item.Branch)
		if err != nil {
			return record, err
		}
		item.BackupRef = fmt.Sprintf("refs/repsync/cleanup/%s/%d", record.ID, i)
		if _, err := git(path, "update-ref", item.BackupRef, item.Commit, strings.Repeat("0", len(item.Commit))); err != nil {
			return record, err
		}
	}
	if err := saveJSON(file, record); err != nil {
		return record, err
	}
	for i := range record.Items {
		item := &record.Items[i]
		var operationErr error
		if item.Kind == "worktree" {
			info, e := os.Lstat(item.Target)
			if e != nil || !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
				operationErr = errors.New("worktree folder changed")
			}
			for _, p := range projects {
				if pathsOverlap(p.Path, item.Target) {
					operationErr = errors.New("worktree is registered; folder preserved")
					break
				}
			}
			if operationErr == nil {
				operationErr = worktreeIdle(item.Target)
			}
			if operationErr == nil {
				status, e := git(item.Target, "status", "--porcelain=v1", "--untracked-files=all", "--ignored=matching")
				if e != nil || status != "" {
					operationErr = errors.New("file state changed; worktree preserved")
				}
			}
			if operationErr == nil {
				head, e := git(item.Target, "rev-parse", "HEAD")
				if e != nil || strings.TrimSpace(head) != item.Commit {
					operationErr = errors.New("worktree commit changed")
				}
				branch, e := git(item.Target, "symbolic-ref", "--short", "HEAD")
				if e != nil || strings.TrimSpace(branch) != item.Branch {
					operationErr = errors.New("worktree branch changed")
				}
			}
			if operationErr == nil {
				_, operationErr = git(path, "-c", "core.hooksPath=/dev/null", "worktree", "remove", "--", item.Target)
			}
		} else {
			current, e := git(path, "rev-parse", "--verify", "refs/heads/"+item.Branch)
			if e != nil || strings.TrimSpace(current) != item.Commit || !gitAncestor(path, item.Commit, "refs/heads/"+view.Policy.BaseBranch) {
				operationErr = errors.New("branch commit or merge state changed")
			}
			if operationErr == nil {
				_, operationErr = git(path, "-c", "core.hooksPath=/dev/null", "branch", "-d", "--", item.Branch)
			}
		}
		if operationErr != nil {
			item.Status, item.Error = "skipped", operationErr.Error()
		} else {
			item.Status = "removed"
		}
		if err := saveJSON(file, record); err != nil {
			return record, fmt.Errorf("failed to save cleanup results; compare record %s with actual Git state: %w", record.ID, err)
		}
	}
	return record, nil
}

// RestoreGitCleanup recreates refs and clean worktrees without overwriting anything.
func (s *Service) RestoreGitCleanup(path, id string) (GitCleanupRecord, error) {
	s.gitMu.Lock()
	defer s.gitMu.Unlock()
	var record GitCleanupRecord
	if _, err := s.registeredProject(path); err != nil {
		return record, err
	}
	if id == "" || filepath.Base(id) != id || strings.ContainsAny(id, "/\\") {
		return record, errors.New("invalid cleanup record ID")
	}
	dir, state, err := s.gitState(path)
	if err != nil {
		return record, err
	}
	file := filepath.Join(dir, "history", id+".json")
	data, err := os.ReadFile(file)
	if err != nil {
		return record, err
	}
	if err = json.Unmarshal(data, &record); err != nil {
		return record, err
	}
	if record.ID != id || record.RestoredAt != "" {
		return record, errors.New("record already restored or ID does not match")
	}
	s.taskMu.Lock()
	defer s.taskMu.Unlock()
	if s.closing {
		return record, errors.New("the app is shutting down")
	}
	// Restored items receive a fresh retention period even before the next scan.
	for _, item := range record.Items {
		delete(state.Seen, "branch:"+item.Branch)
		delete(state.Seen, "worktree:"+item.Target)
	}
	if err := saveJSON(filepath.Join(dir, "state.json"), state); err != nil {
		return record, err
	}
	for i, item := range record.Items {
		if item.Status != "removed" && item.Status != "pending" {
			continue
		}
		ref := fmt.Sprintf("refs/repsync/cleanup/%s/%d", id, i)
		commit, e := git(path, "rev-parse", "--verify", ref+"^{commit}")
		if e != nil || strings.TrimSpace(commit) != item.Commit {
			return record, errors.New("recovery commit unavailable; existing content preserved")
		}
		current, e := git(path, "show-ref", "--verify", "--hash", "refs/heads/"+item.Branch)
		created := e != nil
		if e == nil {
			if strings.TrimSpace(current) != item.Commit {
				return record, errors.New("branch with the same name has new commits; preserved")
			}
		} else if _, e = git(path, "-c", "core.hooksPath=/dev/null", "branch", "--", item.Branch, item.Commit); e != nil {
			return record, e
		}
		if created {
			for _, setting := range item.Config {
				if _, e = git(path, "config", "--local", "--add", setting.Key, setting.Value); e != nil {
					return record, e
				}
			}
		}
		if item.Kind == "worktree" {
			if _, e = os.Lstat(item.Target); !errors.Is(e, os.ErrNotExist) {
				return record, errors.New("original worktree path exists or cannot be verified; preserved")
			}
			if _, e = git(path, "-c", "core.hooksPath=/dev/null", "worktree", "add", "--", item.Target, item.Branch); e != nil {
				return record, e
			}
		}
		record.Items[i].Status = "restored"
		if err = saveJSON(file, record); err != nil {
			return record, err
		}
	}
	record.RestoredAt = time.Now().UTC().Format(time.RFC3339)
	err = saveJSON(file, record)
	return record, err
}

func gitBranchSettings(path, branch string) ([]GitBranchSetting, error) {
	settings := []GitBranchSetting{}
	raw, err := git(path, "config", "--local", "--null", "--get-regexp", "^branch\\."+regexp.QuoteMeta(branch)+"\\.")
	if err != nil {
		var exit *exec.ExitError
		if errors.As(err, &exit) && exit.ExitCode() == 1 {
			return settings, nil
		}
		return settings, err
	}
	for _, field := range strings.Split(raw, "\x00") {
		if field == "" {
			continue
		}
		key, value, ok := strings.Cut(field, "\n")
		if !ok {
			return settings, errors.New("could not fully back up branch settings")
		}
		settings = append(settings, GitBranchSetting{Key: key, Value: value})
	}
	return settings, nil
}

// RunAutomaticGitCleanup checks enabled repositories once; useful to the desktop and CLI.
func (s *Service) RunAutomaticGitCleanup() ([]GitCleanupRecord, error) {
	results := []GitCleanupRecord{}
	projects, err := s.List()
	if err != nil {
		return results, err
	}
	for _, p := range projects {
		s.taskMu.Lock()
		closing := s.closing
		s.taskMu.Unlock()
		if closing {
			return results, err
		}
		s.gitMu.Lock()
		_, state, e := s.gitState(p.Path)
		if e == nil && state.Policy.Enabled {
			view, scanErr := s.gitOverview(p.Path)
			if scanErr == nil {
				var result GitCleanupRecord
				result, e = s.cleanupGitWorkspace(p.Path, view.Version, true)
				if len(result.Items) > 0 {
					results = append(results, result)
				}
			} else {
				e = scanErr
			}
		}
		s.gitMu.Unlock()
		if e != nil && state.Policy.Enabled {
			err = errors.Join(err, fmt.Errorf("%s: %w", p.Name, e))
		}
	}
	return results, err
}

// StartGitCleanup keeps the timer owned by the desktop and joins it during Shutdown.
func (s *Service) StartGitCleanup() {
	s.taskMu.Lock()
	defer s.taskMu.Unlock()
	if s.closing || s.gitCleanupCancel != nil {
		return
	}
	ctx, cancel := context.WithCancel(context.Background())
	s.gitCleanupCancel = cancel
	s.taskWG.Add(1)
	go func() {
		defer s.taskWG.Done()
		timer := time.NewTimer(15 * time.Second)
		defer timer.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-timer.C:
				if _, err := s.RunAutomaticGitCleanup(); err != nil {
					log.Printf("Automatic cleanup incomplete: %v", err)
				}
				timer.Reset(time.Hour)
			}
		}
	}()
}
