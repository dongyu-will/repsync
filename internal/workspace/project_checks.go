package workspace

import (
	"encoding/json"
	"errors"
	"io/fs"
	"maps"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"

	"github.com/pelletier/go-toml/v2"
)

type CheckConfiguration struct {
	Path string `json:"path"`
	Tool string `json:"tool"`
}

type ProjectCheck struct {
	Pipeline Pipeline `json:"pipeline"`
	Source   string   `json:"source"`
}

type ProjectCheckOverview struct {
	Configurations []CheckConfiguration `json:"configurations"`
	Guidance       []string             `json:"guidance"`
	Checks         []ProjectCheck       `json:"checks"`
	Warnings       []string             `json:"warnings"`
	Version        string               `json:"version"`
}

var checkTaskName = regexp.MustCompile(`^(?:ci|check|lint|typecheck|test|verify|validate)(?:$|[:-])`)
var mutatingCheckFlags = regexp.MustCompile(`(?:^|\s)--(?:fix|write|watch|updateSnapshot)(?:=|\s|$)`)
var mutatingCheckName = regexp.MustCompile(`(?:^|[:-])(?:fix|write|update|watch|dev|deploy|publish|release)(?:$|[:-])`)

func isCheckCommand(name, body string) bool {
	return checkTaskName.MatchString(name) && !mutatingCheckName.MatchString(name) && !mutatingCheckFlags.MatchString(body)
}

// ProjectChecks inventories existing tools and commands. Config files are read as
// data; ESLint JS, Make recipes, and prose are never evaluated to discover rules.
func (s *Service) ProjectChecks(path string) (ProjectCheckOverview, error) {
	result := ProjectCheckOverview{Configurations: []CheckConfiguration{}, Guidance: []string{}, Checks: []ProjectCheck{}, Warnings: []string{}}
	if _, err := s.registeredProject(path); err != nil {
		return result, err
	}
	root, err := os.OpenRoot(path)
	if err != nil {
		return result, err
	}
	defer root.Close()
	names, err := projectCheckFiles(path, root)
	if err != nil {
		result.Warnings = append(result.Warnings, err.Error())
	}
	inputs := []byte{}
	seen := map[string]bool{}
	add := func(p Pipeline, source string) {
		key := source + "\x00" + p.ID
		if seen[key] || len(result.Checks) >= 128 {
			return
		}
		p.ID = "check-" + digest(source + "\x00" + p.ID)[:20]
		p.Kind = "ci"
		if p.TimeoutSeconds == 0 {
			p.TimeoutSeconds = 1800
		}
		result.Checks = append(result.Checks, ProjectCheck{Pipeline: p, Source: source})
	}
	// Explicit CI definitions remain authoritative commands and share their existing
	// runner, environment, and working-directory semantics.
	file, err := loadProjectConfigFile(root)
	if err != nil {
		result.Warnings = append(result.Warnings, err.Error())
	} else if raw := strings.TrimSpace(string(file.fields["pipelines"])); raw != "" && raw != "null" {
		pipelines, err := s.Pipelines(path)
		if err != nil {
			result.Warnings = append(result.Warnings, "Pipeline discovery: "+err.Error())
		} else {
			inputs = append(inputs, []byte(pipelines.Version)...)
			for _, p := range pipelines.Pipelines {
				if p.Kind == "ci" {
					add(p, ".repsync.json")
				}
			}
		}
	}
	rootManager := "npm"
	if body, err := readProjectConfig(root, "package.json"); err == nil {
		var pkg struct {
			PackageManager string `json:"packageManager"`
		}
		if json.Unmarshal(body, &pkg) == nil {
			if manager, err := projectPackageManager(root, pkg.PackageManager); err == nil {
				rootManager = manager
			}
		}
	}
	if slices.ContainsFunc(names, func(name string) bool { return strings.HasPrefix(name, ".moon/workspace.") }) {
		checks, input, err := moonProjectChecks(path)
		if err != nil {
			result.Warnings = append(result.Warnings, err.Error())
		} else {
			inputs = append(inputs, input...)
			for _, p := range checks {
				add(p, "Moon")
			}
		}
	}
	for _, name := range names {
		base := filepath.Base(name)
		tool := checkConfigurationTool(base)
		guidance := slices.Contains([]string{"AGENTS.md", "CLAUDE.md", "GEMINI.md", "CONTRIBUTING.md", "README.md", "DEVELOPMENT.md"}, base)
		isPackage := base == "package.json"
		isMake := base == "Makefile" || base == "GNUmakefile"
		if tool == "" && !guidance && !isPackage && !isMake && base != ".repsync.json" {
			continue
		}
		body, err := readProjectConfig(root, name)
		if err != nil {
			result.Warnings = append(result.Warnings, name+": "+err.Error())
			continue
		}
		inputs = append(inputs, []byte(name+"\x00")...)
		inputs = append(inputs, body...)
		if guidance {
			result.Guidance = append(result.Guidance, name)
		}
		if tool != "" {
			if base == "pyproject.toml" {
				// The project owns tool settings; inventory their table names, not a guessed
				// rule list or an executable interpretation of the TOML.
				var doc map[string]any
				if err := toml.Unmarshal(body, &doc); err != nil {
					result.Warnings = append(result.Warnings, name+": invalid TOML")
					continue
				}
				if tools, ok := doc["tool"].(map[string]any); ok {
					for _, key := range slices.Sorted(maps.Keys(tools)) {
						result.Configurations = append(result.Configurations, CheckConfiguration{Path: name, Tool: key})
					}
				}
			} else {
				result.Configurations = append(result.Configurations, CheckConfiguration{Path: name, Tool: tool})
			}
		}
		if isPackage || isMake {
			tasks, err := manifestTasks(root, name, body, rootManager)
			if err != nil {
				result.Warnings = append(result.Warnings, err.Error())
				continue
			}
			for _, task := range tasks {
				if isCheckCommand(task.Name, task.Definition) {
					add(task, name)
				}
			}
		}
		if base == ".repsync.json" && name == base {
			var file map[string]json.RawMessage
			if json.Unmarshal(body, &file) == nil && len(file["policies"]) > 0 && string(file["policies"]) != "[]" && string(file["policies"]) != "null" {
				result.Warnings = append(result.Warnings, "Legacy repsync policies are not executed. Move constraints into project-owned check tools.")
			}
		}
	}
	if len(result.Checks) >= 128 {
		result.Warnings = append(result.Warnings, "Check discovery reached 128 commands; additional commands may be omitted.")
	}
	if len(result.Configurations) == 0 && len(result.Checks) == 0 {
		result.Warnings = append(result.Warnings, "No check configuration or command was discovered.")
	}
	resolved, _ := json.Marshal(result)
	result.Version = digest(string(inputs) + string(resolved))
	return result, nil
}

func checkConfigurationTool(base string) string {
	switch {
	case strings.HasPrefix(base, "eslint.config.") || strings.HasPrefix(base, ".eslintrc"):
		return "ESLint"
	case strings.HasPrefix(base, "tsconfig") && strings.HasSuffix(base, ".json"):
		return "TypeScript"
	case base == "biome.json" || base == "biome.jsonc":
		return "Biome"
	case base == "ruff.toml" || base == ".ruff.toml":
		return "Ruff"
	case base == "pyproject.toml":
		return "Python tools"
	case strings.HasPrefix(base, ".golangci."):
		return "golangci-lint"
	case base == "go.mod":
		return "Go module"
	case strings.HasPrefix(base, ".prettierrc") || strings.HasPrefix(base, "prettier.config."):
		return "Prettier"
	case base == "mypy.ini" || base == ".mypy.ini":
		return "mypy"
	case base == "pytest.ini":
		return "pytest"
	case base == "tox.ini":
		return "tox"
	case base == ".editorconfig":
		return "EditorConfig"
	case strings.HasPrefix(base, ".swiftlint."):
		return "SwiftLint"
	default:
		return ""
	}
}

func projectCheckFiles(path string, root *os.Root) ([]string, error) {
	if _, err := root.Stat(".git"); err == nil {
		output, err := git(path, "ls-files", "-z", "--cached", "--others", "--exclude-standard")
		if err != nil {
			return nil, err
		}
		names := strings.Split(output, "\x00")
		names = slices.DeleteFunc(names, func(name string) bool { return name == "" || !filepath.IsLocal(name) })
		slices.Sort(names)
		return slices.Compact(names), nil
	}
	names := []string{}
	entries := 0
	err := fs.WalkDir(root.FS(), ".", func(name string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		entries++
		if entries > 12000 {
			return errors.New("Check discovery reached 12000 filesystem entries; results are partial")
		}
		if d.IsDir() {
			if name != "." && (strings.Count(name, "/") >= 5 || slices.Contains([]string{".git", "node_modules", "vendor", "build", "dist", "target", ".venv", "venv", ".next", ".mygo", "__pycache__", ".cache"}, d.Name())) {
				return fs.SkipDir
			}
			return nil
		}
		if d.Type().IsRegular() {
			names = append(names, name)
		}
		return nil
	})
	slices.Sort(names)
	return names, err
}

func moonProjectChecks(path string) ([]Pipeline, []byte, error) {
	tasks, input, err := queryMoonPipelineTasks(path, []string{"--type", "test"})
	if err != nil {
		return nil, nil, err
	}
	result := []Pipeline{}
	for _, task := range tasks {
		_, id, _ := strings.Cut(task.Name, ":")
		if task.Kind == "ci" && !mutatingCheckName.MatchString(id) {
			result = append(result, task)
		}
	}
	return result, input, nil
}

func (s *Service) StartProjectCheck(path, id, version string) (PipelineRun, error) {
	overview, err := s.ProjectChecks(path)
	if err != nil {
		return PipelineRun{}, err
	}
	if overview.Version != version {
		return PipelineRun{}, errors.New("check configuration changed; refresh before running")
	}
	index := slices.IndexFunc(overview.Checks, func(check ProjectCheck) bool { return check.Pipeline.ID == id })
	if index < 0 {
		return PipelineRun{}, errors.New("check command no longer exists; refresh")
	}
	pipeline := overview.Checks[index].Pipeline
	if err := validatePipelines([]Pipeline{pipeline}); err != nil {
		return PipelineRun{}, err
	}
	return s.startPipeline(path, id, PipelineConfig{Version: version, Pipelines: []Pipeline{pipeline}})
}
