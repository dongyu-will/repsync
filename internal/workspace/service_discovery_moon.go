package workspace

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"os"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"time"
)

// Moon owns config inheritance and task expansion. Query its resolved graph instead
// of implementing a second YAML/task engine or executing Make during discovery.
type moonRuntimeTask struct {
	Command string   `json:"command"`
	Args    []string `json:"args"`
	Options struct {
		Persistent bool `json:"persistent"`
		Internal   bool `json:"internal"`
	} `json:"options"`
}

var moonRunLine = regexp.MustCompile(`^moon run ([A-Za-z0-9_-]+):(dev|start|dev-infra)$`)
var makeNumber = regexp.MustCompile(`^([A-Za-z_][A-Za-z0-9_]*)\s*(?:\?=|:=|=)\s*([0-9]+)\s*(?:#.*)?$`)
var makePortReference = regexp.MustCompile(`(?:--port\s+|\b[A-Za-z_]*PORT=)["']?\$\(([A-Za-z_][A-Za-z0-9_]*)\)`)

func discoverMoonServices(path string, root *os.Root) ([]LocalServiceDefinition, []byte, bool, error) {
	found := false
	for _, suffix := range []string{"yml", "yaml", "json", "jsonc", "toml", "pkl", "hcl"} {
		if _, err := root.Stat(".moon/workspace." + suffix); err == nil {
			found = true
			break
		} else if !errors.Is(err, os.ErrNotExist) {
			return nil, nil, true, err
		}
	}
	if !found {
		return nil, nil, false, nil
	}
	readme, err := readProjectConfig(root, "README.md")
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return nil, nil, true, err
	}
	// Only standalone commands in shell fences declare the default runtime. Inline
	// optional examples and setup/migration/build tasks are never added to startup.
	targets := []string{}
	fence := ""
	for _, line := range strings.Split(string(readme), "\n") {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "```") {
			if fence != "" {
				fence = ""
			} else if slices.Contains([]string{"```bash", "```sh", "```shell", "```console", "```"}, line) {
				fence = "shell"
			} else {
				fence = "other"
			}
			continue
		}
		if fence == "shell" && moonRunLine.MatchString(line) {
			target := strings.TrimPrefix(line, "moon run ")
			if !slices.Contains(targets, target) {
				targets = append(targets, target)
			}
		}
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	cmd, err := projectCommand(ctx, path, "", nil, []string{"moon", "query", "tasks", "--id", "^(dev|start|dev-infra)$"}, false)
	if err != nil {
		return nil, nil, true, fmt.Errorf("discover Moon runtime: %w", err)
	}
	var out, diagnostics limitedBuffer
	cmd.Stdout, cmd.Stderr = &out, &diagnostics
	if err := cmd.Run(); err != nil {
		return nil, nil, true, fmt.Errorf("Moon runtime query failed (%v); check moon query tasks in the project", err)
	}
	if out.overflow {
		return nil, nil, true, errors.New("Moon runtime graph exceeds 1 MiB")
	}
	var graph struct {
		Tasks map[string]map[string]moonRuntimeTask `json:"tasks"`
	}
	if err := json.Unmarshal(out.Bytes(), &graph); err != nil {
		return nil, nil, true, fmt.Errorf("read Moon runtime graph: %w", err)
	}
	makefile, err := readProjectConfig(root, "Makefile")
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return nil, nil, true, err
	}
	tasks := map[string]moonRuntimeTask{}
	for project, group := range graph.Tasks {
		for name, task := range group {
			if (name == "dev" || name == "start" || name == "dev-infra") && !task.Options.Internal {
				tasks[project+":"+name] = task
			}
		}
	}
	// Without a documented selection, only exact dev server tasks with a known
	// listener are unambiguous enough to join the default runtime.
	if len(targets) == 0 {
		for target, task := range tasks {
			if strings.HasSuffix(target, ":dev") && task.Options.Persistent && moonTaskPort(task, string(makefile)) > 0 {
				targets = append(targets, target)
			}
		}
		slices.Sort(targets)
	}
	services := []LocalServiceDefinition{}
	setup := []string{}
	for _, target := range targets {
		task, ok := tasks[target]
		if !ok {
			return nil, nil, true, fmt.Errorf("README runtime target %s is missing from the Moon graph", target)
		}
		if !task.Options.Persistent && !strings.HasSuffix(target, ":dev-infra") {
			continue
		}
		d := LocalServiceDefinition{ID: target, Name: target, Command: []string{"moon", "run", target}}
		if strings.HasSuffix(target, ":dev-infra") {
			d.OneShot = true
			setup = append(setup, target)
		} else {
			d.Port = moonTaskPort(task, string(makefile))
			d.FixedPort = d.Port > 0
			if d.Port > 0 {
				d.URL = fmt.Sprintf("http://127.0.0.1:%d", d.Port)
			}
		}
		services = append(services, d)
	}
	// Environment URLs supply endpoint metadata only. Credentials, paths, queries,
	// and original URLs never enter definitions, logs, or persisted service records.
	env, err := readProjectConfig(root, ".env")
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return nil, nil, true, err
	}
	observed := moonLocalDependencies(string(env))
	for i := range observed {
		observed[i].DependsOn = slices.Clone(setup)
	}
	prerequisites := slices.Clone(setup)
	for _, d := range observed {
		prerequisites = append(prerequisites, d.ID)
	}
	previous := ""
	for i := range services {
		if services[i].OneShot {
			continue
		}
		services[i].DependsOn = slices.Clone(prerequisites)
		// Follow the documented terminal order for the default applications. A known
		// listener supplies a readiness boundary; no fabricated environment injection.
		if previous != "" {
			services[i].DependsOn = append(services[i].DependsOn, previous)
		}
		if services[i].Port > 0 {
			previous = services[i].ID
		}
	}
	services = append(services, observed...)
	input := append(slices.Clone(readme), makefile...)
	resolved, _ := json.Marshal(services)
	input = append(input, resolved...)
	input = append(input, out.Bytes()...)
	return services, input, true, nil
}

func moonTaskPort(task moonRuntimeTask, makefile string) int {
	text := strings.Join(append([]string{task.Command}, task.Args...), " ")
	if task.Command == "make" && len(task.Args) == 1 {
		text = ""
		active := false
		for _, line := range strings.Split(makefile, "\n") {
			if !strings.HasPrefix(line, "\t") && strings.Contains(line, ":") && !strings.Contains(line, ":=") {
				target, _, _ := strings.Cut(line, ":")
				active = slices.Contains(strings.Fields(target), task.Args[0])
			}
			if active && strings.HasPrefix(line, "\t") {
				text += line + "\n"
			}
		}
	}
	ports := map[int]bool{}
	for _, match := range serviceScriptPort.FindAllStringSubmatch(text, -1) {
		n, _ := strconv.Atoi(match[1])
		ports[n] = true
	}
	variables := map[string]string{}
	for _, line := range strings.Split(makefile, "\n") {
		if m := makeNumber.FindStringSubmatch(line); len(m) > 0 {
			variables[m[1]] = m[2]
		}
	}
	for _, m := range makePortReference.FindAllStringSubmatch(text, -1) {
		value := variables[m[1]]
		if override, ok := os.LookupEnv(m[1]); ok {
			value = override
		}
		n, err := strconv.Atoi(value)
		if err != nil || n < 1 || n > 65535 {
			return 0
		}
		ports[n] = true
	}
	if len(ports) == 1 {
		for port := range ports {
			if port > 0 && port <= 65535 {
				return port
			}
		}
	}
	return 0
}

func moonLocalDependencies(env string) []LocalServiceDefinition {
	ports := map[int]string{}
	for _, line := range strings.Split(env, "\n") {
		key, value, ok := strings.Cut(strings.TrimSpace(line), "=")
		if !ok || strings.HasPrefix(key, "#") {
			continue
		}
		u, err := url.Parse(strings.Trim(strings.TrimSpace(value), "\"'"))
		if err != nil || !slices.Contains([]string{"127.0.0.1", "localhost", "::1"}, u.Hostname()) {
			continue
		}
		scheme := strings.Split(u.Scheme, "+")[0]
		name := ""
		switch scheme {
		case "postgres", "postgresql":
			name = "PostgreSQL"
		case "redis", "rediss":
			name = "Redis"
		default:
			continue
		}
		port, _ := strconv.Atoi(u.Port())
		if port == 0 {
			if name == "PostgreSQL" {
				port = 5432
			} else {
				port = 6379
			}
		}
		if port > 0 && port <= 65535 {
			ports[port] = name
		}
	}
	numbers := []int{}
	for port := range ports {
		numbers = append(numbers, port)
	}
	slices.Sort(numbers)
	result := []LocalServiceDefinition{}
	for _, port := range numbers {
		result = append(result, LocalServiceDefinition{ID: fmt.Sprintf("dependency-%d", port), Name: fmt.Sprintf("%s · %d", ports[port], port), ObserveOnly: true, Port: port})
	}
	return result
}
