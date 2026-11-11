package workspace

import (
	"encoding/json"
	"fmt"
	"maps"
	"net"
	"net/url"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
)

// automaticServicePort adapts known port inputs in memory. It never rewrites a project file.
func automaticServicePort(path string, definition LocalServiceDefinition) (LocalServiceDefinition, []byte, error) {
	if definition.AutoPort || definition.FixedPort {
		return definition, nil, nil
	}
	d := definition
	d.Command, d.Env = slices.Clone(d.Command), maps.Clone(d.Env)
	if len(d.Command) == 3 && slices.Contains([]string{"sh", "bash", "zsh"}, filepath.Base(d.Command[0])) && d.Command[1] == "-c" {
		text := d.Command[2]
		if strings.ContainsAny(text, ";&|<>$()`\n\r\"'\\*?[]{}~#") {
			return definition, nil, nil
		}
		d.Command = strings.Fields(text)
		if len(d.Command) > 1 && d.Command[0] == "exec" {
			d.Command = d.Command[1:]
		}
		if len(d.Command) == 0 || slices.Contains([]string{"command", "builtin", "eval", "set"}, d.Command[0]) {
			return definition, nil, nil
		}
	}
	preferred, adjustable := d.Port, false
	accept := func(value string) bool {
		port, err := strconv.Atoi(value)
		if err != nil || port < 1 || port > 65535 || (preferred != 0 && preferred != port) {
			return false
		}
		preferred = port
		return true
	}
	for i := 1; i < len(d.Command); i++ {
		if d.Command[i] == "--port" && i+1 < len(d.Command) {
			if !accept(d.Command[i+1]) {
				return definition, nil, nil
			}
			d.Command[i+1], adjustable = "{port}", true
			i++
		} else if value, ok := strings.CutPrefix(d.Command[i], "--port="); ok {
			if !accept(value) {
				return definition, nil, nil
			}
			d.Command[i], adjustable = "--port={port}", true
		}
	}
	if value, ok := d.Env["PORT"]; ok {
		if !accept(value) {
			return definition, nil, nil
		}
		d.Env["PORT"], adjustable = "{port}", true
	}
	var input []byte
	if !adjustable {
		var err error
		d, input, adjustable, err = viteServicePort(path, d)
		if err != nil {
			return definition, input, err
		}
		if !adjustable {
			return definition, input, nil
		}
		preferred = d.Port
	}
	d.Port, d.AutoPort = preferred, true
	if len(input) > 0 {
		d.scriptVersion = digest(string(input))
	}
	if d.URL != "" && !strings.Contains(d.URL, "{port}") {
		u, err := url.Parse(d.URL)
		if err != nil {
			return definition, input, err
		}
		d.URL = strings.Replace(d.URL, u.Host, net.JoinHostPort(u.Hostname(), "{port}"), 1)
	}
	return d, input, nil
}

// npm/pnpm/yarn/bun scripts hide Vite's actual argv. Vite accepts a final --port override.
// Only a single plain Vite command is adapted; compound scripts keep their original semantics.
func viteServicePort(path string, d LocalServiceDefinition) (LocalServiceDefinition, []byte, bool, error) {
	args := d.Command
	if len(args) < 3 || !slices.Contains([]string{"npm", "pnpm", "yarn", "bun"}, filepath.Base(args[0])) {
		return d, nil, false, nil
	}
	directory := d.Cwd
	index := 1
	if len(args) >= 5 && args[index] == "--prefix" {
		directory = filepath.Join(directory, args[index+1])
		index += 2
	}
	if len(args) != index+2 || args[index] != "run" {
		return d, nil, false, nil
	}
	working, err := commandDirectory(path, directory)
	if err != nil {
		return d, nil, false, nil
	}
	root, err := os.OpenRoot(working)
	if err != nil {
		return d, nil, false, nil
	}
	defer root.Close()
	data, err := readProjectConfig(root, "package.json")
	if os.IsNotExist(err) {
		return d, nil, false, nil
	}
	if err != nil {
		return d, nil, false, fmt.Errorf("read service script: %w", err)
	}
	var pkg struct {
		Scripts map[string]string `json:"scripts"`
	}
	if err := json.Unmarshal(data, &pkg); err != nil {
		return d, data, false, fmt.Errorf("read service package.json: %w", err)
	}
	script := strings.TrimSpace(pkg.Scripts[args[index+1]])
	words := strings.Fields(script)
	if len(words) == 0 || words[0] != "vite" || strings.ContainsAny(script, ";&|<>$()`\n\r\"'\\") {
		return d, data, false, nil
	}
	// Vite subcommands are finite tasks or previews; only its development server is managed here.
	if len(words) > 1 && slices.Contains([]string{"build", "preview", "optimize"}, words[1]) {
		return d, data, false, nil
	}
	if d.Port == 0 {
		d.Port = 5173
		if match := serviceScriptPort.FindStringSubmatch(script); len(match) > 1 {
			d.Port, _ = strconv.Atoi(match[1])
		}
	}
	d.Command = slices.Clone(args)
	if filepath.Base(args[0]) == "npm" {
		d.Command = append(d.Command, "--")
	}
	d.Command = append(d.Command, "--port", "{port}", "--strictPort")
	return d, data, true, nil
}
