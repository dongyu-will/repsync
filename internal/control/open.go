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
			return DesktopStatus{}, errors.New("当前是 CLI 开发构建；请设置 REPSYNC_APP 为已打包应用的可执行文件，或运行 go tool mygo dev")
		}
	}
	if !filepath.IsAbs(executable) {
		return DesktopStatus{}, errors.New("REPSYNC_APP 必须是应用可执行文件的绝对路径")
	}
	if err := os.MkdirAll(filepath.Dir(socket), 0700); err != nil {
		return DesktopStatus{}, err
	}
	logPath := socket + ".startup.log"
	if info, err := os.Lstat(logPath); err == nil && !info.Mode().IsRegular() {
		return DesktopStatus{}, errors.New("启动日志路径必须是普通文件")
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
			return result, fmt.Errorf("等待控制入口失败: %w；查看 %s，应用可能仍在启动", ctx.Err(), logPath)
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
			return result, fmt.Errorf("应用在控制入口就绪前退出: %v；查看 %s", err, logPath)
		case <-ticker.C:
		}
	}
}
