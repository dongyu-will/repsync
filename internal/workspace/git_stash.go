package workspace

import (
	"errors"
	"fmt"
	"path/filepath"
	"slices"
	"strings"
)

type GitStash struct {
	OID       string `json:"oid"`
	Selector  string `json:"selector"`
	CreatedAt string `json:"createdAt"`
	Subject   string `json:"subject"`
}

type GitStashList struct {
	Entries []GitStash `json:"entries"`
	Head    string     `json:"head"`
	Branch  string     `json:"branch"`
	Version string     `json:"version"`
}

type GitStashPreview struct {
	Stash     GitStash `json:"stash"`
	Version   string   `json:"version"`
	Branch    string   `json:"branch"`
	Patch     string   `json:"patch"`
	Truncated bool     `json:"truncated"`
}

type GitStashApplyResult struct {
	Applied bool   `json:"applied"`
	Message string `json:"message"`
}

// GitStashes reads the shared stash reflog, including stashes created in linked worktrees.
func (s *Service) GitStashes(path string) (GitStashList, error) {
	s.gitMu.Lock()
	defer s.gitMu.Unlock()
	if _, err := s.registeredProject(path); err != nil {
		return GitStashList{}, err
	}
	return readGitStashes(path)
}

func readGitStashes(path string) (GitStashList, error) {
	list := GitStashList{Entries: []GitStash{}}
	raw, err := git(path, "stash", "list", "--format=%H%x00%gd%x00%cI%x00%gs")
	if err != nil {
		return list, fmt.Errorf("read stash: %w", err)
	}
	if strings.Contains(raw, "[output exceeds 1 MiB") {
		return list, errors.New("stash list exceeds read limits; full state cannot be verified")
	}
	for _, line := range strings.Split(strings.TrimSuffix(raw, "\n"), "\n") {
		if line == "" {
			continue
		}
		fields := strings.SplitN(line, "\x00", 4)
		if len(fields) != 4 {
			return list, errors.New("could not fully parse the stash list")
		}
		list.Entries = append(list.Entries, GitStash{OID: fields[0], Selector: fields[1], CreatedAt: fields[2], Subject: fields[3]})
	}
	// An unborn repository can have no HEAD and no stash; listing it is still useful.
	head, err := git(path, "rev-parse", "--verify", "HEAD")
	if err != nil && len(list.Entries) > 0 {
		return list, fmt.Errorf("read restore target commit: %w", err)
	}
	list.Head = strings.TrimSpace(head)
	branch, _ := git(path, "symbolic-ref", "--short", "HEAD")
	list.Branch = strings.TrimSpace(branch)
	list.Version = gitDigest(struct {
		Entries      []GitStash
		Head, Branch string
	}{list.Entries, list.Head, list.Branch})
	return list, nil
}

// GitStashChanges previews a stash by its immutable object ID, never by a shifting index.
func (s *Service) GitStashChanges(path, oid string) (GitStashPreview, error) {
	s.gitMu.Lock()
	defer s.gitMu.Unlock()
	preview := GitStashPreview{}
	if _, err := s.registeredProject(path); err != nil {
		return preview, err
	}
	list, err := readGitStashes(path)
	if err != nil {
		return preview, err
	}
	index := slices.IndexFunc(list.Entries, func(item GitStash) bool { return item.OID == oid })
	if index < 0 {
		return preview, errors.New("stash changed or no longer exists; refresh the list")
	}
	preview.Stash, preview.Version, preview.Branch = list.Entries[index], list.Version, list.Branch
	preview.Patch, err = git(path, "-c", "core.hooksPath=/dev/null", "stash", "show", "--no-ext-diff", "--no-textconv", "--include-untracked", "--stat", "--patch", oid)
	preview.Truncated = strings.Contains(preview.Patch, "[output exceeds 1 MiB")
	if preview.Patch == "" && err == nil {
		preview.Patch = "This stash has no displayable file changes."
	}
	return preview, err
}

// ApplyGitStash restores to a clean registered worktree and always retains the stash.
func (s *Service) ApplyGitStash(path, oid, version string) (GitStashApplyResult, error) {
	result := GitStashApplyResult{}
	s.gitMu.Lock()
	defer s.gitMu.Unlock()
	// Follow registration's sync -> task -> registry lock order.
	s.syncMu.Lock()
	defer s.syncMu.Unlock()
	s.taskMu.Lock()
	defer s.taskMu.Unlock()
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closing {
		return result, errors.New("the app is shutting down")
	}
	projects, err := s.load()
	if err != nil {
		return result, err
	}
	if !slices.ContainsFunc(projects, func(p Project) bool { return p.Path == path }) {
		return result, errors.New("register the project folder first")
	}
	top, err := git(path, "rev-parse", "--show-toplevel")
	if err != nil {
		return result, err
	}
	root, err := filepath.EvalSymlinks(strings.TrimSpace(top))
	if err != nil || root != path {
		return result, errors.New("applying a stash affects the entire worktree; register and open its root first")
	}
	for _, active := range s.activeLocalServices[path] {
		active.mu.Lock()
		running := localServiceRunning(active.state.Status)
		active.mu.Unlock()
		if running {
			return result, errors.New("stop this project's local services before applying a stash")
		}
	}
	if active := s.activePipelines[path]; active != nil {
		active.mu.Lock()
		running := active.run.Status == "running"
		active.mu.Unlock()
		if running {
			return result, errors.New("wait for or stop this project's pipeline first")
		}
	}
	if s.activePolicyChecks[path] != nil {
		return result, errors.New("wait for project rule checks to finish")
	}
	list, err := readGitStashes(path)
	if err != nil {
		return result, err
	}
	if list.Version != version || !slices.ContainsFunc(list.Entries, func(item GitStash) bool { return item.OID == oid }) {
		return result, errors.New("stash list, target branch, or commit changed; review the contents again")
	}
	if list.Branch == "" {
		return result, errors.New("HEAD is detached; switch to the target branch in Git first")
	}
	if err := gitOperationIdle(path); err != nil {
		return result, err
	}
	status, err := git(path, "status", "--porcelain=v1", "--untracked-files=all")
	if err != nil {
		return result, err
	}
	if status != "" {
		return result, errors.New("the worktree has modified or untracked files; resolve them first; stash preserved")
	}
	_, err = git(path, "-c", "core.hooksPath=/dev/null", "stash", "apply", "--index", oid)
	if err != nil {
		result.Message = "Apply incomplete; partial changes or conflicts may remain. Original stash preserved. Review the worktree before continuing."
		return result, nil
	}
	result.Applied = true
	result.Message = "Applied to this worktree; original stash preserved. Review files, index, and untracked content."
	return result, nil
}
