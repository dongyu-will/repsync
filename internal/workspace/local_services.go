package workspace

import (
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"time"
)

type LocalServiceDefinition struct {
	ID            string                     `json:"id"`
	Name          string                     `json:"name"`
	Command       []string                   `json:"command"`
	Port          int                        `json:"port"`
	AutoPort      bool                       `json:"autoPort"`
	FixedPort     bool                       `json:"fixedPort,omitempty"`
	ObserveOnly   bool                       `json:"observeOnly,omitempty"`
	URL           string                     `json:"url"`
	Cwd           string                     `json:"cwd"`
	Env           map[string]string          `json:"env"`
	EnvFrom       map[string]ServiceEndpoint `json:"envFrom,omitempty"`
	DependsOn     []string                   `json:"dependsOn,omitempty"`
	OneShot       bool                       `json:"oneShot,omitempty"`
	scriptVersion string
}

type LocalServiceState struct {
	Definition    LocalServiceDefinition `json:"definition"`
	RunID         string                 `json:"runId"`
	Status        string                 `json:"status"`
	PID           int                    `json:"pid"`
	PortState     string                 `json:"portState"`
	StartedAt     string                 `json:"startedAt"`
	EndedAt       string                 `json:"endedAt"`
	ExitCode      int                    `json:"exitCode"`
	Error         string                 `json:"error"`
	ConfigChanged bool                   `json:"configChanged"`
	Usage         *LocalServiceUsage     `json:"usage,omitempty"`
	UsageError    string                 `json:"usageError,omitempty"`
}

type LocalServicesOverview struct {
	Services []LocalServiceState `json:"services"`
	Source   string              `json:"source"`
	Version  string              `json:"version"`
	Error    string              `json:"error"`
	Warnings []string            `json:"warnings,omitempty"`
}

type localServiceConfig struct {
	services []LocalServiceDefinition
	source   string
	version  string
	warnings []string
}

var serviceScriptPort = regexp.MustCompile(`(?:--port(?:=|\s+)|\bPORT=)([0-9]{1,5})(?:\b|$)`)

func (s *Service) localServiceConfig(path string) (localServiceConfig, error) {
	root, err := os.OpenRoot(path)
	if err != nil {
		return localServiceConfig{}, err
	}
	defer root.Close()
	config := localServiceConfig{services: []LocalServiceDefinition{}, source: ".repsync.json"}
	data, err := readProjectConfig(root, config.source)
	var file struct {
		Services []LocalServiceDefinition `json:"services"`
	}
	if err == nil {
		if err := json.Unmarshal(data, &file); err != nil {
			return config, fmt.Errorf("read .repsync.json: %w", err)
		}
		config.services = file.Services
	} else if !errors.Is(err, os.ErrNotExist) {
		return config, err
	}
	if file.Services == nil {
		moon, input, found, err := discoverMoonServices(path, root)
		if err != nil {
			return config, err
		}
		if found {
			config.services, config.source, data = moon, "Moon · README.md", input
		} else {
			config.source = "package.json"
			config.services, data, config.warnings, err = discoverPackageServices(path, root)
			if err != nil {
				return config, err
			}
		}
	}
	if config.services == nil {
		config.services = []LocalServiceDefinition{}
	}
	if err := validateLocalServices(config.services); err != nil {
		return config, err
	}
	for i, definition := range config.services {
		if _, err := commandDirectory(path, definition.Cwd); err != nil {
			return config, err
		}
		prepared, input, err := automaticServicePort(path, definition)
		if err != nil {
			return config, err
		}
		config.services[i] = prepared
		data = append(data, input...)
	}
	if err := validateLocalServices(config.services); err != nil {
		return config, err
	}
	resolved, _ := json.Marshal(config.services)
	config.version = fmt.Sprintf("%x", sha256.Sum256(append(data, resolved...)))
	return config, nil
}

func validateLocalServices(definitions []LocalServiceDefinition) error {
	if len(definitions) > 8 {
		return errors.New("each project supports up to 8 local services")
	}
	ids := map[string]bool{}
	for i := range definitions {
		d := &definitions[i]
		if d.FixedPort && d.AutoPort {
			return errors.New("fixedPort and autoPort cannot both be enabled")
		}
		if d.ID == "" || len(d.ID) > 80 || ids[d.ID] || strings.TrimSpace(d.Name) == "" {
			return errors.New("local services require a unique id, name, and command arguments")
		}
		if d.ObserveOnly {
			if len(d.Command) > 0 || d.AutoPort || d.Cwd != "" || len(d.Env) > 0 || len(d.EnvFrom) > 0 || d.OneShot {
				return errors.New("observeOnly dependencies cannot declare commands, automatic ports, or command environment")
			}
		} else if len(d.Command) == 0 || strings.TrimSpace(d.Command[0]) == "" {
			return errors.New("local services require a unique id, name, and command arguments")
		}
		ids[d.ID] = true
		if d.OneShot && (d.Port != 0 || d.AutoPort || d.URL != "") {
			return errors.New("oneShot setup commands cannot declare listening ports or URLs")
		}
		for _, dependency := range d.DependsOn {
			if dependency == "" || dependency == d.ID {
				return errors.New("dependsOn requires another service id")
			}
		}
		if err := validateCommandOptions(d.Cwd, d.Env); err != nil {
			return err
		}
		for name, endpoint := range d.EnvFrom {
			if name == "PATH" || name == "PORT" {
				return fmt.Errorf("environment variable %s is provided by this service's path or port configuration and cannot reference another service", name)
			}
			if err := validateCommandOptions("", map[string]string{name: ""}); err != nil {
				return err
			}
			if _, exists := d.Env[name]; exists {
				return fmt.Errorf("environment variable %s cannot be set in both env and envFrom", name)
			}
			if endpoint.Service == "" || (endpoint.Field != "" && endpoint.Field != "url" && endpoint.Field != "port") {
				return errors.New("envFrom requires service; field must be url or port")
			}
		}
		for _, arg := range d.Command {
			if strings.ContainsRune(arg, 0) {
				return errors.New("service commands cannot contain null characters")
			}
		}
		if d.Port < 0 || d.Port > 65535 {
			return errors.New("service port must be 1–65535 or omitted")
		}
		if d.AutoPort {
			portInput := strings.Contains(strings.Join(d.Command, "\n"), "{port}")
			for _, value := range d.Env {
				portInput = portInput || strings.Contains(value, "{port}")
			}
			if !portInput {
				return errors.New("autoPort services require {port} in command or env to use the assigned port")
			}
			if d.URL != "" && !strings.Contains(d.URL, "{port}") {
				return errors.New("autoPort service URLs must contain {port}")
			}
		}
		if d.URL != "" {
			urlText := d.URL
			if strings.Contains(urlText, "{port}") {
				if !d.AutoPort && d.Port == 0 {
					return errors.New("URLs using {port} require port or autoPort")
				}
				port := d.Port
				if port == 0 {
					port = 1
				}
				urlText = strings.ReplaceAll(urlText, "{port}", strconv.Itoa(port))
			}
			u, err := url.Parse(urlText)
			if err != nil || u.User != nil || (u.Scheme != "http" && u.Scheme != "https") || !slices.Contains([]string{"127.0.0.1", "localhost", "::1"}, u.Hostname()) {
				return errors.New("service URLs must use a local HTTP(S) address (localhost, 127.0.0.1, or ::1)")
			}
			port := 80
			if u.Scheme == "https" {
				port = 443
			}
			if u.Port() != "" {
				port, err = strconv.Atoi(u.Port())
			}
			if err != nil || port < 1 || port > 65535 || (d.Port != 0 && d.Port != port) {
				return errors.New("service URL and port must specify the same valid port")
			}
			if !strings.Contains(d.URL, "{port}") {
				d.Port = port
			}
		}
		if d.ObserveOnly && d.Port == 0 {
			return errors.New("observeOnly dependencies require a local listening port")
		}
	}
	return nil
}

func serviceAddress(d LocalServiceDefinition) string {
	host := "127.0.0.1"
	if d.URL != "" {
		if u, err := url.Parse(strings.ReplaceAll(d.URL, "{port}", "1")); err == nil {
			host = u.Hostname()
		}
	}
	return net.JoinHostPort(host, strconv.Itoa(d.Port))
}

func servicePortState(d LocalServiceDefinition) string {
	if d.Port == 0 {
		return "none"
	}
	listener, err := reserveTCPPort(d.Port)
	if err == nil {
		listener.Close()
		return "available"
	}
	var op *net.OpError
	if errors.As(err, &op) && errors.Is(op.Err, os.ErrPermission) {
		return "unknown"
	}
	return "occupied"
}

func servicePortConnects(d LocalServiceDefinition) bool {
	connection, err := net.DialTimeout("tcp", serviceAddress(d), 150*time.Millisecond)
	if err != nil {
		return false
	}
	connection.Close()
	return true
}

func observedServiceState(d LocalServiceDefinition) LocalServiceState {
	state := LocalServiceState{Definition: d, Status: "unreachable", PortState: "unreachable", ExitCode: -1}
	if servicePortConnects(d) {
		state.Status, state.PortState = "reachable", "listening"
	}
	return state
}

func (s *Service) localServiceFile(path, id, extension string) string {
	return filepath.Join(filepath.Dir(s.path), "services", fmt.Sprintf("%x", sha256.Sum256([]byte(path))), fmt.Sprintf("%x", sha256.Sum256([]byte(id)))+extension)
}

// LocalServices retains stop controls even if the configuration has changed or become invalid.
func (s *Service) LocalServices(path string) (LocalServicesOverview, error) {
	if _, err := s.registeredProject(path); err != nil {
		return LocalServicesOverview{}, err
	}
	config, configErr := s.localServiceConfig(path)
	result := LocalServicesOverview{Services: []LocalServiceState{}, Source: config.source, Version: config.version, Warnings: config.warnings}
	if configErr != nil {
		result.Error = configErr.Error()
		config.services = nil
	}
	s.taskMu.Lock()
	owned := map[string]*activeLocalService{}
	for id, active := range s.activeLocalServices[path] {
		owned[id] = active
	}
	s.taskMu.Unlock()
	seen := map[string]bool{}
	for _, definition := range config.services {
		seen[definition.ID] = true
		if definition.ObserveOnly {
			active := owned[definition.ID]
			running := false
			if active != nil {
				active.mu.Lock()
				running = localServiceRunning(active.state.Status)
				active.mu.Unlock()
			}
			if !running {
				result.Services = append(result.Services, observedServiceState(definition))
				continue
			}
		}
		state := LocalServiceState{Definition: definition, Status: "stopped", ExitCode: -1}
		if active := owned[definition.ID]; active != nil {
			active.mu.Lock()
			state = active.state
			active.mu.Unlock()
			state.ConfigChanged = !sameServiceDefinition(active.configured, definition)
			if !localServiceRunning(state.Status) {
				state.Definition = definition
			}
		} else {
			var previous LocalServiceState
			err := readPipelineJSON(s.localServiceFile(path, definition.ID, ".json"), &previous)
			if err == nil {
				state = previous
				state.Definition = definition
				if localServiceRunning(state.Status) {
					state.Status, state.Error = "interrupted", "previous service run did not stop cleanly; check the port before starting manually"
				}
				state.PID = 0
			} else if !errors.Is(err, os.ErrNotExist) {
				state.Error = "read previous service record: " + err.Error()
			}
		}
		if !localServiceRunning(state.Status) {
			state.PortState = servicePortState(definition)
		}
		result.Services = append(result.Services, state)
	}
	for id, active := range owned {
		if seen[id] {
			continue
		}
		active.mu.Lock()
		state := active.state
		active.mu.Unlock()
		if localServiceRunning(state.Status) {
			state.ConfigChanged = true
			result.Services = append(result.Services, state)
		}
	}
	populateServiceUsage(result.Services)
	return result, nil
}

func localServiceRunning(status string) bool {
	return status == "starting" || status == "running" || status == "stopping"
}
