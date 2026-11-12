// Package workspace owns project registration, inspection and explicit rule updates.
package workspace

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"time"
)

type Project struct {
	Name string `json:"name"`
	Path string `json:"path"`
	ID   string `json:"id,omitempty"`
}

type Inspection struct {
	Project     Project           `json:"project"`
	Git         bool              `json:"git"`
	Branch      string            `json:"branch"`
	Status      string            `json:"status"`
	StatusKnown bool              `json:"statusKnown"`
	StashKnown  bool              `json:"stashKnown"`
	StashCount  int               `json:"stashCount"`
	Documents   []string          `json:"documents"`
	Scripts     map[string]string `json:"scripts"`
	Warnings    []string          `json:"warnings"`
}

// Service persists registrations separately from the repositories it inspects.
type Service struct {
	mu                  sync.Mutex
	path                string
	syncMu              sync.Mutex
	previews            map[string]pendingUpdate
	taskMu              sync.Mutex
	serviceStartMu      sync.Mutex
	activePipelines     map[string]*activePipeline
	activeLocalServices map[string]map[string]*activeLocalService
	activeClone         *activeClone
	taskWG              sync.WaitGroup
	closing             bool
	gitMu               sync.Mutex
	reviewMu            sync.Mutex
	memoryMu            sync.Mutex
	memoryExtractMu     sync.Mutex
	gitCleanupCancel    context.CancelFunc
}

func NewService(path string) *Service { return &Service{path: path} }

func (s *Service) List() ([]Project, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.load()
}

func (s *Service) load() ([]Project, error) {
	projects := []Project{}
	data, err := os.ReadFile(s.path)
	if errors.Is(err, os.ErrNotExist) {
		return projects, nil
	}
	if err != nil {
		return nil, fmt.Errorf("read project registry: %w", err)
	}
	if err := json.Unmarshal(data, &projects); err != nil {
		return nil, fmt.Errorf("invalid project registry: %w", err)
	}
	if projects == nil {
		projects = []Project{}
	}
	return projects, nil
}

// Add registers a directory. It never writes into that directory.
func (s *Service) Add(path string) (Project, error) {
	if !filepath.IsAbs(path) {
		return Project{}, errors.New("enter the project's absolute path")
	}
	canonical, err := filepath.EvalSymlinks(filepath.Clean(path))
	if err != nil {
		return Project{}, fmt.Errorf("cannot access folder: %w", err)
	}
	info, err := os.Stat(canonical)
	if err != nil {
		return Project{}, err
	}
	if !info.IsDir() {
		return Project{}, errors.New("select a folder rather than a file")
	}
	p := Project{Name: filepath.Base(canonical), Path: canonical}
	s.mu.Lock()
	defer s.mu.Unlock()
	projects, err := s.load()
	if err != nil {
		return Project{}, err
	}
	if slices.ContainsFunc(projects, func(existing Project) bool { return existing.Path == p.Path }) {
		return p, nil
	}
	projects = append(projects, p)
	if err := saveJSON(s.path, projects); err != nil {
		return Project{}, err
	}
	return p, nil
}

// RemoveProject unregisters a project without deleting its files, logs or backups.
func (s *Service) RemoveProject(path string) error {
	_, err := s.changeProject(path, "")
	return err
}

// RelocateProject repairs a registration after a directory move; it does not move files.
func (s *Service) RelocateProject(path, destination string) (Project, error) {
	if !filepath.IsAbs(destination) {
		return Project{}, errors.New("enter the new folder's absolute path")
	}
	canonical, err := filepath.EvalSymlinks(filepath.Clean(destination))
	if err != nil {
		return Project{}, err
	}
	info, err := os.Stat(canonical)
	if err != nil || !info.IsDir() {
		return Project{}, errors.New("the new path must be an accessible folder")
	}
	return s.changeProject(path, canonical)
}

func (s *Service) changeProject(path, destination string) (Project, error) {
	// Match synchronization's lock order. Task ownership cannot change during registration edits.
	s.syncMu.Lock()
	defer s.syncMu.Unlock()
	s.taskMu.Lock()
	defer s.taskMu.Unlock()
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closing {
		return Project{}, errors.New("the app is shutting down")
	}
	projects, err := s.load()
	if err != nil {
		return Project{}, err
	}
	index := slices.IndexFunc(projects, func(p Project) bool { return p.Path == path })
	if index < 0 {
		return Project{}, errors.New("register the project folder first")
	}
	for _, active := range s.activeLocalServices[path] {
		active.mu.Lock()
		running := localServiceRunning(active.state.Status)
		active.mu.Unlock()
		if running {
			return Project{}, errors.New("stop this project's local services first")
		}
	}
	if active := s.activePipelines[path]; active != nil {
		active.mu.Lock()
		running := active.run.Status == "running"
		active.mu.Unlock()
		if running {
			return Project{}, errors.New("stop this project's pipelines first")
		}
	}
	updated := projects[index]
	if destination == "" {
		projects = slices.Delete(projects, index, index+1)
	} else {
		if destination != path && slices.ContainsFunc(projects, func(p Project) bool { return p.Path == destination }) {
			return Project{}, errors.New("the new folder is already registered")
		}
		updated = Project{Name: filepath.Base(destination), Path: destination}
		projects[index] = updated
	}
	if err := saveJSON(s.path, projects); err != nil {
		return Project{}, err
	}
	for token, preview := range s.previews {
		if preview.Record.Project.Path == path {
			delete(s.previews, token)
		}
	}
	return updated, nil
}

func (s *Service) registeredProject(path string) (Project, error) {
	projects, err := s.List()
	if err != nil {
		return Project{}, err
	}
	index := slices.IndexFunc(projects, func(p Project) bool { return p.Path == path })
	if index < 0 {
		return Project{}, errors.New("register the project folder first")
	}
	return projects[index], nil
}

// Inspect only accepts registered projects and does not execute project scripts.
func (s *Service) Inspect(path string) (Inspection, error) {
	project, err := s.registeredProject(path)
	if err != nil {
		return Inspection{}, err
	}
	result := Inspection{Project: project, Documents: []string{}, Scripts: map[string]string{}, Warnings: []string{}}
	info, err := os.Stat(path)
	if err != nil {
		return result, fmt.Errorf("project folder inaccessible: %w", err)
	}
	if !info.IsDir() {
		return result, errors.New("the project path is no longer a folder")
	}
	branch, err := git(path, "rev-parse", "--abbrev-ref", "HEAD")
	if err == nil {
		result.Git = true
		result.Branch = strings.TrimSpace(branch)
	} else {
		// An unborn branch still belongs to a Git repository.
		if _, repoErr := git(path, "rev-parse", "--git-dir"); repoErr == nil {
			result.Git = true
			result.Branch, _ = git(path, "symbolic-ref", "--short", "HEAD")
			result.Branch = strings.TrimSpace(result.Branch)
		} else {
			result.Warnings = append(result.Warnings, "No usable Git repository found: "+err.Error())
		}
	}
	if result.Git {
		result.Status, err = git(path, "status", "--short", "--untracked-files=normal")
		if err != nil {
			result.Warnings = append(result.Warnings, err.Error())
		}
		result.StatusKnown = err == nil
		stashes, stashErr := readGitStashes(path)
		result.StashKnown, result.StashCount = stashErr == nil, len(stashes.Entries)
		if stashErr != nil {
			result.Warnings = append(result.Warnings, stashErr.Error())
		}
	}
	entries, err := os.ReadDir(path)
	if err != nil {
		result.Warnings = append(result.Warnings, "Read document list: "+err.Error())
	} else {
		for _, entry := range entries {
			if !entry.IsDir() && markdownFile(entry.Name()) {
				result.Documents = append(result.Documents, entry.Name())
			}
		}
	}
	if data, readErr := readText(filepath.Join(path, "package.json")); readErr == nil {
		var pkg struct {
			Scripts map[string]string `json:"scripts"`
		}
		if err := json.Unmarshal([]byte(data), &pkg); err != nil {
			result.Warnings = append(result.Warnings, "Cannot parse package.json")
		} else if pkg.Scripts != nil {
			result.Scripts = pkg.Scripts
		}
	} else if !errors.Is(readErr, os.ErrNotExist) {
		result.Warnings = append(result.Warnings, "package.json: "+readErr.Error())
	}
	return result, nil
}

func markdownFile(name string) bool {
	ext := strings.ToLower(filepath.Ext(name))
	return ext == ".md" || ext == ".markdown"
}

// ReadDocument reads a root Markdown file within a registered project, on demand.
func (s *Service) ReadDocument(path, name string) (string, error) {
	if _, err := s.registeredProject(path); err != nil {
		return "", err
	}
	if filepath.Base(name) != name || !markdownFile(name) {
		return "", errors.New("select a Markdown file in the project root")
	}
	root, err := os.OpenRoot(path)
	if err != nil {
		return "", err
	}
	defer root.Close()
	info, err := root.Stat(name)
	if err != nil {
		return "", err
	}
	if !info.Mode().IsRegular() {
		return "", errors.New("only regular Markdown files can be viewed")
	}
	f, err := root.Open(name)
	if err != nil {
		return "", err
	}
	defer f.Close()
	return readLimitedText(f)
}

func readText(path string) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer f.Close()
	return readLimitedText(f)
}

func readLimitedText(reader io.Reader) (string, error) {
	data, err := io.ReadAll(io.LimitReader(reader, 256*1024+1))
	if err != nil {
		return "", err
	}
	if len(data) > 256*1024 {
		return "", errors.New("file exceeds the 256 KiB display limit")
	}
	return string(data), nil
}

type limitedBuffer struct {
	bytes.Buffer
	overflow bool
}

func (b *limitedBuffer) Write(p []byte) (int, error) {
	n := len(p)
	remaining := 1024*1024 - b.Len()
	if n > remaining {
		b.overflow = true
		p = p[:remaining]
	}
	_, _ = b.Buffer.Write(p)
	return n, nil
}

func git(path string, args ...string) (string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, "git", append([]string{"--no-optional-locks", "-c", "core.fsmonitor=false", "-C", path}, args...)...)
	// Git discovery must refer to this project, even if the launcher has Git env vars.
	for _, entry := range os.Environ() {
		if !strings.HasPrefix(entry, "GIT_") {
			cmd.Env = append(cmd.Env, entry)
		}
	}
	cmd.Env = append(cmd.Env, "GIT_TERMINAL_PROMPT=0", "LC_ALL=C")
	var out, stderr limitedBuffer
	cmd.Stdout, cmd.Stderr = &out, &stderr
	if err := cmd.Run(); err != nil {
		return "", fmt.Errorf("git %s: %w %s", args[0], err, strings.TrimSpace(stderr.String()))
	}
	if out.overflow {
		return out.String() + "\n[output exceeds 1 MiB; truncated]", nil
	}
	return out.String(), nil
}
