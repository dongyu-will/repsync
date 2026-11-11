package workspace

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"time"
)

// AIResource describes discovery and project intent, never model compliance.
type AIResource struct {
	ID            string   `json:"id"`
	Kind          string   `json:"kind"`
	Name          string   `json:"name"`
	Description   string   `json:"description"`
	Source        string   `json:"source"`
	Location      string   `json:"location"`
	Version       string   `json:"version"`
	Availability  string   `json:"availability"`
	Detail        string   `json:"detail"`
	Applicability string   `json:"applicability"`
	Reason        string   `json:"reason"`
	Tasks         string   `json:"tasks"`
	Projects      []string `json:"projects"`
}

type AIResourceReport struct {
	Resources []AIResource `json:"resources"`
	Issues    []string     `json:"issues"`
	CheckedAt string       `json:"checkedAt"`
	Scope     string       `json:"scope"`
}

// AIResourceAdoption is explicit project intent maintained by the project's AI.
type AIResourceAdoption struct {
	Kind   string `json:"kind"`
	Name   string `json:"name"`
	Source string `json:"source,omitempty"`
	Reason string `json:"reason"`
	Tasks  string `json:"tasks"`
}

type aiPaths struct{ home, codex string }

type aiProjectInfo struct {
	project   Project
	adoptions []AIResourceAdoption
	guide     []byte
	goProject bool
}

func aiProjectInfos(projects []Project, issues *[]string) []aiProjectInfo {
	infos := []aiProjectInfo{}
	for _, project := range projects {
		info := aiProjectInfo{project: project, adoptions: readAIIntent(project.Path, issues)}
		root, err := os.OpenRoot(project.Path)
		if err == nil {
			var guideErr error
			info.guide, guideErr = readProjectConfig(root, "AGENTS.md")
			if guideErr != nil && !errors.Is(guideErr, os.ErrNotExist) {
				*issues = append(*issues, "AGENTS.md unreadable; project fit evidence incomplete: "+project.Path)
			}
			_, err := root.Stat("go.mod")
			info.goProject = err == nil
			root.Close()
		}
		infos = append(infos, info)
	}
	return infos
}

func currentAIPaths() (aiPaths, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return aiPaths{}, fmt.Errorf("read user directory: %w", err)
	}
	codex := os.Getenv("CODEX_HOME")
	if codex == "" {
		codex = filepath.Join(home, ".codex")
	}
	if !filepath.IsAbs(codex) {
		return aiPaths{}, errors.New("CODEX_HOME must be an absolute path")
	}
	return aiPaths{home: home, codex: codex}, nil
}

func aiID(kind, source string) string {
	sum := sha256.Sum256([]byte(source))
	return kind + ":" + hex.EncodeToString(sum[:12])
}

func skillID(path string) string {
	if resolved, err := filepath.EvalSymlinks(path); err == nil {
		path = resolved
	}
	return aiID("skill", filepath.Clean(path))
}

// AIResources inventories files and executable paths. Only Codex's configuration-listing
// command is run; MCP servers and project commands are never started.
func (s *Service) AIResources(path string) (AIResourceReport, error) {
	projects, err := s.List()
	if err != nil {
		return AIResourceReport{}, err
	}
	if path != "" {
		project, err := s.registeredProject(path)
		if err != nil {
			return AIResourceReport{}, err
		}
		projects = []Project{project}
	}
	truncated := len(projects) > 64
	if truncated {
		projects = projects[:64]
	}
	paths, err := currentAIPaths()
	if err != nil {
		return AIResourceReport{}, err
	}
	report := inventoryAIResources(paths, projects, path)
	if truncated {
		report.Issues = append(report.Issues, "Only the first 64 registered projects are inventoried")
	}
	directory := paths.home
	if path != "" {
		directory = path
	}
	resources, err := readCodexMCP(directory)
	if err != nil {
		report.Issues = append(report.Issues, err.Error())
	} else {
		infos := aiProjectInfos(projects, &report.Issues)
		for _, resource := range resources {
			applyAIIntent(&resource, infos, path)
			report.Resources = append(report.Resources, resource)
		}
	}
	completeAIInventory(&report, projects)
	return report, nil
}

func completeAIInventory(report *AIResourceReport, projects []Project) {
	for _, info := range aiProjectInfos(projects, &report.Issues) {
		for _, adoption := range info.adoptions {
			matches := []int{}
			for i, resource := range report.Resources {
				if resource.Kind == adoption.Kind && resource.Name == adoption.Name && (adoption.Source == "" || resource.Location == adoption.Source) {
					matches = append(matches, i)
				}
			}
			if len(matches) == 0 {
				report.Resources = append(report.Resources, AIResource{ID: aiID(adoption.Kind, info.project.Path+":"+adoption.Name+":"+adoption.Source), Kind: adoption.Kind, Name: adoption.Name, Source: "Project .repsync.json", Location: adoption.Source, Availability: "missing", Detail: "Declared by the project, but no matching source found; check the name, source, and scan gaps", Applicability: "adopted", Reason: adoption.Reason, Tasks: adoption.Tasks, Projects: []string{info.project.Path}})
			}
			if len(matches) > 1 && adoption.Source == "" {
				report.Issues = append(report.Issues, "Resource named "+adoption.Name+" has multiple sources; specify source in the project's aiResources")
				for _, i := range matches {
					report.Resources[i].Applicability, report.Resources[i].Reason = "candidate", "Same-name resource sources need review; project adoption is uncertain"
					report.Resources[i].Projects = slices.DeleteFunc(report.Resources[i].Projects, func(path string) bool { return path == info.project.Path })
				}
			}
		}
	}
	slices.Sort(report.Issues)
	report.Issues = slices.Compact(report.Issues)
	sortAIResources(report.Resources)
	budget := 3 * 1024 * 1024
	for i, resource := range report.Resources {
		encoded, err := json.Marshal(resource)
		if err != nil || len(encoded) > budget {
			report.Resources = report.Resources[:i]
			report.Issues = append(report.Issues, "Resource response reached the 3 MiB limit; list incomplete")
			break
		}
		budget -= len(encoded)
	}
}

func inventoryAIResources(paths aiPaths, projects []Project, scope string) AIResourceReport {
	report := AIResourceReport{Resources: []AIResource{}, Issues: []string{}, CheckedAt: time.Now().UTC().Format(time.RFC3339), Scope: scope}
	infos := aiProjectInfos(projects, &report.Issues)
	type source struct {
		path, label string
		depth       int
	}
	sources := []source{
		{filepath.Join(paths.home, ".agents", "skills"), "User Skill", 3},
		{filepath.Join(paths.codex, "skills"), "Codex Skill", 3},
		{filepath.Join(paths.home, ".claude", "skills"), "Claude Skill", 3},
		{filepath.Join(paths.codex, "plugins", "cache"), "Plugin cache (enabled state unknown)", 9},
	}
	for _, project := range projects {
		for _, directory := range []string{".agents/skills", ".codex/skills", ".claude/skills"} {
			sources = append(sources, source{filepath.Join(project.Path, directory), "Project Skill", 3})
		}
	}
	seen := map[string]bool{}
	nodes := 0
	deadline := time.Now().Add(4 * time.Second)
	for _, source := range sources {
		if _, err := os.Stat(source.path); errors.Is(err, os.ErrNotExist) {
			continue
		}
		var walk func(string, int)
		walk = func(directory string, depth int) {
			if depth > source.depth || nodes >= 8000 || len(report.Resources) >= 600 || time.Now().After(deadline) {
				return
			}
			entries, err := os.ReadDir(directory)
			if err != nil {
				report.Issues = append(report.Issues, "Failed to read Skill directory: "+directory)
				return
			}
			for _, entry := range entries {
				nodes++
				if nodes > 8000 || len(report.Resources) >= 600 || time.Now().After(deadline) {
					return
				}
				location := filepath.Join(directory, entry.Name())
				if entry.Name() == "SKILL.md" {
					canonical, err := filepath.EvalSymlinks(location)
					if err != nil || seen[canonical] {
						continue
					}
					seen[canonical] = true
					resource := AIResource{ID: skillID(canonical), Kind: "skill", Name: filepath.Base(filepath.Dir(location)), Source: source.label, Location: canonical, Availability: "discovered", Detail: "File discovered; availability in the current AI session is unknown", Projects: []string{}}
					content, err := readText(canonical)
					if err != nil {
						resource.Availability, resource.Detail = "error", err.Error()
					} else {
						resource.Version = textVersion(content)
						if name := skillField(content, "name"); name != "" {
							if len(name) <= 200 {
								resource.Name = name
							} else {
								resource.Availability, resource.Detail = "error", "Skill name exceeds the 200 byte limit"
							}
						}
						resource.Description = truncateAIText(skillField(content, "description"), 2000)
					}
					applyAIIntent(&resource, infos, scope)
					report.Resources = append(report.Resources, resource)
					continue
				}
				if slices.Contains([]string{"node_modules", ".git", "vendor", ".venv"}, entry.Name()) {
					continue
				}
				if entry.IsDir() || entry.Type()&fs.ModeSymlink != 0 {
					// Depth bounds also protect against symlink cycles.
					if info, err := os.Stat(location); err == nil && info.IsDir() {
						walk(location, depth+1)
					}
				}
			}
		}
		walk(source.path, 0)
	}
	if nodes >= 8000 || len(report.Resources) >= 600 || time.Now().After(deadline) {
		report.Issues = append(report.Issues, "Skill inventory reached 600 resources, 8000 directory entries, or 4 seconds; list incomplete")
	}
	cliNames := []string{"codex", "git", "gh", "node", "npm", "pnpm", "go", "python3", "docker", "wrangler"}
	for _, project := range infos {
		for _, adoption := range project.adoptions {
			if adoption.Kind == "cli" && adoption.Name != "" {
				cliNames = append(cliNames, adoption.Name)
			}
		}
	}
	slices.Sort(cliNames)
	cliNames = slices.Compact(cliNames)
	for _, name := range cliNames {
		directory := paths.home
		if scope != "" {
			directory = scope
		}
		resource := AIResource{ID: "cli:" + name, Kind: "cli", Name: name, Source: "repsync execution environment", Availability: "missing", Detail: "Executable not found", Projects: []string{}}
		if command, err := projectCommand(context.Background(), directory, "", nil, []string{name}, false); err == nil {
			resource.Location, resource.Availability = command.Path, "available"
			resource.ID = aiID("cli", command.Path)
			resource.Detail = "Executable located; version, authentication, and AI session environment not verified"
		}
		applyAIIntent(&resource, infos, scope)
		report.Resources = append(report.Resources, resource)
	}
	report.Issues = slices.Compact(report.Issues)
	sortAIResources(report.Resources)
	return report
}

func textVersion(text string) string {
	sum := sha256.Sum256([]byte(text))
	return hex.EncodeToString(sum[:])
}

func truncateAIText(value string, limit int) string {
	if len(value) <= limit {
		return value
	}
	runes := []rune(value)
	if len(runes) <= limit {
		return value
	}
	return string(runes[:limit]) + "…"
}

func skillField(content, key string) string {
	lines := strings.Split(strings.ReplaceAll(content, "\r\n", "\n"), "\n")
	if len(lines) < 3 || strings.TrimSpace(lines[0]) != "---" {
		return ""
	}
	for i := 1; i < len(lines) && lines[i] != "---"; i++ {
		if value, ok := strings.CutPrefix(lines[i], key+":"); ok {
			value = strings.TrimSpace(value)
			if slices.Contains([]string{"|", ">", "|-", ">-"}, value) {
				value = ""
				for j := i + 1; j < len(lines) && strings.HasPrefix(lines[j], " "); j++ {
					value += " " + strings.TrimSpace(lines[j])
				}
			}
			return strings.Trim(strings.TrimSpace(value), "\"'")
		}
	}
	return ""
}

func readAIIntent(path string, issues *[]string) []AIResourceAdoption {
	root, err := os.OpenRoot(path)
	if err != nil {
		if issues != nil {
			*issues = append(*issues, "Failed to read project resource configuration: "+path)
		}
		return nil
	}
	defer root.Close()
	data, err := readProjectConfig(root, ".repsync.json")
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	var config struct {
		AIResources []AIResourceAdoption `json:"aiResources"`
	}
	if err == nil {
		err = json.Unmarshal(data, &config)
	}
	if err != nil || len(config.AIResources) > 128 {
		if issues != nil {
			*issues = append(*issues, "Invalid AI resource configuration or more than 128 entries: "+path)
		}
		return nil
	}
	for i, adoption := range config.AIResources {
		if !slices.Contains([]string{"skill", "mcp", "cli"}, adoption.Kind) || strings.TrimSpace(adoption.Name) == "" || len(adoption.Name) > 200 || len(adoption.Reason) > 2000 || len(adoption.Tasks) > 2000 || adoption.Source != "" && !filepath.IsAbs(adoption.Source) {
			if issues != nil {
				*issues = append(*issues, "AI resource declarations require valid kind/name; source must be absolute when provided: "+path)
			}
			return nil
		}
		if adoption.Kind == "cli" && !regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._-]*$`).MatchString(adoption.Name) {
			if issues != nil {
				*issues = append(*issues, "CLI name must be a command name without paths or arguments: "+path)
			}
			return nil
		}
		if adoption.Source != "" {
			config.AIResources[i].Source = filepath.Clean(adoption.Source)
			if adoption.Kind == "skill" {
				if resolved, err := filepath.EvalSymlinks(adoption.Source); err == nil {
					config.AIResources[i].Source = resolved
				}
			}
		}
	}
	return config.AIResources
}

func applyAIIntent(resource *AIResource, projects []aiProjectInfo, scope string) {
	resource.Applicability, resource.Reason = "unassigned", "No project fit evidence yet; choose per task"
	for _, project := range projects {
		adopted := false
		for _, adoption := range project.adoptions {
			if adoption.Kind != resource.Kind || adoption.Name != resource.Name {
				continue
			}
			if adoption.Source != "" && filepath.Clean(adoption.Source) != resource.Location {
				continue
			}
			resource.Applicability, resource.Reason, resource.Tasks = "adopted", adoption.Reason, adoption.Tasks
			if resource.Reason == "" {
				resource.Reason = "Explicitly adopted in .repsync.json; fit rationale not provided"
			}
			adopted = true
		}
		if len(project.guide) > 0 && resource.Kind == "skill" {
			pattern := `(^|[^A-Za-z0-9_-])` + regexp.QuoteMeta(resource.Name) + `($|[^A-Za-z0-9_-])`
			if regexp.MustCompile(pattern).Match(project.guide) {
				if !adopted {
					resource.Applicability, resource.Reason = "recommended", "Explicitly mentioned in project AGENTS.md; follow its task conditions"
				}
				adopted = true
			}
		}
		if adopted {
			resource.Projects = append(resource.Projects, project.project.Path)
		}
		if !adopted && scope != "" && resource.Applicability == "unassigned" {
			if project.goProject && (strings.HasPrefix(resource.Name, "golang-") || resource.Kind == "cli" && resource.Name == "go") {
				resource.Applicability, resource.Reason, resource.Tasks = "candidate", "go.mod detected; stack-based candidate, not declared as adopted", "Choose only for relevant Go tasks"
			}
		}
	}
	resource.Projects = slices.Compact(resource.Projects)
}

func sortAIResources(resources []AIResource) {
	priority := map[string]int{"adopted": 0, "recommended": 1, "candidate": 2, "unassigned": 3}
	slices.SortStableFunc(resources, func(a, b AIResource) int {
		if delta := priority[a.Applicability] - priority[b.Applicability]; delta != 0 {
			return delta
		}
		return strings.Compare(a.Kind+":"+a.Name+":"+a.Location, b.Kind+":"+b.Name+":"+b.Location)
	})
}

func readCodexMCP(directory string) ([]AIResource, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 6*time.Second)
	defer cancel()
	cmd, err := projectCommand(ctx, directory, "", nil, []string{"codex", "mcp", "list", "--json"}, false)
	if err != nil {
		return nil, errors.New("Codex CLI not found; MCP configuration not inventoried; connection and session visibility unknown")
	}
	var stdout, stderr limitedBuffer
	cmd.Stdout, cmd.Stderr = &stdout, &stderr
	configureCommandProcess(cmd, false)
	if err := cmd.Run(); err != nil {
		return nil, errors.New("Codex MCP configuration request failed or timed out; no MCP servers started")
	}
	if stdout.overflow {
		return nil, errors.New("Codex MCP configuration response exceeds size limits")
	}
	var servers []struct {
		Name      string `json:"name"`
		Enabled   bool   `json:"enabled"`
		Transport struct {
			Type    string `json:"type"`
			Command string `json:"command"`
		} `json:"transport"`
	}
	if err := json.Unmarshal(stdout.Bytes(), &servers); err != nil {
		return nil, errors.New("unrecognized MCP configuration format from the current Codex CLI")
	}
	if len(servers) > 128 {
		return nil, errors.New("MCP configuration exceeds 128 entries")
	}
	resources := []AIResource{}
	for _, server := range servers {
		resource := AIResource{ID: "mcp:codex:" + server.Name, Kind: "mcp", Name: server.Name, Source: "Current Codex CLI configuration", Availability: "configured", Detail: "Enabled in configuration; connection, authentication, and current AI session visibility not verified", Location: "Codex MCP / " + server.Name, Projects: []string{}}
		if !server.Enabled {
			resource.Availability, resource.Detail = "disabled", "Disabled in CLI configuration; desktop plugins and historical sessions may differ"
		}
		resources = append(resources, resource)
	}
	return resources, nil
}
