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
	return slices.Equal(a.Command, b.Command) && maps.Equal(a.Env, b.Env) && maps.Equal(a.EnvFrom, b.EnvFrom) && a.Cwd == b.Cwd && a.Port == b.Port && a.URL == b.URL && a.AutoPort == b.AutoPort && a.FixedPort == b.FixedPort && a.ObserveOnly == b.ObserveOnly && a.scriptVersion == b.scriptVersion
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
		return nil, fmt.Errorf("port %d has listener %s (PID %d)", port, owners[0].Name, owners[0].PID)
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
				return d, nil, fmt.Errorf("port %d is already used by %s", port, owner)
			}
			continue
		}
		listener, err := reserveTCPPort(port)
		if err != nil {
			if !d.AutoPort {
				return d, nil, fmt.Errorf("port %d unavailable: %w; other processes were not stopped", port, err)
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
	return d, nil, errors.New("no available port found; change the preferred port and retry")
}

// StartProjects validates all selected projects before starting any new service.
// Processes run concurrently after cmd.Start; a successful receipt does not assert application readiness.
func (s *Service) StartProjects(paths []string, dryRun bool) (ServiceBatch, error) {
	return s.startServices(paths, dryRun, nil, nil)
}

// StartProject uses the configuration version displayed in the project workspace.
func (s *Service) StartProject(path, version string) (ServiceBatch, error) {
	if version == "" {
		return ServiceBatch{}, errors.New("refresh the project configuration before starting")
	}
	batch, err := s.startServices([]string{path}, false, nil, map[string]string{path: version})
	if err != nil {
		details := []string{}
		for _, item := range batch.Services {
			if item.Error != "" {
				details = append(details, item.ServiceID+": "+item.Error)
			}
		}
		if len(details) > 0 {
			err = fmt.Errorf("%w: %s", err, strings.Join(details, "; "))
		}
	}
	return batch, err
}

func (s *Service) startServices(paths []string, dryRun bool, selection map[string]string, versions map[string]string) (ServiceBatch, error) {
	s.serviceStartMu.Lock()
	defer s.serviceStartMu.Unlock()
	result := ServiceBatch{DryRun: dryRun, Services: []ServiceStartResult{}}
	if len(paths) == 0 || len(paths) > 32 {
		return result, errors.New("specify 1–32 registered projects per request")
	}
	plans, err := s.servicePlans(paths, selection)
	if err != nil {
		return result, err
	}
	for _, plan := range plans {
		if expected := versions[plan.path]; expected != "" && expected != plan.config.version {
			return result, errors.New("service configuration changed; refresh and review commands before starting")
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
		return result, errors.New("the app is shutting down")
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
					entry.Status, entry.Error = "blocked", "Running configuration changed or is stopping; review and stop the original instance first"
					blocked = true
				}
				result.Services[i] = entry
				continue
			}
		}
		if plan.configured.ObserveOnly {
			entry.State = observedServiceState(plan.configured)
			entry.Status = "observed"
			if entry.State.Status != "reachable" {
				entry.Status, entry.Error = "blocked", "External dependency is unreachable; start it using its existing owner"
				blocked = true
			}
			result.Services[i] = entry
			continue
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
			err = errors.New("up to 16 local services can run concurrently")
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
			err = errors.New("dependency endpoint changed; restart this service to update environment variables")
		}
		if err != nil {
			result.Services[i].Status, result.Services[i].Error = "blocked", err.Error()
			blocked = true
		} else {
			result.Services[i].State.Definition = resolved
		}
	}
	if blocked {
		return result, errors.New("startup preflight failed; no new services started")
	}
	// Detect configuration changes made during preflight before the first spawn.
	for _, plan := range plans {
		if _, err := s.registeredProject(plan.path); err != nil {
			return result, err
		}
		latest, err := s.localServiceConfig(plan.path)
		if err != nil || latest.version != plan.config.version {
			return result, fmt.Errorf("%s configuration changed; run preflight again", plan.path)
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
		if result.Services[i].Status == "already_running" || result.Services[i].Status == "observed" {
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
			startErr = errors.New("the app is shutting down")
		}
		if startErr == nil {
			_, startErr = s.registeredProject(plan.path)
		}
		if startErr == nil {
			latest, readErr := s.localServiceConfig(plan.path)
			if readErr != nil {
				startErr = readErr
			} else if latest.version != plan.config.version {
				startErr = errors.New("service configuration changed; run preflight again")
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
			return result, errors.New("some services failed to start; review individual results; started services remain running")
		}
		result.Services[i].Status = "started"
	}
	return result, nil
}

// StopProjects captures and cancels only instances owned at the time of this request.
func (s *Service) StopProjects(paths []string) (ServiceBatch, error) {
	result := ServiceBatch{Services: []ServiceStartResult{}}
	if len(paths) == 0 || len(paths) > 32 {
		return result, errors.New("specify 1–32 registered projects per request")
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

// ServiceReadiness checks exact managed runs and current observed dependencies without run IDs.
func (s *Service) ServiceReadiness(runs []ServiceRunRef) (ServiceBatch, error) {
	result := ServiceBatch{Services: []ServiceStartResult{}}
	if len(runs) == 0 || len(runs) > 256 {
		return result, errors.New("specify 1–256 service run IDs")
	}
	failed := false
	for _, run := range runs {
		item := ServiceStartResult{Path: run.Path, ServiceID: run.ServiceID, Status: "pending"}
		if run.RunID == "" {
			if _, err := s.registeredProject(run.Path); err != nil {
				return result, err
			}
			config, err := s.localServiceConfig(run.Path)
			if err != nil {
				return result, err
			}
			index := slices.IndexFunc(config.services, func(d LocalServiceDefinition) bool {
				return d.ID == run.ServiceID && d.ObserveOnly
			})
			if index < 0 {
				return result, errors.New("observed dependency configuration changed; refresh project status")
			}
			item.State = observedServiceState(config.services[index])
			item.PortChecked = true
			item.Status = "ready"
			if item.State.Status != "reachable" {
				item.Status, item.Error, failed = "failed", "External dependency is unreachable", true
			}
			result.Services = append(result.Services, item)
			continue
		}
		output, err := s.LocalServiceLog(run.Path, run.ServiceID, run.RunID)
		item.State = output.State
		item.PortChecked = output.State.Definition.Port != 0
		switch {
		case err != nil:
			item.Status, item.Error, failed = "failed", err.Error(), true
		case !localServiceRunning(output.State.Status) || output.State.Status == "stopping":
			item.Status, item.Error, failed = "failed", "Service exited or is stopping", true
		case !item.PortChecked:
			item.Status = "running_unchecked"
		case servicePortConnects(output.State.Definition):
			item.Status = "ready"
			item.State.Status, item.State.PortState = "running", "listening"
		}
		result.Services = append(result.Services, item)
	}
	if failed {
		return result, errors.New("some services are not ready; check individual results and logs")
	}
	return result, nil
}
