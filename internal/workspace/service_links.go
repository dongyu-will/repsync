package workspace

import (
	"context"
	"errors"
	"fmt"
	"maps"
	"path/filepath"
	"slices"
	"strconv"
	"syscall"
	"time"
)

type ServiceEndpoint struct {
	Project string `json:"project,omitempty"`
	Service string `json:"service"`
	Field   string `json:"field,omitempty"`
}

type servicePlan struct {
	path         string
	configured   LocalServiceDefinition
	config       localServiceConfig
	dependencies map[string]int
}

func endpointProject(path string, endpoint ServiceEndpoint) (string, error) {
	if endpoint.Project == "" {
		return path, nil
	}
	target := endpoint.Project
	if !filepath.IsAbs(target) {
		target = filepath.Join(path, target)
	}
	canonical, err := filepath.EvalSymlinks(target)
	if err != nil {
		return "", fmt.Errorf("依赖项目不可访问: %w", err)
	}
	return canonical, nil
}

func (s *Service) servicePlans(paths []string, selection map[string]string) ([]servicePlan, error) {
	configs := map[string]localServiceConfig{}
	load := func(path string) (localServiceConfig, error) {
		if config, ok := configs[path]; ok {
			return config, nil
		}
		if _, err := s.registeredProject(path); err != nil {
			return localServiceConfig{}, fmt.Errorf("%s: %w", path, err)
		}
		config, err := s.localServiceConfig(path)
		if err == nil {
			configs[path] = config
		}
		return config, err
	}
	plans := []servicePlan{}
	indexes := map[string]int{}
	visiting := map[string]bool{}
	var visit func(string, string) (int, error)
	visit = func(path, id string) (int, error) {
		key := path + "\x00" + id
		if index, ok := indexes[key]; ok {
			return index, nil
		}
		if visiting[key] {
			return 0, fmt.Errorf("服务依赖存在循环: %s / %s", path, id)
		}
		if len(visiting)+len(plans) >= 256 {
			return 0, errors.New("依赖服务过多")
		}
		visiting[key] = true
		config, err := load(path)
		if err != nil {
			return 0, fmt.Errorf("%s: %w", path, err)
		}
		index := slices.IndexFunc(config.services, func(d LocalServiceDefinition) bool { return d.ID == id })
		if index < 0 {
			return 0, fmt.Errorf("未找到依赖服务: %s / %s", path, id)
		}
		plan := servicePlan{path: path, configured: config.services[index], config: config, dependencies: map[string]int{}}
		names := slices.Sorted(maps.Keys(plan.configured.EnvFrom))
		for _, name := range names {
			endpoint := plan.configured.EnvFrom[name]
			target, err := endpointProject(path, endpoint)
			if err != nil {
				return 0, err
			}
			dependency, err := visit(target, endpoint.Service)
			if err != nil {
				return 0, err
			}
			plan.dependencies[name] = dependency
		}
		delete(visiting, key)
		indexes[key] = len(plans)
		plans = append(plans, plan)
		return len(plans) - 1, nil
	}
	for _, path := range paths {
		config, err := load(path)
		if err != nil {
			return nil, err
		}
		if len(config.services) == 0 {
			return nil, fmt.Errorf("%s 没有可启动的服务，请先由 AI 接入启动命令", path)
		}
		if id := selection[path]; id != "" {
			if _, err := visit(path, id); err != nil {
				return nil, err
			}
		} else {
			for _, d := range config.services {
				if _, err := visit(path, d.ID); err != nil {
					return nil, err
				}
			}
		}
	}
	return plans, nil
}

func resolveServiceLinks(d LocalServiceDefinition, plan servicePlan, results []ServiceStartResult) (LocalServiceDefinition, error) {
	if len(plan.dependencies) == 0 {
		return d, nil
	}
	d.Env = maps.Clone(d.Env)
	if d.Env == nil {
		d.Env = map[string]string{}
	}
	for name, index := range plan.dependencies {
		source := results[index]
		endpoint := d.EnvFrom[name]
		if source.Status == "blocked" {
			return d, fmt.Errorf("依赖 %s 启动预检未通过", endpoint.Service)
		}
		definition := source.State.Definition
		value := definition.URL
		if endpoint.Field == "port" {
			value = strconv.Itoa(definition.Port)
		}
		if value == "" || definition.Port == 0 {
			return d, fmt.Errorf("依赖 %s 缺少可用的 %s 或监听端口", endpoint.Service, endpoint.Field)
		}
		d.Env[name] = value
	}
	return d, nil
}

func (s *Service) waitServiceEndpoint(ctx context.Context, item ServiceStartResult) error {
	s.taskMu.Lock()
	active := s.activeLocalServices[item.Path][item.ServiceID]
	s.taskMu.Unlock()
	if active == nil {
		return fmt.Errorf("依赖服务 %s 已停止", item.ServiceID)
	}
	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()
	for {
		active.mu.Lock()
		state := active.state
		active.mu.Unlock()
		if state.RunID != item.State.RunID || !localServiceRunning(state.Status) || state.Status == "stopping" {
			return fmt.Errorf("依赖服务 %s 已结束或正在停止", item.ServiceID)
		}
		if servicePortConnects(state.Definition) {
			owners, err := localPortOwners(state.Definition.Port)
			if err != nil {
				return fmt.Errorf("核对依赖服务 %s 的监听归属: %w", item.ServiceID, err)
			}
			for _, owner := range owners {
				group, err := syscall.Getpgid(owner.PID)
				if err == nil && group == state.PID {
					select {
					case <-active.done:
						return fmt.Errorf("依赖服务 %s 已结束", item.ServiceID)
					default:
					}
					return nil
				}
			}
		}
		select {
		case <-ctx.Done():
			return fmt.Errorf("等待依赖服务 %s 端口就绪失败: %w", item.ServiceID, ctx.Err())
		case <-active.done:
			return fmt.Errorf("依赖服务 %s 已结束", item.ServiceID)
		case <-ticker.C:
		}
	}
}
