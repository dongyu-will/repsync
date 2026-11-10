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
	URL           string                     `json:"url"`
	Cwd           string                     `json:"cwd"`
	Env           map[string]string          `json:"env"`
	EnvFrom       map[string]ServiceEndpoint `json:"envFrom,omitempty"`
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
}

type LocalServicesOverview struct {
	Services []LocalServiceState `json:"services"`
	Source   string              `json:"source"`
	Version  string              `json:"version"`
	Error    string              `json:"error"`
}

type localServiceConfig struct {
	services []LocalServiceDefinition
	source   string
	version  string
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
			return config, fmt.Errorf("读取 .repsync.json: %w", err)
		}
		config.services = file.Services
	} else if !errors.Is(err, os.ErrNotExist) {
		return config, err
	}
	if file.Services == nil {
		config.source = "package.json"
		data, err = readProjectConfig(root, config.source)
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
		if err := json.Unmarshal(data, &pkg); err != nil {
			return config, fmt.Errorf("读取 package.json: %w", err)
		}
		for _, name := range []string{"dev", "start"} {
			script, ok := pkg.Scripts[name]
			if !ok {
				continue
			}
			manager, err := projectPackageManager(root, pkg.PackageManager)
			if err != nil {
				return config, err
			}
			definition := LocalServiceDefinition{ID: name, Name: name, Command: []string{manager, "run", name}}
			if match := serviceScriptPort.FindStringSubmatch(script); len(match) > 1 {
				definition.Port, _ = strconv.Atoi(match[1])
			}
			config.services = []LocalServiceDefinition{definition}
			break
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
		return errors.New("每项目最多配置 8 个本地服务")
	}
	ids := map[string]bool{}
	for i := range definitions {
		d := &definitions[i]
		if d.FixedPort && d.AutoPort {
			return errors.New("fixedPort 与 autoPort 不能同时启用")
		}
		if d.ID == "" || len(d.ID) > 80 || ids[d.ID] || strings.TrimSpace(d.Name) == "" || len(d.Command) == 0 || strings.TrimSpace(d.Command[0]) == "" {
			return errors.New("本地服务需要唯一 id、名称和 command 命令参数")
		}
		ids[d.ID] = true
		if err := validateCommandOptions(d.Cwd, d.Env); err != nil {
			return err
		}
		for name, endpoint := range d.EnvFrom {
			if name == "PATH" || name == "PORT" {
				return fmt.Errorf("环境变量 %s 由当前服务的路径或端口配置提供，不能引用其他服务", name)
			}
			if err := validateCommandOptions("", map[string]string{name: ""}); err != nil {
				return err
			}
			if _, exists := d.Env[name]; exists {
				return fmt.Errorf("环境变量 %s 不能同时配置 env 和 envFrom", name)
			}
			if endpoint.Service == "" || (endpoint.Field != "" && endpoint.Field != "url" && endpoint.Field != "port") {
				return errors.New("envFrom 需要 service，field 只支持 url 或 port")
			}
		}
		for _, arg := range d.Command {
			if strings.ContainsRune(arg, 0) {
				return errors.New("服务命令不能包含空字符")
			}
		}
		if d.Port < 0 || d.Port > 65535 {
			return errors.New("服务端口应为 1–65535，或留空")
		}
		if d.AutoPort {
			portInput := strings.Contains(strings.Join(d.Command, "\n"), "{port}")
			for _, value := range d.Env {
				portInput = portInput || strings.Contains(value, "{port}")
			}
			if !portInput {
				return errors.New("autoPort 服务的 command 或 env 必须包含 {port}，确保命令使用分配的端口")
			}
			if d.URL != "" && !strings.Contains(d.URL, "{port}") {
				return errors.New("autoPort 服务的 URL 需要包含 {port}")
			}
		}
		if d.URL != "" {
			urlText := d.URL
			if strings.Contains(urlText, "{port}") {
				if !d.AutoPort && d.Port == 0 {
					return errors.New("URL 使用 {port} 时需设置 port 或 autoPort")
				}
				port := d.Port
				if port == 0 {
					port = 1
				}
				urlText = strings.ReplaceAll(urlText, "{port}", strconv.Itoa(port))
			}
			u, err := url.Parse(urlText)
			if err != nil || u.User != nil || (u.Scheme != "http" && u.Scheme != "https") || !slices.Contains([]string{"127.0.0.1", "localhost", "::1"}, u.Hostname()) {
				return errors.New("服务 URL 需要使用本机 http/https 地址（localhost、127.0.0.1 或 ::1）")
			}
			port := 80
			if u.Scheme == "https" {
				port = 443
			}
			if u.Port() != "" {
				port, err = strconv.Atoi(u.Port())
			}
			if err != nil || port < 1 || port > 65535 || (d.Port != 0 && d.Port != port) {
				return errors.New("服务 URL 与 port 的端口必须一致且有效")
			}
			if !strings.Contains(d.URL, "{port}") {
				d.Port = port
			}
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

func (s *Service) localServiceFile(path, id, extension string) string {
	return filepath.Join(filepath.Dir(s.path), "services", fmt.Sprintf("%x", sha256.Sum256([]byte(path))), fmt.Sprintf("%x", sha256.Sum256([]byte(id)))+extension)
}

// LocalServices retains stop controls even if the configuration has changed or become invalid.
func (s *Service) LocalServices(path string) (LocalServicesOverview, error) {
	if _, err := s.registeredProject(path); err != nil {
		return LocalServicesOverview{}, err
	}
	config, configErr := s.localServiceConfig(path)
	result := LocalServicesOverview{Services: []LocalServiceState{}, Source: config.source, Version: config.version}
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
					state.Status, state.Error = "interrupted", "上次服务运行未正常结束，请检查端口后手动启动"
				}
				state.PID = 0
			} else if !errors.Is(err, os.ErrNotExist) {
				state.Error = "读取上次服务记录: " + err.Error()
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
	return result, nil
}

func localServiceRunning(status string) bool {
	return status == "starting" || status == "running" || status == "stopping"
}
