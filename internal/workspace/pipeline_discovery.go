package workspace

import (
	"context"
	"encoding/json"
	"fmt"
	"maps"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"time"
)

var deliveryTaskName = regexp.MustCompile(`^(?:cd|deploy|release|publish)(?:$|[:-])`)
var buildTaskName = regexp.MustCompile(`^build(?:$|[:-])`)
var interactiveTaskName = regexp.MustCompile(`(?:^|[:-])(?:fix|write|watch|update|dev)(?:$|[:-])`)

func detectedPipelineKind(name, definition string) string {
	if interactiveTaskName.MatchString(name) || mutatingCheckFlags.MatchString(definition) {
		return ""
	}
	if deliveryTaskName.MatchString(name) {
		return "cd"
	}
	if isCheckCommand(name, definition) || buildTaskName.MatchString(name) {
		return "ci"
	}
	return ""
}

// manifestTasks returns the project's invocation and original declaration. It
// does not split shell syntax into invented stages or execute discovery targets.
func manifestTasks(root *os.Root, source string, body []byte, inheritedManager string) ([]Pipeline, error) {
	cwd := filepath.ToSlash(filepath.Dir(source))
	if cwd == "." {
		cwd = ""
	}
	result := []Pipeline{}
	add := func(name, definition string, command []string) {
		result = append(result, Pipeline{ID: name, Name: name, Steps: [][]string{command}, Cwd: cwd, Source: source, Definition: definition})
	}
	if filepath.Base(source) == "package.json" {
		var pkg struct {
			Scripts        map[string]string `json:"scripts"`
			PackageManager string            `json:"packageManager"`
		}
		if err := json.Unmarshal(body, &pkg); err != nil {
			return nil, fmt.Errorf("%s: invalid JSON", source)
		}
		manager := inheritedManager
		if pkg.PackageManager != "" {
			var err error
			manager, err = projectPackageManager(root, pkg.PackageManager)
			if err != nil {
				return nil, err
			}
		}
		for _, name := range slices.Sorted(maps.Keys(pkg.Scripts)) {
			add(name, pkg.Scripts[name], []string{manager, "run", name})
		}
		return result, nil
	}
	definitions := map[string]string{}
	targets := []string{}
	for _, line := range strings.Split(string(body), "\n") {
		if strings.HasPrefix(line, "\t") {
			for _, target := range targets {
				definitions[target] += line + "\n"
			}
			continue
		}
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		target, rest, ok := strings.Cut(line, ":")
		if !ok || strings.HasPrefix(rest, "=") || strings.ContainsAny(target, "=$()%") {
			targets = nil
			continue
		}
		targets = strings.Fields(target)
		for _, name := range targets {
			if _, ok := definitions[name]; !ok {
				definitions[name] = ""
			}
		}
	}
	for _, name := range slices.Sorted(maps.Keys(definitions)) {
		add(name, definitions[name], []string{"make", name})
	}
	return result, nil
}

func discoverPipelines(path string, root *os.Root) ([]Pipeline, []byte, []string, error) {
	names, scanErr := projectCheckFiles(path, root)
	warnings := []string{}
	if scanErr != nil {
		warnings = append(warnings, scanErr.Error())
	}
	inputs := []byte{}
	pipelines := []Pipeline{}
	seen := map[string]bool{}
	add := func(p Pipeline) {
		key := p.Source + "\x00" + p.ID
		if seen[key] {
			return
		}
		seen[key] = true
		if len(pipelines) >= 128 {
			return
		}
		p.ID = "pipeline-" + digest(key)[:20]
		p.TimeoutSeconds = 1800
		pipelines = append(pipelines, p)
	}
	manager := "npm"
	if body, err := readProjectConfig(root, "package.json"); err == nil {
		var pkg struct {
			PackageManager string `json:"packageManager"`
		}
		if json.Unmarshal(body, &pkg) == nil {
			var err error
			manager, err = projectPackageManager(root, pkg.PackageManager)
			if err != nil {
				return nil, nil, nil, err
			}
		}
	}
	if slices.ContainsFunc(names, func(name string) bool { return strings.HasPrefix(name, ".moon/workspace.") }) {
		for _, filter := range [][]string{{"--type", "^(test|build)$"}, {"--id", "^(cd|deploy|release|publish)(:|-|$)"}} {
			tasks, input, err := queryMoonPipelineTasks(path, filter)
			if err != nil {
				warnings = append(warnings, err.Error())
				continue
			}
			inputs = append(inputs, input...)
			for _, p := range tasks {
				add(p)
			}
		}
	}
	for _, name := range names {
		base := filepath.Base(name)
		if base != "package.json" && base != "Makefile" && base != "GNUmakefile" {
			continue
		}
		body, err := readProjectConfig(root, name)
		if err != nil {
			warnings = append(warnings, name+": "+err.Error())
			continue
		}
		inputs = append(inputs, []byte(name+"\x00")...)
		inputs = append(inputs, body...)
		tasks, err := manifestTasks(root, name, body, manager)
		if err != nil {
			warnings = append(warnings, err.Error())
			continue
		}
		for _, p := range tasks {
			p.Kind = detectedPipelineKind(p.Name, p.Definition)
			if p.Kind != "" {
				add(p)
			}
		}
	}
	if len(pipelines) >= 128 {
		warnings = append(warnings, "Pipeline discovery reached 128 entries; additional entries may be omitted.")
	}
	return pipelines, inputs, warnings, nil
}

func queryMoonPipelineTasks(path string, filters []string) ([]Pipeline, []byte, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	args := append([]string{"moon", "query", "tasks"}, filters...)
	cmd, err := projectCommand(ctx, path, "", nil, args, false)
	if err != nil {
		return nil, nil, err
	}
	var output, diagnostics limitedBuffer
	cmd.Stdout, cmd.Stderr = &output, &diagnostics
	configureCommandProcess(cmd, false)
	if err := cmd.Run(); err != nil {
		return nil, nil, fmt.Errorf("Moon pipeline discovery failed (%v); inspect moon query tasks", err)
	}
	if output.overflow {
		return nil, nil, fmt.Errorf("Moon pipeline graph exceeds 1 MiB; discovery is incomplete")
	}
	var graph struct {
		Tasks map[string]map[string]struct {
			Command string   `json:"command"`
			Args    []string `json:"args"`
			Type    string   `json:"type"`
			Options struct {
				Persistent bool `json:"persistent"`
				Internal   bool `json:"internal"`
			} `json:"options"`
		} `json:"tasks"`
	}
	if err := json.Unmarshal(output.Bytes(), &graph); err != nil {
		return nil, nil, fmt.Errorf("Moon pipeline graph: %w", err)
	}
	result := []Pipeline{}
	for _, project := range slices.Sorted(maps.Keys(graph.Tasks)) {
		for _, name := range slices.Sorted(maps.Keys(graph.Tasks[project])) {
			task := graph.Tasks[project][name]
			if len(filters) == 2 && filters[0] == "--type" {
				if filters[1] == "test" && task.Type != "test" {
					continue
				}
				if filters[1] != "test" && task.Type != "test" && task.Type != "build" {
					continue
				}
			}
			if len(filters) == 2 && filters[0] == "--id" && !deliveryTaskName.MatchString(name) {
				continue
			}
			if task.Options.Persistent || task.Options.Internal || interactiveTaskName.MatchString(name) {
				continue
			}
			definition := strings.Join(append([]string{task.Command}, task.Args...), " ")
			if mutatingCheckFlags.MatchString(definition) {
				continue
			}
			kind := detectedPipelineKind(name, definition)
			if kind == "" && (task.Type == "test" || task.Type == "build") {
				kind = "ci"
			}
			if kind == "" {
				continue
			}
			target := project + ":" + name
			result = append(result, Pipeline{ID: target, Name: target, Kind: kind, Source: "Moon", Steps: [][]string{{"moon", "run", target}}, Definition: definition})
		}
	}
	return result, output.Bytes(), nil
}
