package workspace

import (
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
)

type Pipeline struct {
	ID                string            `json:"id"`
	Name              string            `json:"name"`
	Kind              string            `json:"kind"`
	Steps             [][]string        `json:"steps"`
	TimeoutSeconds    int               `json:"timeoutSeconds"`
	Cwd               string            `json:"cwd"`
	Env               map[string]string `json:"env"`
	Coverage          []ReviewCoverage  `json:"coverage,omitempty"`
	ReviewEnvironment string            `json:"reviewEnvironment,omitempty"`
	ReviewTarget      string            `json:"reviewTarget,omitempty"`
}

type PipelineConfig struct {
	Pipelines []Pipeline `json:"pipelines"`
	Source    string     `json:"source"`
	Version   string     `json:"version"`
}

func readProjectConfig(root *os.Root, name string) ([]byte, error) {
	info, err := root.Stat(name)
	if err != nil {
		return nil, err
	}
	if !info.Mode().IsRegular() {
		return nil, fmt.Errorf("%s must be a regular file", name)
	}
	f, err := root.Open(name)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	text, err := readLimitedText(f)
	return []byte(text), err
}

func projectPackageManager(root *os.Root, field string) (string, error) {
	manager := strings.Split(field, "@")[0]
	if manager == "" {
		manager = "npm"
		for _, lock := range []struct{ file, manager string }{{"pnpm-lock.yaml", "pnpm"}, {"yarn.lock", "yarn"}, {"bun.lock", "bun"}, {"bun.lockb", "bun"}} {
			if _, err := root.Stat(lock.file); err == nil {
				manager = lock.manager
				break
			}
		}
	}
	if !slices.Contains([]string{"npm", "pnpm", "yarn", "bun"}, manager) {
		return "", errors.New("cannot detect package manager; configure commands explicitly in .repsync.json")
	}
	return manager, nil
}

// Pipelines reads explicit project commands. Discovery never runs them.
func (s *Service) Pipelines(path string) (PipelineConfig, error) {
	if _, err := s.registeredProject(path); err != nil {
		return PipelineConfig{}, err
	}
	root, err := os.OpenRoot(path)
	if err != nil {
		return PipelineConfig{}, err
	}
	defer root.Close()
	read := func(name string) ([]byte, error) { return readProjectConfig(root, name) }
	config := PipelineConfig{Pipelines: []Pipeline{}, Source: ".repsync.json"}
	data, err := read(config.Source)
	var file struct {
		Pipelines []Pipeline `json:"pipelines"`
	}
	if err == nil {
		if err = json.Unmarshal(data, &file); err != nil {
			return config, fmt.Errorf("read .repsync.json: %w", err)
		}
		config.Pipelines = file.Pipelines
	} else if !errors.Is(err, os.ErrNotExist) {
		return config, err
	}
	if file.Pipelines == nil {
		config.Source = "package.json"
		data, err = read(config.Source)
		if errors.Is(err, os.ErrNotExist) {
			data, err = []byte("{}"), nil
		}
		if err != nil {
			return config, err
		}
		var pkg struct {
			Scripts        map[string]string `json:"scripts"`
			PackageManager string            `json:"packageManager"`
		}
		if err = json.Unmarshal(data, &pkg); err != nil {
			return config, fmt.Errorf("read package.json: %w", err)
		}
		for _, name := range []string{"ci", "check:ci", "check"} {
			if _, ok := pkg.Scripts[name]; !ok {
				continue
			}
			manager, err := projectPackageManager(root, pkg.PackageManager)
			if err != nil {
				return config, err
			}
			config.Pipelines = []Pipeline{{ID: name, Name: name, Kind: "ci", Steps: [][]string{{manager, "run", name}}, TimeoutSeconds: 1800}}
			break
		}
	}
	if config.Pipelines == nil {
		config.Pipelines = []Pipeline{}
	}
	if err := validatePipelines(config.Pipelines); err != nil {
		return config, err
	}
	for _, pipeline := range config.Pipelines {
		if _, err := commandDirectory(path, pipeline.Cwd); err != nil {
			return config, err
		}
	}
	// Include source bytes: changing the underlying package script invalidates a preview.
	resolved, _ := json.Marshal(config.Pipelines)
	config.Version = fmt.Sprintf("%x", sha256.Sum256(append(data, resolved...)))
	return config, nil
}

func validatePipelines(pipelines []Pipeline) error {
	if len(pipelines) > 32 {
		return errors.New("each project supports up to 32 pipelines")
	}
	ids := map[string]bool{}
	for i := range pipelines {
		p := &pipelines[i]
		if p.ID == "" || len(p.ID) > 80 || ids[p.ID] || strings.TrimSpace(p.Name) == "" || (p.Kind != "ci" && p.Kind != "cd") {
			return errors.New("pipelines require a unique id, name, and ci/cd kind")
		}
		ids[p.ID] = true
		if err := validateReviewCoverage(p.Coverage, p.ReviewEnvironment, p.ReviewTarget); err != nil {
			return err
		}
		if err := validateCommandOptions(p.Cwd, p.Env); err != nil {
			return err
		}
		if p.TimeoutSeconds == 0 {
			p.TimeoutSeconds = 1800
		}
		if p.TimeoutSeconds < 1 || p.TimeoutSeconds > 7200 {
			return errors.New("pipeline timeout must be 1–7200 seconds")
		}
		if len(p.Steps) < 1 || len(p.Steps) > 32 {
			return errors.New("each pipeline requires 1–32 command steps")
		}
		for _, step := range p.Steps {
			if len(step) == 0 || strings.TrimSpace(step[0]) == "" {
				return errors.New("command steps cannot be empty")
			}
			for _, arg := range step {
				if strings.ContainsRune(arg, 0) {
					return errors.New("commands cannot contain null characters")
				}
			}
		}
	}
	return nil
}

type ActionRun struct {
	DatabaseID   int64  `json:"databaseId"`
	WorkflowName string `json:"workflowName"`
	DisplayTitle string `json:"displayTitle"`
	Status       string `json:"status"`
	Conclusion   string `json:"conclusion"`
	HeadBranch   string `json:"headBranch"`
	HeadSha      string `json:"headSha"`
	CreatedAt    string `json:"createdAt"`
	URL          string `json:"url"`
}

type ActionsOverview struct {
	Repository string      `json:"repository"`
	Runs       []ActionRun `json:"runs"`
}

var githubRepository = regexp.MustCompile(`^[A-Za-z0-9_.-]+/[A-Za-z0-9_.-]+$`)

func githubRepo(remote string) string {
	remote = strings.TrimSpace(remote)
	var repo string
	if strings.HasPrefix(remote, "git@github.com:") {
		repo = strings.TrimPrefix(remote, "git@github.com:")
	} else if u, err := url.Parse(remote); err == nil && u.Host == "github.com" && (u.Scheme == "https" || u.Scheme == "ssh") {
		repo = strings.TrimPrefix(u.Path, "/")
	}
	repo = strings.TrimSuffix(repo, ".git")
	if !githubRepository.MatchString(repo) || strings.Contains(repo, "..") {
		return ""
	}
	return repo
}

// GitHubActions uses the local gh login, including its access to private repos.
func (s *Service) GitHubActions(path string) (ActionsOverview, error) {
	result := ActionsOverview{Runs: []ActionRun{}}
	if _, err := s.registeredProject(path); err != nil {
		return result, err
	}
	remote, err := git(path, "remote", "get-url", "origin")
	if err != nil {
		return result, errors.New("no origin remote configured; local CI/CD remains available")
	}
	result.Repository = githubRepo(remote)
	if result.Repository == "" {
		return result, errors.New("origin is not a github.com repository; local CI/CD remains available")
	}
	output, err := runGitHubCLI(path, "run", "list", "--repo", result.Repository, "--limit", "20", "--json", "databaseId,workflowName,displayTitle,status,conclusion,headBranch,headSha,createdAt,url")
	if err != nil {
		return result, err
	}
	if err := json.Unmarshal(output, &result.Runs); err != nil {
		return result, fmt.Errorf("parse GitHub Actions: %w", err)
	}
	if result.Runs == nil {
		result.Runs = []ActionRun{}
	}
	// Only expose links belonging to this repository, including malformed CLI output.
	for i := range result.Runs {
		if !strings.HasPrefix(result.Runs[i].URL, "https://github.com/"+result.Repository+"/actions/runs/") {
			result.Runs[i].URL = ""
		}
	}
	return result, nil
}

func (s *Service) pipelineDirectory(path string) string {
	return filepath.Join(filepath.Dir(s.path), "pipelines", fmt.Sprintf("%x", sha256.Sum256([]byte(path))))
}

func readPipelineJSON(path string, value any) error {
	f, err := os.Open(path)
	if err != nil {
		return err
	}
	defer f.Close()
	return json.NewDecoder(io.LimitReader(f, 256*1024)).Decode(value)
}
