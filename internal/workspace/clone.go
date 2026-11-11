package workspace

import (
	"context"
	"crypto/rand"
	"errors"
	"fmt"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"time"
	"unicode"
)

type CloneState struct {
	ID          string  `json:"id"`
	Remote      string  `json:"remote"`
	Destination string  `json:"destination"`
	Status      string  `json:"status"`
	Log         string  `json:"log"`
	Error       string  `json:"error"`
	Cloned      bool    `json:"cloned"`
	Project     Project `json:"project"`
}

type activeClone struct {
	mu     sync.Mutex
	state  CloneState
	log    *processLog
	cancel context.CancelFunc
}

var scpRemote = regexp.MustCompile(`^[A-Za-z0-9._][A-Za-z0-9._-]*@[A-Za-z0-9][A-Za-z0-9.-]*:[^\s]+$`)

func validateCloneRemote(remote string) error {
	if remote == "" || len(remote) > 4096 || strings.IndexFunc(remote, unicode.IsControl) >= 0 {
		return errors.New("enter a valid remote repository URL")
	}
	if !strings.Contains(remote, "://") {
		if scpRemote.MatchString(remote) {
			return nil
		}
		return errors.New("enter an HTTPS or SSH repository URL, e.g. git@github.com:owner/repo.git")
	}
	address, err := url.Parse(remote)
	if err != nil || address.Hostname() == "" || strings.HasPrefix(address.Hostname(), "-") || address.Path == "" || address.Path == "/" {
		return errors.New("remote repository URL is incomplete")
	}
	switch address.Scheme {
	case "https", "http", "ssh", "git":
	default:
		return errors.New("only HTTP(S), SSH, and Git remote URLs are supported")
	}
	if address.RawQuery != "" || address.Fragment != "" {
		return errors.New("repository URL cannot contain query parameters or fragments; use a Git clone URL")
	}
	if address.User != nil {
		_, password := address.User.Password()
		if address.Scheme != "ssh" || password || strings.HasPrefix(address.User.Username(), "-") {
			return errors.New("do not put passwords or tokens in URLs; use local Git credentials or SSH keys")
		}
	}
	return nil
}

// StartClone reserves a new directory and clones without executing project commands.
func (s *Service) StartClone(remote, parent, name string) (CloneState, error) {
	remote, parent, name = strings.TrimSpace(remote), strings.TrimSpace(parent), strings.TrimSpace(name)
	if err := validateCloneRemote(remote); err != nil {
		return CloneState{}, err
	}
	if !filepath.IsAbs(parent) {
		return CloneState{}, errors.New("select the parent folder for the repository")
	}
	if name == "" || name == "." || name == ".." || filepath.Base(name) != name || strings.ContainsAny(name, "/\\\x00") {
		return CloneState{}, errors.New("folder name must be a single path component")
	}
	gitBinary, err := exec.LookPath("git")
	if err != nil {
		return CloneState{}, errors.New("Git not found; install Git and make sure the app can access it")
	}
	canonical, err := filepath.EvalSymlinks(parent)
	if err != nil {
		return CloneState{}, fmt.Errorf("parent folder inaccessible: %w", err)
	}
	root, err := os.OpenRoot(canonical)
	if err != nil {
		return CloneState{}, err
	}
	defer root.Close()
	s.taskMu.Lock()
	defer s.taskMu.Unlock()
	if s.closing {
		return CloneState{}, errors.New("the app is shutting down")
	}
	if s.activeClone != nil {
		s.activeClone.mu.Lock()
		running := s.activeClone.state.Status == "running"
		s.activeClone.mu.Unlock()
		if running {
			return CloneState{}, errors.New("a clone is already running; wait or stop it first")
		}
	}
	id := rand.Text()
	logDir := filepath.Join(filepath.Dir(s.path), "clones")
	if err := os.MkdirAll(logDir, 0700); err != nil {
		return CloneState{}, err
	}
	file, err := os.OpenFile(filepath.Join(logDir, id+".log"), os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
	if err != nil {
		return CloneState{}, err
	}
	if err := root.Mkdir(name, 0700); err != nil {
		file.Close()
		os.Remove(file.Name())
		if errors.Is(err, os.ErrExist) {
			return CloneState{}, errors.New("destination exists; use the existing local folder or choose another name")
		}
		return CloneState{}, fmt.Errorf("create destination folder: %w", err)
	}
	state := CloneState{ID: id, Remote: remote, Destination: filepath.Join(canonical, name), Status: "running"}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Minute)
	active := &activeClone{state: state, log: &processLog{file: file, tail: true}, cancel: cancel}
	s.activeClone = active
	s.taskWG.Add(1)
	go func() {
		defer s.taskWG.Done()
		defer cancel()
		s.executeClone(ctx, gitBinary, canonical, name, active)
	}()
	return state, nil
}

func (s *Service) executeClone(ctx context.Context, gitBinary, parent, name string, active *activeClone) {
	cmd := exec.CommandContext(ctx, gitBinary, "-c", "protocol.file.allow=never", "-c", "protocol.ext.allow=never", "-c", "core.hooksPath=/dev/null", "clone", "--progress", "--", active.state.Remote, name)
	cmd.Dir = parent
	env := commandEnvironment(false)
	// Keep credential helpers and SSH-agent configuration; disallow terminal prompts.
	for _, entry := range env {
		key, _, _ := strings.Cut(entry, "=")
		if key != "GIT_TERMINAL_PROMPT" && key != "GIT_ASKPASS" && key != "SSH_ASKPASS" {
			cmd.Env = append(cmd.Env, entry)
		}
	}
	cmd.Env = append(cmd.Env, "GIT_TERMINAL_PROMPT=0", "GIT_ASKPASS=/usr/bin/false", "SSH_ASKPASS=/usr/bin/false")
	if os.Getenv("GIT_SSH_COMMAND") == "" && os.Getenv("GIT_SSH") == "" {
		cmd.Env = append(cmd.Env, "GIT_SSH_COMMAND=ssh -oBatchMode=yes")
	}
	cmd.Stdout, cmd.Stderr = active.log, active.log
	configureCommandProcess(cmd, false)
	err := cmd.Run()
	logErr := active.log.Close()
	active.mu.Lock()
	defer active.mu.Unlock()
	active.state.Cloned = err == nil
	if ctx.Err() != nil {
		active.state.Status = "cancelled"
		active.state.Error = "Clone stopped. Any partial files remain in the destination folder."
		if errors.Is(ctx.Err(), context.DeadlineExceeded) {
			active.state.Status = "timed_out"
			active.state.Error = "Clone stopped after 30 minutes. Check your network and retry with a new folder."
		}
		return
	}
	if err != nil {
		active.state.Status = "failure"
		active.state.Error = "Clone failed. Review output, URL, network, and local repository access. Any partial files remain in the destination."
		return
	}
	if logErr != nil {
		active.state.Status = "failure"
		active.state.Error = "Repository cloned, but log save failed: " + logErr.Error()
		return
	}
	project, err := s.Add(active.state.Destination)
	if err != nil {
		active.state.Status = "failure"
		active.state.Error = "Repository cloned, but registration failed. Add it as an existing local folder: " + err.Error()
		return
	}
	active.state.Project = project
	active.state.Status = "success"
}

// CloneStatus returns the last clone's bounded progress output.
func (s *Service) CloneStatus(id string) (CloneState, error) {
	s.taskMu.Lock()
	active := s.activeClone
	s.taskMu.Unlock()
	if active == nil {
		return CloneState{}, errors.New("clone task not found; the app may have restarted")
	}
	active.mu.Lock()
	defer active.mu.Unlock()
	if active.state.ID != id {
		return CloneState{}, errors.New("clone task replaced")
	}
	state := active.state
	state.Log = active.log.Text()
	return state, nil
}

// CancelClone stops only the clone subprocess owned by this service.
func (s *Service) CancelClone(id string) error {
	s.taskMu.Lock()
	active := s.activeClone
	s.taskMu.Unlock()
	if active == nil {
		return errors.New("no repository is being cloned")
	}
	active.mu.Lock()
	defer active.mu.Unlock()
	if active.state.ID != id {
		return errors.New("clone task IDs do not match")
	}
	if active.state.Status == "running" {
		active.cancel()
	}
	return nil
}
