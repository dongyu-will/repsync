package control

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"time"
)

type DesktopStatus struct {
	PID    int    `json:"pid"`
	Status string `json:"status"`
	Socket string `json:"socket"`
	Log    string `json:"log,omitempty"`
}

// OpenDesktop starts the packaged application, which retains ownership after the CLI exits.
func OpenDesktop(ctx context.Context, socket string) (DesktopStatus, error) {
	ctx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()
	if response, err := Call(ctx, socket, Request{Action: "ping"}); err == nil && response.OK {
		var status DesktopStatus
		_ = json.Unmarshal(response.Data, &status)
		status.Status, status.Socket = "already_running", socket
		return status, nil
	}
	if err := ctx.Err(); err != nil {
		return DesktopStatus{}, err
	}
	executable := os.Getenv("REPSYNC_APP")
	if executable == "" {
		var err error
		executable, err = os.Executable()
		if err != nil {
			return DesktopStatus{}, err
		}
		executable, err = filepath.EvalSymlinks(executable)
		if err != nil {
			return DesktopStatus{}, err
		}
		if !strings.Contains(executable, ".app/Contents/MacOS/") {
			return DesktopStatus{}, errors.New("this is a development CLI build; set REPSYNC_APP to a packaged desktop executable or run go tool mygo dev")
		}
	}
	if !filepath.IsAbs(executable) {
		return DesktopStatus{}, errors.New("REPSYNC_APP must be the absolute path to the app executable")
	}
	if err := os.MkdirAll(filepath.Dir(socket), 0700); err != nil {
		return DesktopStatus{}, err
	}
	logPath := socket + ".startup.log"
	if info, err := os.Lstat(logPath); err == nil && !info.Mode().IsRegular() {
		return DesktopStatus{}, errors.New("startup log path must be a regular file")
	}
	log, err := os.OpenFile(logPath, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0600)
	if err != nil {
		return DesktopStatus{}, err
	}
	defer log.Close()
	command := exec.Command(executable)
	command.Stdout, command.Stderr = log, log
	command.SysProcAttr = &syscall.SysProcAttr{Setsid: true}
	if err := command.Start(); err != nil {
		return DesktopStatus{}, err
	}
	result := DesktopStatus{PID: command.Process.Pid, Status: "started", Socket: socket, Log: logPath}
	finished := make(chan error, 1)
	go func() { finished <- command.Wait() }()
	ticker := time.NewTicker(150 * time.Millisecond)
	defer ticker.Stop()
	for {
		if response, err := Call(ctx, socket, Request{Action: "ping"}); err == nil && response.OK {
			var owner DesktopStatus
			if err := json.Unmarshal(response.Data, &owner); err == nil {
				result.PID = owner.PID
			}
			return result, nil
		}
		select {
		case <-ctx.Done():
			return result, fmt.Errorf("waiting for control endpoint failed: %w; see %s; app may still be starting", ctx.Err(), logPath)
		case err := <-finished:
			// A concurrent launch may have acquired the single-owner lock first.
			if response, callErr := Call(ctx, socket, Request{Action: "ping"}); callErr == nil && response.OK {
				result.Status = "already_running"
				var owner DesktopStatus
				if err := json.Unmarshal(response.Data, &owner); err == nil {
					result.PID = owner.PID
				}
				return result, nil
			}
			return result, fmt.Errorf("app exited before the control endpoint was ready: %v; see %s", err, logPath)
		case <-ticker.C:
		}
	}
}
