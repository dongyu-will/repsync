package workspace

import (
	"bytes"
	"context"
	"crypto/rand"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"sync"
	"syscall"
	"time"
)

type PipelineRun struct {
	ID                string           `json:"id"`
	PipelineID        string           `json:"pipelineId"`
	Name              string           `json:"name"`
	Kind              string           `json:"kind"`
	Status            string           `json:"status"`
	StartedAt         string           `json:"startedAt"`
	EndedAt           string           `json:"endedAt"`
	Commit            string           `json:"commit"`
	Worktree          string           `json:"worktree"`
	Step              int              `json:"step"`
	ExitCode          int              `json:"exitCode"`
	Error             string           `json:"error"`
	Fingerprint       string           `json:"fingerprint,omitempty"`
	EndFingerprint    string           `json:"endFingerprint,omitempty"`
	FingerprintError  string           `json:"fingerprintError,omitempty"`
	ConfigVersion     string           `json:"configVersion,omitempty"`
	Coverage          []ReviewCoverage `json:"coverage,omitempty"`
	ReviewEnvironment string           `json:"reviewEnvironment,omitempty"`
	ReviewTarget      string           `json:"reviewTarget,omitempty"`
}

type PipelineOutput struct {
	Run PipelineRun `json:"run"`
	Log string      `json:"log"`
}

type activePipeline struct {
	mu     sync.Mutex
	run    PipelineRun
	log    *processLog
	cancel context.CancelFunc
}

// StartPipeline requires the exact configuration the user has just reviewed.
func (s *Service) StartPipeline(path, pipelineID, version string) (PipelineRun, error) {
	config, err := s.Pipelines(path)
	if err != nil {
		return PipelineRun{}, err
	}
	if config.Version != version {
		return PipelineRun{}, errors.New("pipeline configuration changed; refresh and review commands before running")
	}
	return s.startPipeline(path, pipelineID, config)
}

func (s *Service) startPipeline(path, pipelineID string, config PipelineConfig) (PipelineRun, error) {
	index := slices.IndexFunc(config.Pipelines, func(p Pipeline) bool { return p.ID == pipelineID })
	if index < 0 {
		return PipelineRun{}, errors.New("pipeline not found; refresh")
	}
	commit, _ := git(path, "rev-parse", "HEAD")
	fingerprint, fingerprintErr := reviewFingerprint(path)
	worktree := "unknown"
	if status, err := git(path, "status", "--porcelain"); err == nil {
		worktree = "clean"
		if strings.TrimSpace(status) != "" {
			worktree = "dirty"
		}
	}
	s.taskMu.Lock()
	defer s.taskMu.Unlock()
	if s.closing {
		return PipelineRun{}, errors.New("the app is shutting down")
	}
	if _, err := s.registeredProject(path); err != nil {
		return PipelineRun{}, err
	}
	running := 0
	for project, active := range s.activePipelines {
		active.mu.Lock()
		busy := active.run.Status == "running"
		active.mu.Unlock()
		if busy {
			running++
			if project == path {
				return PipelineRun{}, errors.New("a pipeline is already running for this project")
			}
		}
	}
	if running >= 4 {
		return PipelineRun{}, errors.New("pipelines can run for up to 4 projects concurrently")
	}
	pipeline := config.Pipelines[index]
	run := PipelineRun{ID: fmt.Sprintf("%019d-%s", time.Now().UnixMilli(), rand.Text()), PipelineID: pipeline.ID, Name: pipeline.Name, Kind: pipeline.Kind, Status: "running", StartedAt: time.Now().UTC().Format(time.RFC3339Nano), Commit: strings.TrimSpace(commit), Worktree: worktree, ExitCode: -1}
	run.Fingerprint, run.ConfigVersion = fingerprint, config.Version
	run.Coverage, run.ReviewEnvironment, run.ReviewTarget = pipeline.Coverage, pipeline.ReviewEnvironment, pipeline.ReviewTarget
	if fingerprintErr != nil {
		run.FingerprintError = fingerprintErr.Error()
	}
	dir := s.pipelineDirectory(path)
	if err := saveJSON(filepath.Join(dir, run.ID+".json"), run); err != nil {
		return PipelineRun{}, err
	}
	log, err := os.OpenFile(filepath.Join(dir, run.ID+".log"), os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
	if err != nil {
		_ = os.Remove(filepath.Join(dir, run.ID+".json"))
		return PipelineRun{}, err
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Duration(pipeline.TimeoutSeconds)*time.Second)
	active := &activePipeline{run: run, log: &processLog{file: log}, cancel: cancel}
	if s.activePipelines == nil {
		s.activePipelines = map[string]*activePipeline{}
	}
	s.activePipelines[path] = active
	s.taskWG.Add(1)
	go func() {
		defer s.taskWG.Done()
		defer cancel()
		s.executePipeline(ctx, path, pipeline, active)
	}()
	return run, nil
}

func (s *Service) executePipeline(ctx context.Context, path string, pipeline Pipeline, active *activePipeline) {
	status, exitCode, detail := "success", 0, ""
	for i, args := range pipeline.Steps {
		if status != "success" {
			break
		}
		active.mu.Lock()
		active.run.Step = i + 1
		active.mu.Unlock()
		_, err := fmt.Fprintf(active.log, "\n[%d/%d] %q\n", i+1, len(pipeline.Steps), args)
		if err != nil {
			status, exitCode, detail = "failure", -1, "Failed to write log: "+err.Error()
			break
		}
		cmd, err := projectCommand(ctx, path, pipeline.Cwd, pipeline.Env, args, true)
		if err != nil {
			status, exitCode, detail = "failure", -1, err.Error()
			break
		}
		cmd.Stdout, cmd.Stderr = active.log, active.log
		configureCommandProcess(cmd, false)
		err = cmd.Run()
		if err != nil {
			status, exitCode, detail = "failure", -1, err.Error()
			if cmd.ProcessState != nil {
				exitCode = cmd.ProcessState.ExitCode()
			}
			break
		}
	}
	if ctx.Err() != nil {
		status, exitCode = "cancelled", -1
		detail = "Run stopped"
		if errors.Is(ctx.Err(), context.DeadlineExceeded) {
			status, detail = "timed_out", "Pipeline exceeded the configured timeout"
		}
	}
	endFingerprint, fingerprintErr := reviewFingerprint(path)
	active.mu.Lock()
	defer active.mu.Unlock()
	active.run.EndFingerprint = endFingerprint
	if fingerprintErr != nil {
		active.run.FingerprintError = fingerprintErr.Error()
	}
	if err := active.log.Close(); err != nil {
		status, detail = "failure", "Failed to save logs: "+err.Error()
	}
	active.run.Status, active.run.ExitCode, active.run.Error = status, exitCode, detail
	active.run.EndedAt = time.Now().UTC().Format(time.RFC3339)
	if err := saveJSON(filepath.Join(s.pipelineDirectory(path), active.run.ID+".json"), active.run); err != nil {
		active.run.Error += " Failed to save run record: " + err.Error()
	}
}

// configureCommandProcess stops the command's ordinary children as a group on macOS/Linux.
func configureCommandProcess(cmd *exec.Cmd, graceful bool) {
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	cmd.Cancel = func() error {
		signal := syscall.SIGKILL
		if graceful {
			signal = syscall.SIGTERM
		}
		return killCommandGroup(cmd, signal)
	}
	cmd.WaitDelay = 2 * time.Second
}

func killCommandGroup(cmd *exec.Cmd, signal syscall.Signal) error {
	if cmd.Process == nil {
		return os.ErrProcessDone
	}
	err := syscall.Kill(-cmd.Process.Pid, signal)
	if errors.Is(err, syscall.ESRCH) {
		return os.ErrProcessDone
	}
	return err
}

func commandEnvironment(ci bool) []string {
	env := []string{}
	for _, entry := range os.Environ() {
		name, _, _ := strings.Cut(entry, "=")
		switch name {
		case "GIT_DIR", "GIT_WORK_TREE", "GIT_INDEX_FILE", "GIT_COMMON_DIR", "GIT_OBJECT_DIRECTORY", "GIT_ALTERNATE_OBJECT_DIRECTORIES", "CI", "NO_COLOR":
			continue
		}
		env = append(env, entry)
	}
	if ci {
		env = append(env, "CI=true")
	}
	return append(env, "NO_COLOR=1")
}

func (s *Service) StopPipeline(path, id string) error {
	if _, err := s.registeredProject(path); err != nil {
		return err
	}
	s.taskMu.Lock()
	active := s.activePipelines[path]
	s.taskMu.Unlock()
	if active == nil {
		return errors.New("this pipeline is no longer running")
	}
	active.mu.Lock()
	defer active.mu.Unlock()
	if active.run.ID != id || active.run.Status != "running" {
		return errors.New("this pipeline is no longer running")
	}
	active.cancel()
	return nil
}

// Shutdown waits for owned subprocesses to stop before the application quits.
func (s *Service) Shutdown() {
	s.taskMu.Lock()
	s.closing = true
	if s.gitCleanupCancel != nil {
		s.gitCleanupCancel()
	}
	if s.activeClone != nil {
		s.activeClone.cancel()
	}
	for _, active := range s.activePipelines {
		active.cancel()
	}
	for _, project := range s.activeLocalServices {
		for _, active := range project {
			active.cancel()
		}
	}
	s.taskMu.Unlock()
	s.taskWG.Wait()
}

func (s *Service) PipelineRuns(path string) ([]PipelineRun, error) {
	if _, err := s.registeredProject(path); err != nil {
		return nil, err
	}
	runs := []PipelineRun{}
	entries, err := os.ReadDir(s.pipelineDirectory(path))
	if errors.Is(err, os.ErrNotExist) {
		return runs, nil
	}
	if err != nil {
		return nil, err
	}
	s.taskMu.Lock()
	active := s.activePipelines[path]
	s.taskMu.Unlock()
	for i := len(entries) - 1; i >= 0 && len(runs) < 20; i-- {
		entry := entries[i]
		if !strings.HasSuffix(entry.Name(), ".json") {
			continue
		}
		var run PipelineRun
		if err := readPipelineJSON(filepath.Join(s.pipelineDirectory(path), entry.Name()), &run); err != nil {
			return nil, fmt.Errorf("read run history: %w", err)
		}
		if active != nil {
			active.mu.Lock()
			if active.run.ID == run.ID {
				run = active.run
			}
			active.mu.Unlock()
		}
		if run.Status == "running" && (active == nil || active.run.ID != run.ID) {
			run.Status, run.Error = "interrupted", "Run did not finish before the previous app exit; result unknown"
		}
		runs = append(runs, run)
	}
	return runs, nil
}

var pipelineRunID = regexp.MustCompile(`^[A-Za-z0-9-]{1,100}$`)

func (s *Service) PipelineLog(path, id string) (PipelineOutput, error) {
	if _, err := s.registeredProject(path); err != nil {
		return PipelineOutput{}, err
	}
	if !pipelineRunID.MatchString(id) {
		return PipelineOutput{}, errors.New("invalid run record ID")
	}
	s.taskMu.Lock()
	active := s.activePipelines[path]
	s.taskMu.Unlock()
	if active != nil {
		active.mu.Lock()
		if active.run.ID == id {
			output := PipelineOutput{Run: active.run, Log: active.log.Text()}
			active.mu.Unlock()
			return output, nil
		}
		active.mu.Unlock()
	}
	var run PipelineRun
	dir := s.pipelineDirectory(path)
	if err := readPipelineJSON(filepath.Join(dir, id+".json"), &run); err != nil {
		return PipelineOutput{}, err
	}
	if run.Status == "running" {
		run.Status, run.Error = "interrupted", "Run did not finish before the previous app exit; result unknown"
	}
	f, err := os.Open(filepath.Join(dir, id+".log"))
	if err != nil {
		return PipelineOutput{}, err
	}
	defer f.Close()
	var output bytes.Buffer
	_, err = output.ReadFrom(io.LimitReader(f, processLogLimit+256))
	return PipelineOutput{Run: run, Log: strings.ToValidUTF8(output.String(), "�")}, err
}

func runGitHubCLI(path string, args ...string) ([]byte, error) {
	if _, err := exec.LookPath("gh"); err != nil {
		return nil, errors.New("GitHub CLI not found; install gh and run gh auth login; local CI/CD remains available")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, "gh", args...)
	cmd.Dir, cmd.Env = path, append(commandEnvironment(true), "GH_PROMPT_DISABLED=1", "GH_HOST=github.com")
	var stdout, stderr limitedBuffer
	cmd.Stdout, cmd.Stderr = &stdout, &stderr
	configureCommandProcess(cmd, false)
	if err := cmd.Run(); err != nil {
		if ctx.Err() != nil {
			return nil, errors.New("GitHub Actions request timed out; refresh later")
		}
		return nil, fmt.Errorf("failed to read GitHub Actions; check gh authentication and repository permissions: %s", strings.TrimSpace(stderr.String()))
	}
	if stdout.overflow {
		return nil, errors.New("GitHub Actions response exceeds limits")
	}
	return stdout.Bytes(), nil
}
