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
	waitFor      []int
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
		return "", fmt.Errorf("dependency project inaccessible: %w", err)
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
			return 0, fmt.Errorf("circular service dependency: %s / %s", path, id)
		}
		if len(visiting)+len(plans) >= 256 {
			return 0, errors.New("too many service dependencies")
		}
		visiting[key] = true
		config, err := load(path)
		if err != nil {
			return 0, fmt.Errorf("%s: %w", path, err)
		}
		index := slices.IndexFunc(config.services, func(d LocalServiceDefinition) bool { return d.ID == id })
		if index < 0 {
			return 0, fmt.Errorf("dependency service not found: %s / %s", path, id)
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
			plan.waitFor = append(plan.waitFor, dependency)
		}
		for _, id := range plan.configured.DependsOn {
			dependency, err := visit(path, id)
			if err != nil {
				return 0, err
			}
			if source := plans[dependency].configured; !source.OneShot && source.Port == 0 && !source.AutoPort {
				return 0, fmt.Errorf("dependency %s needs a listening port or oneShot completion", id)
			}
			if !slices.Contains(plan.waitFor, dependency) {
				plan.waitFor = append(plan.waitFor, dependency)
			}
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
			return nil, fmt.Errorf("%s has no startable services; ask AI to configure start commands first", path)
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
			return d, fmt.Errorf("dependency %s failed startup preflight", endpoint.Service)
		}
		definition := source.State.Definition
		value := definition.URL
		if endpoint.Field == "port" {
			value = strconv.Itoa(definition.Port)
		}
		if value == "" || definition.Port == 0 {
			return d, fmt.Errorf("dependency %s has no available %s or listening port", endpoint.Service, endpoint.Field)
		}
		d.Env[name] = value
	}
	return d, nil
}

func (s *Service) waitServiceEndpoint(ctx context.Context, item ServiceStartResult) error {
	if item.State.Definition.OneShot {
		return s.waitServiceSetup(ctx, item)
	}
	if item.State.Definition.ObserveOnly {
		if servicePortConnects(item.State.Definition) {
			return nil
		}
		return fmt.Errorf("external dependency %s is unreachable", item.ServiceID)
	}
	s.taskMu.Lock()
	active := s.activeLocalServices[item.Path][item.ServiceID]
	s.taskMu.Unlock()
	if active == nil {
		return fmt.Errorf("dependency service %s stopped", item.ServiceID)
	}
	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()
	for {
		active.mu.Lock()
		state := active.state
		active.mu.Unlock()
		if state.RunID != item.State.RunID || !localServiceRunning(state.Status) || state.Status == "stopping" {
			return fmt.Errorf("dependency service %s exited or is stopping", item.ServiceID)
		}
		if servicePortConnects(state.Definition) {
			owners, err := localPortOwners(state.Definition.Port)
			if err != nil {
				return fmt.Errorf("verify listener ownership for dependency service %s: %w", item.ServiceID, err)
			}
			for _, owner := range owners {
				group, err := syscall.Getpgid(owner.PID)
				if err == nil && group == state.PID {
					select {
					case <-active.done:
						return fmt.Errorf("dependency service %s exited", item.ServiceID)
					default:
					}
					return nil
				}
			}
		}
		select {
		case <-ctx.Done():
			return fmt.Errorf("waiting for dependency service %s port failed: %w", item.ServiceID, ctx.Err())
		case <-active.done:
			return fmt.Errorf("dependency service %s exited", item.ServiceID)
		case <-ticker.C:
		}
	}
}

func (s *Service) waitServiceSetup(ctx context.Context, item ServiceStartResult) error {
	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()
	for {
		output, err := s.LocalServiceLog(item.Path, item.ServiceID, item.State.RunID)
		if err != nil {
			return err
		}
		if output.State.Status == "exited" && output.State.ExitCode == 0 {
			return nil
		}
		if !localServiceRunning(output.State.Status) {
			return fmt.Errorf("setup %s failed; inspect its log", item.ServiceID)
		}
		select {
		case <-ctx.Done():
			return fmt.Errorf("waiting for setup %s: %w", item.ServiceID, ctx.Err())
		case <-ticker.C:
		}
	}
}
