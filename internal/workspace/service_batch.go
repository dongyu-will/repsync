package workspace

import (
	"context"
	"errors"
	"fmt"
	"maps"
	"net"
	"slices"
	"strconv"
	"strings"
	"syscall"
	"time"
)

// ServiceStartResult is a per-service receipt. Only status=already_running reuses an existing run.
type ServiceStartResult struct {
	Path        string            `json:"path"`
	ServiceID   string            `json:"serviceId"`
	Status      string            `json:"status"`
	State       LocalServiceState `json:"state"`
	Error       string            `json:"error"`
	PortChecked bool              `json:"portChecked"`
}

type ServiceBatch struct {
	DryRun   bool                 `json:"dryRun"`
	Services []ServiceStartResult `json:"services"`
}

func sameServiceDefinition(a, b LocalServiceDefinition) bool {
	return slices.Equal(a.Command, b.Command) && maps.Equal(a.Env, b.Env) && maps.Equal(a.EnvFrom, b.EnvFrom) && a.Cwd == b.Cwd && a.Port == b.Port && a.URL == b.URL && a.AutoPort == b.AutoPort && a.FixedPort == b.FixedPort && a.scriptVersion == b.scriptVersion
}

func (s *Service) activeServicePortsLocked() (int, map[int]string) {
	count, ports := 0, map[int]string{}
	for path, services := range s.activeLocalServices {
		for _, active := range services {
			active.mu.Lock()
			state := active.state
			active.mu.Unlock()
			if localServiceRunning(state.Status) {
				count++
				if state.Definition.Port != 0 {
					ports[state.Definition.Port] = path + " / " + state.Definition.Name
				}
			}
		}
	}
	return count, ports
}

func resolveServicePort(d LocalServiceDefinition, port int) LocalServiceDefinition {
	resolved := d
	resolved.Port = port
	resolved.Command = slices.Clone(d.Command)
	for i, value := range resolved.Command {
		resolved.Command[i] = strings.ReplaceAll(value, "{port}", strconv.Itoa(port))
	}
	resolved.Env = maps.Clone(d.Env)
	for key, value := range resolved.Env {
		resolved.Env[key] = strings.ReplaceAll(value, "{port}", strconv.Itoa(port))
	}
	resolved.URL = strings.ReplaceAll(d.URL, "{port}", strconv.Itoa(port))
	return resolved
}

// Reserve IPv4 and IPv6 separately: on macOS an IPv6 wildcard may not cover IPv4.
type portReservation struct {
	net.Listener
	ipv6 net.Listener
}

func (r *portReservation) Close() error {
	err := r.Listener.Close()
	if r.ipv6 != nil {
		_ = r.ipv6.Close()
	}
	return err
}
func reserveTCPPort(port int) (net.Listener, error) {
	// lsof sees listeners on every interface; loopback reservations preserve quick restarts
	// without treating TCP TIME_WAIT as an active service.
	if port == 0 {
		candidate, err := net.Listen("tcp4", "127.0.0.1:0")
		if err != nil {
			return nil, err
		}
		port = candidate.Addr().(*net.TCPAddr).Port
		candidate.Close()
	}
	owners, err := localPortOwners(port)
	if err != nil {
		return nil, err
	}
	if len(owners) > 0 {
		return nil, fmt.Errorf("端口 %d 存在监听进程 %s（PID %d）", port, owners[0].Name, owners[0].PID)
	}
	ipv4, err := net.Listen("tcp4", net.JoinHostPort("127.0.0.1", strconv.Itoa(port)))
	if err != nil {
		return nil, err
	}
	ipv6, err := net.Listen("tcp6", net.JoinHostPort("::1", strconv.Itoa(port)))
	if err != nil && !errors.Is(err, syscall.EAFNOSUPPORT) && !errors.Is(err, syscall.EPROTONOSUPPORT) && !errors.Is(err, syscall.EADDRNOTAVAIL) {
		ipv4.Close()
		return nil, err
	}
	return &portReservation{Listener: ipv4, ipv6: ipv6}, nil
}

// Keep sockets reserved during batch preflight. Children must bind after we release their socket.
func reserveServicePort(d LocalServiceDefinition, used map[int]string) (LocalServiceDefinition, net.Listener, error) {
	if d.Port == 0 && !d.AutoPort {
		return d, nil, nil
	}
	attempts := 1
	if d.AutoPort {
		attempts = 100
	}
	for offset := 0; offset < attempts; offset++ {
		port := d.Port
		if port > 0 {
			port += offset
			if port > 65535 {
				break
			}
		}
		if owner, ok := used[port]; ok {
			if !d.AutoPort {
				return d, nil, fmt.Errorf("端口 %d 已由 %s 使用", port, owner)
			}
			continue
		}
		listener, err := reserveTCPPort(port)
		if err != nil {
			if !d.AutoPort {
				return d, nil, fmt.Errorf("端口 %d 不可用: %w；未停止其他进程", port, err)
			}
			continue
		}
		assigned := listener.Addr().(*net.TCPAddr).Port
		if _, ok := used[assigned]; ok {
			listener.Close()
			continue
		}
		return resolveServicePort(d, assigned), listener, nil
	}
	return d, nil, errors.New("找不到可用端口，请调整首选端口后重试")
}

// StartProjects validates all selected projects before starting any new service.
// Processes run concurrently after cmd.Start; a successful receipt does not assert application readiness.
func (s *Service) StartProjects(paths []string, dryRun bool) (ServiceBatch, error) {
	return s.startServices(paths, dryRun, nil, nil)
}

func (s *Service) startServices(paths []string, dryRun bool, selection map[string]string, versions map[string]string) (ServiceBatch, error) {
	s.serviceStartMu.Lock()
	defer s.serviceStartMu.Unlock()
	result := ServiceBatch{DryRun: dryRun, Services: []ServiceStartResult{}}
	if len(paths) == 0 || len(paths) > 32 {
		return result, errors.New("一次需要指定 1–32 个已登记项目")
	}
	plans, err := s.servicePlans(paths, selection)
	if err != nil {
		return result, err
	}
	for _, plan := range plans {
		if expected := versions[plan.path]; expected != "" && expected != plan.config.version {
			return result, errors.New("服务配置已改变，请刷新并核对命令后启动")
		}
	}
	s.taskMu.Lock()
	locked := true
	defer func() {
		if locked {
			s.taskMu.Unlock()
		}
	}()
	if s.closing {
		return result, errors.New("应用正在退出")
	}
	count, ports := s.activeServicePortsLocked()
	reservations := make([]net.Listener, len(plans))
	defer func() {
		for _, listener := range reservations {
			if listener != nil {
				listener.Close()
			}
		}
	}()
	blocked := false
	result.Services = make([]ServiceStartResult, len(plans))
	// Give fixed services priority; automatic services move around those reservations.
	order := []int{}
	for i, plan := range plans {
		if !plan.configured.AutoPort {
			order = append(order, i)
		}
	}
	for i, plan := range plans {
		if plan.configured.AutoPort {
			order = append(order, i)
		}
	}
	for _, i := range order {
		plan := plans[i]
		entry := ServiceStartResult{Path: plan.path, ServiceID: plan.configured.ID, Status: "planned", PortChecked: plan.configured.Port != 0 || plan.configured.AutoPort}
		if active := s.activeLocalServices[plan.path][plan.configured.ID]; active != nil {
			active.mu.Lock()
			state := active.state
			configured := active.configured
			active.mu.Unlock()
			if localServiceRunning(state.Status) {
				entry.State = state
				if sameServiceDefinition(configured, plan.configured) && state.Status != "stopping" {
					entry.Status = "already_running"
				} else {
					entry.Status, entry.Error = "blocked", "运行中的配置已变化或正在停止，请先核对并停止原实例"
					blocked = true
				}
				result.Services[i] = entry
				continue
			}
		}
		resolved, listener, err := reserveServicePort(plan.configured, ports)
		if err == nil {
			reservations[i] = listener
			if resolved.Port != 0 {
				ports[resolved.Port] = plan.path + " / " + resolved.Name
			}
			_, err = projectCommand(context.Background(), plan.path, resolved.Cwd, resolved.Env, resolved.Command, false)
		}
		count++
		if err == nil && count > 16 {
			err = errors.New("最多同时运行 16 个本地服务")
		}
		entry.State = LocalServiceState{Definition: resolved, Status: "stopped", ExitCode: -1}
		if err != nil {
			entry.Status, entry.Error = "blocked", err.Error()
			blocked = true
		}
		result.Services[i] = entry
	}
	for i, plan := range plans {
		if result.Services[i].Status == "blocked" {
			continue
		}
		resolved := result.Services[i].State.Definition
		if result.Services[i].Status == "already_running" {
			resolved = resolveServicePort(plan.configured, resolved.Port)
		}
		resolved, err = resolveServiceLinks(resolved, plan, result.Services)
		if err == nil && result.Services[i].Status == "already_running" && !maps.Equal(resolved.Env, result.Services[i].State.Definition.Env) {
			err = errors.New("依赖服务的地址已变化，请重启此服务以更新环境变量")
		}
		if err != nil {
			result.Services[i].Status, result.Services[i].Error = "blocked", err.Error()
			blocked = true
		} else {
			result.Services[i].State.Definition = resolved
		}
	}
	if blocked {
		return result, errors.New("启动预检未通过，没有启动任何新服务")
	}
	// Detect configuration changes made during preflight before the first spawn.
	for _, plan := range plans {
		if _, err := s.registeredProject(plan.path); err != nil {
			return result, err
		}
		latest, err := s.localServiceConfig(plan.path)
		if err != nil || latest.version != plan.config.version {
			return result, fmt.Errorf("%s 配置已变化，请重新预检", plan.path)
		}
	}
	if dryRun {
		return result, nil
	}
	s.taskMu.Unlock()
	locked = false
	waitCtx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	depths := make([]int, len(plans))
	launchOrder := make([]int, len(plans))
	for i, plan := range plans {
		launchOrder[i] = i
		for _, dependency := range plan.dependencies {
			if depths[dependency]+1 > depths[i] {
				depths[i] = depths[dependency] + 1
			}
		}
	}
	slices.SortStableFunc(launchOrder, func(a, b int) int { return depths[a] - depths[b] })
	for _, i := range launchOrder {
		plan := plans[i]
		if result.Services[i].Status == "already_running" {
			continue
		}
		var state LocalServiceState
		var startErr error
		for _, dependency := range plan.dependencies {
			if startErr = s.waitServiceEndpoint(waitCtx, result.Services[dependency]); startErr != nil {
				break
			}
		}
		s.taskMu.Lock()
		if startErr == nil && s.closing {
			startErr = errors.New("应用正在退出")
		}
		if startErr == nil {
			_, startErr = s.registeredProject(plan.path)
		}
		if startErr == nil {
			latest, readErr := s.localServiceConfig(plan.path)
			if readErr != nil {
				startErr = readErr
			} else if latest.version != plan.config.version {
				startErr = errors.New("服务配置已变化，请重新预检")
			}
		}
		if reservations[i] != nil && startErr == nil {
			reservations[i].Close()
			reservations[i] = nil
		}
		if startErr == nil {
			state, startErr = s.startLocalServiceLocked(plan.path, plan.configured, result.Services[i].State.Definition)
		}
		s.taskMu.Unlock()
		if state.RunID != "" {
			result.Services[i].State = state
		}
		if startErr != nil {
			result.Services[i].Status, result.Services[i].Error = "failed", startErr.Error()
			for j := range plans {
				if result.Services[j].Status == "planned" {
					result.Services[j].Status = "not_started"
				}
			}
			return result, errors.New("部分服务启动失败，请按逐项结果处理；已启动服务保留")
		}
		result.Services[i].Status = "started"
	}
	return result, nil
}

// StopProjects captures and cancels only instances owned at the time of this request.
func (s *Service) StopProjects(paths []string) (ServiceBatch, error) {
	result := ServiceBatch{Services: []ServiceStartResult{}}
	if len(paths) == 0 || len(paths) > 32 {
		return result, errors.New("一次需要指定 1–32 个已登记项目")
	}
	s.taskMu.Lock()
	for _, path := range paths {
		if _, err := s.registeredProject(path); err != nil {
			s.taskMu.Unlock()
			return result, err
		}
	}
	done := []chan struct{}{}
	stopped := []*activeLocalService{}
	seen := map[string]bool{}
	for _, path := range paths {
		if seen[path] {
			continue
		}
		seen[path] = true
		for id, active := range s.activeLocalServices[path] {
			active.mu.Lock()
			if localServiceRunning(active.state.Status) {
				active.state.Status = "stopping"
				result.Services = append(result.Services, ServiceStartResult{Path: path, ServiceID: id, State: active.state, Status: "stopped", PortChecked: active.state.Definition.Port != 0})
				active.cancel()
				done = append(done, active.done)
				stopped = append(stopped, active)
			}
			active.mu.Unlock()
		}
	}
	s.taskMu.Unlock()
	for _, finished := range done {
		<-finished
	}
	// Read each captured instance's final state, rather than a possible replacement run.
	for i, active := range stopped {
		active.mu.Lock()
		result.Services[i].State = active.state
		active.mu.Unlock()
	}
	return result, nil
}

type ServiceRunRef struct {
	Path      string `json:"path"`
	ServiceID string `json:"serviceId"`
	RunID     string `json:"runId"`
}

// ServiceReadiness reports TCP readiness for exact run receipts; it never follows replacements.
func (s *Service) ServiceReadiness(runs []ServiceRunRef) (ServiceBatch, error) {
	result := ServiceBatch{Services: []ServiceStartResult{}}
	if len(runs) == 0 || len(runs) > 256 {
		return result, errors.New("需要指定 1–256 个服务运行编号")
	}
	failed := false
	for _, run := range runs {
		item := ServiceStartResult{Path: run.Path, ServiceID: run.ServiceID, Status: "pending"}
		output, err := s.LocalServiceLog(run.Path, run.ServiceID, run.RunID)
		item.State = output.State
		item.PortChecked = output.State.Definition.Port != 0
		switch {
		case err != nil:
			item.Status, item.Error, failed = "failed", err.Error(), true
		case !localServiceRunning(output.State.Status) || output.State.Status == "stopping":
			item.Status, item.Error, failed = "failed", "服务已结束或正在停止", true
		case !item.PortChecked:
			item.Status = "running_unchecked"
		case servicePortConnects(output.State.Definition):
			item.Status = "ready"
			item.State.Status, item.State.PortState = "running", "listening"
		}
		result.Services = append(result.Services, item)
	}
	if failed {
		return result, errors.New("部分服务未就绪，请查看逐项结果与日志")
	}
	return result, nil
}
