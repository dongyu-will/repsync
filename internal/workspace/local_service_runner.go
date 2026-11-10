package workspace

import (
	"context"
	"crypto/rand"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"slices"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"
)

type activeLocalService struct {
	mu         sync.Mutex
	state      LocalServiceState
	configured LocalServiceDefinition
	log        *processLog
	cancel     context.CancelFunc
	done       chan struct{}
}

type LocalServiceOutput struct {
	State LocalServiceState `json:"state"`
	Log   string            `json:"log"`
}

// StartLocalService starts only the registered project's reviewed command.
func (s *Service) StartLocalService(path, id, version string) (LocalServiceState, error) {
	if _, err := s.registeredProject(path); err != nil {
		return LocalServiceState{}, err
	}
	config, err := s.localServiceConfig(path)
	if err != nil {
		return LocalServiceState{}, err
	}
	if config.version != version {
		return LocalServiceState{}, errors.New("服务配置已改变，请刷新并核对命令后启动")
	}
	index := slices.IndexFunc(config.services, func(d LocalServiceDefinition) bool { return d.ID == id })
	if index < 0 {
		return LocalServiceState{}, errors.New("服务配置已移除，请刷新")
	}
	definition := config.services[index]
	if len(definition.EnvFrom) > 0 {
		batch, err := s.startServices([]string{path}, false, map[string]string{path: id}, map[string]string{path: version})
		for _, item := range batch.Services {
			if item.Path == path && item.ServiceID == id {
				if item.Status == "already_running" && err == nil {
					return item.State, errors.New("此服务已启动")
				}
				return item.State, err
			}
		}
		return LocalServiceState{}, err
	}
	s.serviceStartMu.Lock()
	defer s.serviceStartMu.Unlock()
	s.taskMu.Lock()
	defer s.taskMu.Unlock()
	if s.closing {
		return LocalServiceState{}, errors.New("应用正在退出")
	}
	if _, err := s.registeredProject(path); err != nil {
		return LocalServiceState{}, err
	}
	count, ports := s.activeServicePortsLocked()
	if active := s.activeLocalServices[path][id]; active != nil {
		active.mu.Lock()
		running := localServiceRunning(active.state.Status)
		active.mu.Unlock()
		if running {
			return LocalServiceState{}, errors.New("此服务已启动")
		}
	}
	if count >= 16 {
		return LocalServiceState{}, errors.New("最多同时运行 16 个本地服务")
	}
	resolved, reservation, err := reserveServicePort(definition, ports)
	if err != nil {
		return LocalServiceState{}, err
	}
	if reservation != nil {
		reservation.Close()
	}
	return s.startLocalServiceLocked(path, definition, resolved)
}

// Caller holds taskMu; the original definition is retained for configuration comparisons.
func (s *Service) startLocalServiceLocked(path string, configured, definition LocalServiceDefinition) (LocalServiceState, error) {
	id := definition.ID
	state := LocalServiceState{Definition: definition, RunID: rand.Text(), Status: "starting", StartedAt: time.Now().UTC().Format(time.RFC3339), ExitCode: -1, PortState: "waiting"}
	if definition.Port == 0 {
		state.Status, state.PortState = "running", "none"
	}
	statePath := s.localServiceFile(path, id, ".json")
	if err := saveJSON(statePath, state); err != nil {
		return LocalServiceState{}, err
	}
	file, err := os.OpenFile(s.localServiceFile(path, id, ".log"), os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0600)
	if err != nil {
		state.Status, state.Error = "failure", "创建服务日志失败: "+err.Error()
		_ = saveJSON(statePath, state)
		return LocalServiceState{}, err
	}
	ctx, cancel := context.WithCancel(context.Background())
	active := &activeLocalService{state: state, configured: configured, log: &processLog{file: file, tail: true}, cancel: cancel, done: make(chan struct{})}
	cmd, commandErr := projectCommand(ctx, path, definition.Cwd, definition.Env, definition.Command, false)
	if commandErr != nil {
		cancel()
		active.log.Close()
		state.Status, state.Error, state.EndedAt = "failure", commandErr.Error(), time.Now().UTC().Format(time.RFC3339)
		_ = saveJSON(statePath, state)
		return state, commandErr
	}
	cmd.Stdout, cmd.Stderr = active.log, active.log
	configureCommandProcess(cmd, true)
	_, logErr := fmt.Fprintf(active.log, "$ %q\n", definition.Command)
	if logErr == nil {
		err = cmd.Start()
	} else {
		err = logErr
	}
	if err != nil {
		cancel()
		active.log.Close()
		state.Status, state.Error, state.EndedAt = "failure", err.Error(), time.Now().UTC().Format(time.RFC3339)
		_ = saveJSON(statePath, state)
		return state, fmt.Errorf("启动 %s 失败: %w", definition.Name, err)
	}
	active.state.PID = cmd.Process.Pid
	if err := saveJSON(statePath, active.state); err != nil {
		cancel()
		_ = cmd.Wait()
		_ = killCommandGroup(cmd, syscall.SIGKILL)
		active.log.Close()
		return LocalServiceState{}, fmt.Errorf("保存服务状态失败，已停止启动进程: %w", err)
	}
	if s.activeLocalServices == nil {
		s.activeLocalServices = map[string]map[string]*activeLocalService{}
	}
	if s.activeLocalServices[path] == nil {
		s.activeLocalServices[path] = map[string]*activeLocalService{}
	}
	s.activeLocalServices[path][id] = active
	result := active.state
	s.taskWG.Add(1)
	go s.waitLocalService(ctx, path, active, cmd)
	return result, nil
}

func (s *Service) waitLocalService(ctx context.Context, path string, active *activeLocalService, cmd *exec.Cmd) {
	defer s.taskWG.Done()
	defer close(active.done)
	defer active.cancel()
	finished, monitored := make(chan struct{}), make(chan struct{})
	go func() {
		defer close(monitored)
		ticker := time.NewTicker(750 * time.Millisecond)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-finished:
				return
			case <-ticker.C:
				active.mu.Lock()
				definition := active.state.Definition
				active.mu.Unlock()
				if definition.Port == 0 {
					continue
				}
				listening := servicePortConnects(definition)
				active.mu.Lock()
				if active.state.Status != "stopping" {
					active.state.PortState = "waiting"
					if listening {
						active.state.Status, active.state.PortState = "running", "listening"
					}
				}
				active.mu.Unlock()
			}
		}
	}()
	err := cmd.Wait()
	// WaitDelay bounds graceful shutdown; also remove children that outlive the parent.
	_ = killCommandGroup(cmd, syscall.SIGKILL)
	close(finished)
	<-monitored
	active.mu.Lock()
	active.state.Status, active.state.ExitCode = "exited", cmd.ProcessState.ExitCode()
	if err != nil {
		active.state.Status, active.state.Error = "failure", err.Error()
	}
	if ctx.Err() != nil {
		active.state.Status, active.state.Error = "stopped", ""
	}
	if err := active.log.Close(); err != nil {
		active.state.Error += " 保存日志失败: " + err.Error()
	}
	active.state.PID, active.state.EndedAt = 0, time.Now().UTC().Format(time.RFC3339)
	active.state.PortState = "unknown"
	if err := saveJSON(s.localServiceFile(path, active.state.Definition.ID, ".json"), active.state); err != nil {
		active.state.Error += " 保存记录失败: " + err.Error()
	}
	id := active.state.Definition.ID
	active.mu.Unlock()
	s.taskMu.Lock()
	if s.activeLocalServices[path][id] == active {
		delete(s.activeLocalServices[path], id)
	}
	s.taskMu.Unlock()
}

// StopLocalService never accepts a PID and cannot terminate an external process.
func (s *Service) StopLocalService(path, id, runID string) error {
	if _, err := s.registeredProject(path); err != nil {
		return err
	}
	s.taskMu.Lock()
	active := s.activeLocalServices[path][id]
	s.taskMu.Unlock()
	if active == nil {
		return errors.New("服务已经停止或不属于本次应用")
	}
	active.mu.Lock()
	if active.state.RunID != runID {
		active.mu.Unlock()
		return errors.New("服务已经重新启动，请刷新后操作")
	}
	if !localServiceRunning(active.state.Status) {
		active.mu.Unlock()
		return nil
	}
	active.state.Status = "stopping"
	active.mu.Unlock()
	active.cancel()
	<-active.done
	return nil
}

func (s *Service) LocalServiceLog(path, id, runID string) (LocalServiceOutput, error) {
	if _, err := s.registeredProject(path); err != nil {
		return LocalServiceOutput{}, err
	}
	s.taskMu.Lock()
	active := s.activeLocalServices[path][id]
	s.taskMu.Unlock()
	if active != nil {
		active.mu.Lock()
		state := active.state
		active.mu.Unlock()
		if state.RunID != runID {
			return LocalServiceOutput{}, errors.New("服务已重新启动，请刷新日志")
		}
		return LocalServiceOutput{State: state, Log: active.log.Text()}, nil
	}
	var state LocalServiceState
	if err := readPipelineJSON(s.localServiceFile(path, id, ".json"), &state); err != nil {
		return LocalServiceOutput{}, err
	}
	if state.RunID != runID {
		return LocalServiceOutput{}, errors.New("服务已重新启动，请刷新日志")
	}
	if localServiceRunning(state.Status) {
		state.Status, state.PID = "interrupted", 0
	}
	f, err := os.Open(s.localServiceFile(path, id, ".log"))
	if err != nil {
		return LocalServiceOutput{}, err
	}
	defer f.Close()
	data, err := io.ReadAll(io.LimitReader(f, processLogLimit))
	if err != nil {
		return LocalServiceOutput{}, err
	}
	var current LocalServiceState
	if err := readPipelineJSON(s.localServiceFile(path, id, ".json"), &current); err != nil {
		return LocalServiceOutput{}, err
	}
	if current.RunID != runID {
		return LocalServiceOutput{}, errors.New("服务已重新启动，请刷新日志")
	}
	return LocalServiceOutput{State: state, Log: strings.ToValidUTF8(string(data), "�")}, err
}

type LocalPortOwner struct {
	PID  int    `json:"pid"`
	Name string `json:"name"`
}

func (s *Service) InspectLocalServicePort(path, id string) ([]LocalPortOwner, error) {
	overview, err := s.LocalServices(path)
	if err != nil {
		return nil, err
	}
	index := slices.IndexFunc(overview.Services, func(service LocalServiceState) bool { return service.Definition.ID == id })
	if index < 0 || overview.Services[index].Definition.Port == 0 {
		return nil, errors.New("此服务未配置端口")
	}
	return localPortOwners(overview.Services[index].Definition.Port)
}

func localPortOwners(port int) ([]LocalPortOwner, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, "lsof", "-nP", "-a", fmt.Sprintf("-iTCP:%d", port), "-sTCP:LISTEN", "-Fpc")
	var out, stderr limitedBuffer
	cmd.Stdout, cmd.Stderr = &out, &stderr
	if err := cmd.Run(); err != nil {
		var exit *exec.ExitError
		if errors.As(err, &exit) && exit.ExitCode() == 1 && out.Len() == 0 && stderr.Len() == 0 {
			return []LocalPortOwner{}, nil
		}
		return nil, fmt.Errorf("读取端口占用失败: %w %s", err, stderr.String())
	}
	owners := []LocalPortOwner{}
	for _, line := range strings.Split(out.String(), "\n") {
		if strings.HasPrefix(line, "p") {
			if pid, err := strconv.Atoi(line[1:]); err == nil {
				owners = append(owners, LocalPortOwner{PID: pid})
			}
		}
		if strings.HasPrefix(line, "c") && len(owners) > 0 {
			owners[len(owners)-1].Name = line[1:]
		}
	}
	return owners, nil
}
