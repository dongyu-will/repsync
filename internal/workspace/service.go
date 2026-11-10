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
}

type Inspection struct {
	Project     Project           `json:"project"`
	Git         bool              `json:"git"`
	Branch      string            `json:"branch"`
	Status      string            `json:"status"`
	StatusKnown bool              `json:"statusKnown"`
	Documents   []string          `json:"documents"`
	Scripts     map[string]string `json:"scripts"`
	Warnings    []string          `json:"warnings"`
}

// Service persists registrations separately from the repositories it inspects.
type Service struct {
	mu       sync.Mutex
	path     string
	syncMu   sync.Mutex
	previews map[string]pendingUpdate
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
		return nil, fmt.Errorf("读取项目登记: %w", err)
	}
	if err := json.Unmarshal(data, &projects); err != nil {
		return nil, fmt.Errorf("项目登记损坏: %w", err)
	}
	if projects == nil {
		projects = []Project{}
	}
	return projects, nil
}

// Add registers a directory. It never writes into that directory.
func (s *Service) Add(path string) (Project, error) {
	if !filepath.IsAbs(path) {
		return Project{}, errors.New("请输入项目的绝对路径")
	}
	canonical, err := filepath.EvalSymlinks(filepath.Clean(path))
	if err != nil {
		return Project{}, fmt.Errorf("无法访问目录: %w", err)
	}
	info, err := os.Stat(canonical)
	if err != nil {
		return Project{}, err
	}
	if !info.IsDir() {
		return Project{}, errors.New("请选择目录，而不是文件")
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
	if err := os.MkdirAll(filepath.Dir(s.path), 0700); err != nil {
		return Project{}, err
	}
	data, err := json.MarshalIndent(projects, "", "  ")
	if err != nil {
		return Project{}, err
	}
	f, err := os.CreateTemp(filepath.Dir(s.path), ".projects-*")
	if err != nil {
		return Project{}, err
	}
	defer os.Remove(f.Name())
	if _, err = f.Write(data); err != nil {
		_ = f.Close()
		return Project{}, err
	}
	if err = f.Close(); err != nil {
		return Project{}, err
	}
	if err = os.Rename(f.Name(), s.path); err != nil {
		return Project{}, err
	}
	return p, nil
}

func (s *Service) registeredProject(path string) (Project, error) {
	projects, err := s.List()
	if err != nil {
		return Project{}, err
	}
	index := slices.IndexFunc(projects, func(p Project) bool { return p.Path == path })
	if index < 0 {
		return Project{}, errors.New("请先登记项目目录")
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
		return result, fmt.Errorf("项目目录不可访问: %w", err)
	}
	if !info.IsDir() {
		return result, errors.New("项目路径已不再是目录")
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
			result.Warnings = append(result.Warnings, "未识别到可用 Git 仓库："+err.Error())
		}
	}
	if result.Git {
		result.Status, err = git(path, "status", "--short", "--untracked-files=normal")
		if err != nil {
			result.Warnings = append(result.Warnings, err.Error())
		}
		result.StatusKnown = err == nil
	}
	entries, err := os.ReadDir(path)
	if err != nil {
		result.Warnings = append(result.Warnings, "读取文档列表："+err.Error())
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
			result.Warnings = append(result.Warnings, "package.json 无法解析")
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
		return "", errors.New("请选择项目根目录中的 Markdown 文件")
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
		return "", errors.New("只能查看普通 Markdown 文件")
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
		return "", errors.New("文件超过 256 KiB，暂不展示")
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
		return out.String() + "\n[输出超过 1 MiB，已截断]", nil
	}
	return out.String(), nil
}
