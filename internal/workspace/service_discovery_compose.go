package workspace

import (
	"fmt"
	"net"
	"path/filepath"
	"regexp"
	"slices"
	"strings"

	"go.yaml.in/yaml/v3"
)

var dockerSpawn = regexp.MustCompile(`\bspawnSync\(\s*['"]docker['"]\s*,\s*\[([^\]]+)\]`)
var jsLiteralArgument = regexp.MustCompile(`^\s*(['"])([^'"\r\n]*)['"]\s*$`)

func (r *packageRuntime) composeCommand(args []string, depth int) []string {
	if depth >= 8 || len(args) == 0 {
		return nil
	}
	if dir, name, ok := r.packageRun(args, ""); ok && dir == "" {
		words, ok := shellWords(r.pkg("").Scripts[name])
		if ok {
			return r.composeCommand(words, depth+1)
		}
		return nil
	}
	if len(args) >= 2 && args[0] == "docker" && args[1] == "compose" {
		return args[2:]
	}
	if len(args) == 2 && args[0] == "node" {
		body := string(r.read(args[1]))
		matches := dockerSpawn.FindAllStringSubmatch(body, -1)
		if len(matches) != 1 {
			return nil
		}
		arguments := []string{}
		for _, part := range strings.Split(matches[0][1], ",") {
			if strings.TrimSpace(part) == "" {
				continue
			}
			match := jsLiteralArgument.FindStringSubmatch(part)
			if len(match) == 0 {
				return nil
			}
			arguments = append(arguments, match[2])
		}
		if len(arguments) > 0 && arguments[0] == "compose" {
			return arguments[1:]
		}
	}
	return nil
}

func (r *packageRuntime) preparations(entry string) ([]LocalServiceDefinition, []LocalServiceDefinition) {
	readme := string(r.read("README.md"))
	var block [][]string
	shell := false
	directory := ""
	for _, line := range strings.Split(readme, "\n") {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "```") {
			block = nil
			directory = ""
			shell = !shell && slices.Contains([]string{"```", "```sh", "```bash", "```shell", "```console"}, line)
			continue
		}
		if !shell {
			continue
		}
		args, ok := shellWords(line)
		if !ok || len(args) == 0 {
			continue
		}
		if len(args) == 2 && args[0] == "cd" {
			directory = filepath.Clean(filepath.Join(directory, args[1]))
			if directory == "." {
				directory = ""
			}
			continue
		}
		if directory != "" {
			continue
		}
		if dir, name, ok := r.packageRun(args, directory); ok && dir == "" && name == entry {
			setup, observed := []LocalServiceDefinition{}, []LocalServiceDefinition{}
			seen := map[string]bool{}
			for _, command := range block {
				dir, name, ok := r.packageRun(command, "")
				if !ok || dir != "" || seen[name] {
					continue
				}
				compose := r.composeCommand(command, 0)
				dependencies, ok := r.composePorts(compose)
				if !ok {
					continue
				}
				seen[name] = true
				d := LocalServiceDefinition{ID: name, Name: name, Command: command, OneShot: true}
				if len(setup) > 0 {
					d.DependsOn = []string{setup[len(setup)-1].ID}
				}
				setup = append(setup, d)
				for _, dependency := range dependencies {
					if !slices.ContainsFunc(observed, func(existing LocalServiceDefinition) bool { return existing.Port == dependency.Port }) {
						observed = append(observed, dependency)
					}
				}
			}
			return setup, observed
		}
		block = append(block, args)
	}
	return nil, nil
}

// Only an explicitly documented, detached Compose up is a finite preparation
// step. Never add installs, migrations, down/reset commands or arbitrary scripts.
func (r *packageRuntime) composePorts(args []string) ([]LocalServiceDefinition, bool) {
	file := ""
	if len(args) >= 2 && (args[0] == "-f" || args[0] == "--file") {
		file, args = args[1], args[2:]
	}
	if len(args) == 0 || args[0] != "up" {
		return nil, false
	}
	detached := false
	selected := []string{}
	for _, arg := range args[1:] {
		switch arg {
		case "-d", "--detach":
			detached = true
		case "--wait":
		default:
			if strings.HasPrefix(arg, "-") {
				return nil, false
			}
			selected = append(selected, arg)
		}
	}
	if !detached {
		return nil, false
	}
	if file == "" {
		for _, candidate := range []string{"compose.yaml", "compose.yml", "docker-compose.yaml", "docker-compose.yml"} {
			if r.read(candidate) != nil {
				file = candidate
				break
			}
		}
	}
	if file == "" || !filepath.IsLocal(file) {
		return nil, false
	}
	var compose struct {
		Services map[string]struct {
			Ports    []yaml.Node `yaml:"ports"`
			Profiles []string    `yaml:"profiles"`
		} `yaml:"services"`
	}
	if err := yaml.Unmarshal(r.read(file), &compose); err != nil {
		r.err = fmt.Errorf("read %s: invalid Compose YAML", file)
		return nil, false
	}
	names := []string{}
	for name, service := range compose.Services {
		if slices.Contains(selected, name) || len(selected) == 0 && len(service.Profiles) == 0 {
			names = append(names, name)
		}
	}
	slices.Sort(names)
	result := []LocalServiceDefinition{}
	for _, name := range names {
		for _, node := range compose.Services[name].Ports {
			port := 0
			if node.Kind == yaml.ScalarNode {
				text := node.Value
				if strings.HasSuffix(text, "/udp") {
					continue
				}
				text = strings.TrimSuffix(text, "/tcp")
				published, _, ok := strings.Cut(text, ":")
				if strings.Count(text, ":") > 1 {
					index := strings.LastIndex(text, ":")
					host, value, err := net.SplitHostPort(text[:index])
					if err != nil || !runtimeLocalHost(host) {
						continue
					}
					published = value
				}
				if ok {
					port, _ = parseRuntimePort(published)
				}
			} else if node.Kind == yaml.MappingNode {
				var published struct {
					Port     string `yaml:"published"`
					Host     string `yaml:"host_ip"`
					Protocol string `yaml:"protocol"`
				}
				if node.Decode(&published) == nil && (published.Protocol == "" || published.Protocol == "tcp") && runtimeLocalHost(published.Host) {
					port, _ = parseRuntimePort(published.Port)
				}
			}
			if port > 0 && !slices.ContainsFunc(result, func(d LocalServiceDefinition) bool { return d.Port == port }) {
				result = append(result, LocalServiceDefinition{ID: fmt.Sprintf("dependency-%d", port), Name: fmt.Sprintf("%s · %d", name, port), ObserveOnly: true, Port: port})
			}
		}
	}
	return result, true
}

func runtimeLocalHost(host string) bool {
	return slices.Contains([]string{"", "localhost", "127.0.0.1", "::1", "0.0.0.0", "::"}, host)
}
