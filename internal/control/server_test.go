//go:build integration

package control_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"repsync/internal/control"
	"repsync/internal/workspace"
)

func shortSocket(t *testing.T) string {
	t.Helper()
	root, err := os.MkdirTemp("/tmp", "rs-control-")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.RemoveAll(root) })
	return filepath.Join(root, "control.sock")
}

func TestControlCLIProcess(t *testing.T) {
	index := slices.Index(os.Args, "--repsync-control-cli")
	if index < 0 {
		return
	}
	os.Exit(control.RunCLI(context.Background(), os.Args[index+1:], os.Stdout, os.Stderr))
}

func cli(t *testing.T, socket string, args ...string) (control.Response, int) {
	t.Helper()
	binary, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	command := exec.Command(binary, append([]string{"-test.run=^TestControlCLIProcess$", "--", "--repsync-control-cli"}, args...)...)
	command.Env = append(os.Environ(), "REPSYNC_SOCKET="+socket)
	var output, diagnostics bytes.Buffer
	command.Stdout = &output
	command.Stderr = &diagnostics
	err = command.Run()
	code := 0
	if err != nil {
		var exit *exec.ExitError
		if ok := errors.As(err, &exit); !ok {
			t.Fatal(err)
		}
		code = exit.ExitCode()
	}
	var result control.Response
	if err := json.Unmarshal(output.Bytes(), &result); err != nil {
		t.Fatalf("non-JSON CLI output: %q %q %v", output.String(), diagnostics.String(), err)
	}
	return result, code
}

func TestControlCLIUsesDesktopOwner(t *testing.T) {
	t.Parallel()
	socket := shortSocket(t)
	service := workspace.NewService(filepath.Join(t.TempDir(), "projects.json"))
	server, err := control.Listen(socket, service)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { service.Shutdown(); server.Close() })
	paths := []string{}
	for range 2 {
		path := t.TempDir()
		config := `{"services":[{"id":"worker","name":"Worker","command":["/bin/sh","-c","echo CONTROL-STARTED; exec /bin/sleep 30"]}]}`
		if err := os.WriteFile(filepath.Join(path, ".repsync.json"), []byte(config), 0600); err != nil {
			t.Fatal(err)
		}
		result, code := cli(t, socket, "add", path)
		if code != 0 || !result.OK {
			t.Fatalf("add: %+v", result)
		}
		p, _ := filepath.EvalSymlinks(path)
		paths = append(paths, p)
	}
	projects, _ := service.List()
	if len(projects) != 2 {
		t.Fatal("CLI wrote another registry")
	}
	preview, code := cli(t, socket, "start", "--dry-run", paths[0], paths[1])
	if code != 0 || !preview.OK {
		t.Fatalf("dry run: %+v", preview)
	}
	for _, path := range paths {
		state, _ := service.LocalServices(path)
		if state.Services[0].RunID != "" {
			t.Fatal("dry-run executed process")
		}
	}
	result, code := cli(t, socket, "start", paths[0], paths[1])
	if code != 0 || !result.OK {
		t.Fatalf("start: %+v", result)
	}
	var receipt workspace.ServiceBatch
	if err := json.Unmarshal(result.Data, &receipt); err != nil {
		t.Fatal(err)
	}
	if len(receipt.Services) != 2 {
		t.Fatalf("missing service receipts: %+v", receipt)
	}
	for i, item := range receipt.Services {
		overview, _ := service.LocalServices(paths[i])
		if overview.Services[0].RunID != item.State.RunID {
			t.Fatal("CLI and desktop have different owner")
		}
		deadline := time.Now().Add(3 * time.Second)
		for {
			logs, code := cli(t, socket, "logs", paths[i], "worker", item.State.RunID)
			if code != 0 {
				t.Fatalf("logs: %+v", logs)
			}
			var output workspace.LocalServiceOutput
			json.Unmarshal(logs.Data, &output)
			if strings.Contains(output.Log, "CONTROL-STARTED") {
				break
			}
			if time.Now().After(deadline) {
				t.Fatal("missing process output")
			}
			time.Sleep(10 * time.Millisecond)
		}
	}
	status, code := cli(t, socket, "status", paths[0])
	var states []control.ProjectStatus
	json.Unmarshal(status.Data, &states)
	if code != 0 || len(states) != 1 || states[0].Project.Path != paths[0] {
		t.Fatalf("status returned wrong projects: %+v %+v", status, states)
	}
	stale, code := cli(t, socket, "stop", paths[0], "worker", "stale")
	if code != 1 || stale.OK {
		t.Fatal("stale receipt stopped service")
	}
	for i, item := range receipt.Services {
		stopped, code := cli(t, socket, "stop", paths[i], "worker", item.State.RunID)
		if code != 0 || !stopped.OK {
			t.Fatalf("stop: %+v", stopped)
		}
	}
	info, err := os.Stat(socket)
	if err != nil || info.Mode().Perm() != 0600 {
		t.Fatalf("socket permissions: %v %v", info, err)
	}
}

func TestControlSingleOwnerAndStaleSocket(t *testing.T) {
	t.Parallel()
	socket := shortSocket(t)
	service := workspace.NewService(filepath.Join(t.TempDir(), "projects.json"))
	defer service.Shutdown()
	listener, err := net.Listen("unix", socket)
	if err != nil {
		t.Fatal(err)
	}
	listener.(*net.UnixListener).SetUnlinkOnClose(false)
	listener.Close()
	server, err := control.Listen(socket, service)
	if err != nil {
		t.Fatal(err)
	}
	defer server.Close()
	duplicate, err := control.Listen(socket, service)
	if err == nil {
		duplicate.Close()
		t.Fatal("second owner replaced active endpoint")
	}
	result, err := control.Call(context.Background(), socket, control.Request{Action: "projects"})
	if err != nil || !result.OK {
		t.Fatalf("duplicate damaged original socket: %+v %v", result, err)
	}
	server.Close()
	if _, err := os.Lstat(socket); !os.IsNotExist(err) {
		t.Fatal("socket left behind")
	}
	if err := os.WriteFile(socket, []byte("keep"), 0600); err != nil {
		t.Fatal(err)
	}
	if other, err := control.Listen(socket, service); err == nil {
		other.Close()
		t.Fatal("regular file replaced")
	}
	data, _ := os.ReadFile(socket)
	if string(data) != "keep" {
		t.Fatal("existing non-socket file deleted")
	}
}

func TestControlRejectsBadRequestsAndDisconnectedCLI(t *testing.T) {
	t.Parallel()
	socket := shortSocket(t)
	service := workspace.NewService(filepath.Join(t.TempDir(), "projects.json"))
	server, err := control.Listen(socket, service)
	if err != nil {
		t.Fatal(err)
	}
	defer server.Close()
	defer service.Shutdown()
	transport := &http.Transport{DialContext: func(ctx context.Context, _, _ string) (net.Conn, error) {
		return (&net.Dialer{}).DialContext(ctx, "unix", socket)
	}}
	defer transport.CloseIdleConnections()
	client := &http.Client{Transport: transport, Timeout: time.Second}
	for _, test := range []struct{ name, body, origin string }{
		{"unknown field", `{"action":"projects","arbitraryCommand":"bad"}`, ""},
		{"second object", `{"action":"projects"} {}`, ""},
		{"browser origin", `{"action":"projects"}`, "http://localhost:5178"},
	} {
		t.Run(test.name, func(t *testing.T) {
			request, _ := http.NewRequest(http.MethodPost, "http://local/v1/control", strings.NewReader(test.body))
			request.Header.Set("Origin", test.origin)
			response, err := client.Do(request)
			if err != nil {
				t.Fatal(err)
			}
			data, _ := io.ReadAll(response.Body)
			response.Body.Close()
			if response.StatusCode != 400 {
				t.Fatalf("accepted invalid request: %s", data)
			}
		})
	}
	bad, code := cli(t, socket, "start")
	if code != 2 || bad.OK {
		t.Fatal("invalid CLI args did not return exit 2")
	}
	server.Close()
	missing, code := cli(t, socket, "projects")
	if code != 1 || missing.OK || !strings.Contains(missing.Error, "desktop app") {
		t.Fatalf("disconnection not actionable: %+v", missing)
	}
}

func TestControlProjectLifecyclePipelinesAndSync(t *testing.T) {
	t.Parallel()
	socket := shortSocket(t)
	service := workspace.NewService(filepath.Join(t.TempDir(), "projects.json"))
	server, err := control.Listen(socket, service)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { service.Shutdown(); server.Close() })
	path := t.TempDir()
	result, code := cli(t, socket, "add", path)
	if code != 0 {
		t.Fatal(result)
	}
	var project workspace.Project
	if err := json.Unmarshal(result.Data, &project); err != nil {
		t.Fatal(err)
	}
	path = project.Path
	config := `{"services":[{"id":"worker","name":"Worker","command":["/bin/sleep","30"]}],"pipelines":[{"id":"check","name":"Check","kind":"ci","steps":[["/bin/echo","CHECK-COMPLETE"]]}]}`
	if err := os.WriteFile(filepath.Join(path, ".repsync.json"), []byte(config), 0600); err != nil {
		t.Fatal(err)
	}
	started, code := cli(t, socket, "start", "--wait", path)
	if code != 0 || !strings.Contains(string(started.Data), "running_unchecked") {
		t.Fatalf("wait: %+v", started)
	}
	if _, code := cli(t, socket, "remove", path); code != 1 {
		t.Fatal("removed running project")
	}
	if result, code := cli(t, socket, "stop-projects", path); code != 0 {
		t.Fatal(result)
	}
	configResult, code := cli(t, socket, "pipelines", path)
	if code != 0 {
		t.Fatal(configResult)
	}
	var pipelines workspace.PipelineConfig
	if err := json.Unmarshal(configResult.Data, &pipelines); err != nil {
		t.Fatal(err)
	}
	if _, code := cli(t, socket, "run", path, "check", "stale"); code != 1 {
		t.Fatal("ran stale pipeline")
	}
	checked, code := cli(t, socket, "run", "--wait", "--timeout", "10s", path, "check", pipelines.Version)
	if code != 0 || !strings.Contains(string(checked.Data), "CHECK-COMPLETE") || !strings.Contains(string(checked.Data), `"status":"success"`) {
		t.Fatalf("check: %+v", checked)
	}
	source := filepath.Join(t.TempDir(), "COMMON.md")
	if err := os.WriteFile(source, []byte("# Common\nKeep scoped checks.\n"), 0600); err != nil {
		t.Fatal(err)
	}
	if response, code := cli(t, socket, "sync-source", source); code != 0 {
		t.Fatal(response)
	}
	previewResult, code := cli(t, socket, "sync-preview", path)
	if code != 0 {
		t.Fatal(previewResult)
	}
	var preview workspace.UpdatePreview
	if err := json.Unmarshal(previewResult.Data, &preview); err != nil {
		t.Fatal(err)
	}
	applied, code := cli(t, socket, "sync-apply", preview.Token)
	if code != 0 {
		t.Fatal(applied)
	}
	var record workspace.UpdateHistory
	if err := json.Unmarshal(applied.Data, &record); err != nil {
		t.Fatal(err)
	}
	if status, code := cli(t, socket, "sync-status"); code != 0 || !strings.Contains(string(status.Data), `"status":"current"`) {
		t.Fatal(status)
	}
	if history, code := cli(t, socket, "sync-history", path); code != 0 || !strings.Contains(string(history.Data), record.ID) {
		t.Fatal(history)
	}
	if restored, code := cli(t, socket, "sync-restore", record.ID); code != 0 {
		t.Fatal(restored)
	}
	destination := t.TempDir()
	if relocated, code := cli(t, socket, "relocate", path, destination); code != 0 {
		t.Fatal(relocated)
	}
	if _, code := cli(t, socket, "remove", destination); code != 0 {
		t.Fatal("remove relocated project failed")
	}
	if _, err := os.Stat(filepath.Join(path, ".repsync.json")); err != nil {
		t.Fatal("remove deleted original files")
	}
}

func TestControlWaitTimeoutKeepsRunAndReportsFailure(t *testing.T) {
	t.Parallel()
	socket := shortSocket(t)
	service := workspace.NewService(filepath.Join(t.TempDir(), "projects.json"))
	server, err := control.Listen(socket, service)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { service.Shutdown(); server.Close() })
	project, err := service.Add(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	config := `{"services":[{"id":"web","name":"Web","command":["/bin/sleep","30"],"autoPort":true,"env":{"PORT":"{port}"}}],"pipelines":[{"id":"fail","name":"Fail","kind":"cd","steps":[["/bin/sh","-c","exit 7"]]}]}`
	if err := os.WriteFile(filepath.Join(project.Path, ".repsync.json"), []byte(config), 0600); err != nil {
		t.Fatal(err)
	}
	timeout, code := cli(t, socket, "start", "--wait", "--timeout", "100ms", project.Path)
	var batch workspace.ServiceBatch
	json.Unmarshal(timeout.Data, &batch)
	if code != 1 || len(batch.Services) != 1 || batch.Services[0].State.RunID == "" || !strings.Contains(timeout.Error, "Wait") {
		t.Fatalf("timeout lost receipt: %+v", timeout)
	}
	status, err := service.LocalServices(project.Path)
	if err != nil || status.Services[0].PID == 0 {
		t.Fatal("timeout stopped run")
	}
	pipelines, err := service.Pipelines(project.Path)
	if err != nil {
		t.Fatal(err)
	}
	failed, code := cli(t, socket, "run", "--wait", project.Path, "fail", pipelines.Version)
	if code != 1 || !strings.Contains(string(failed.Data), `"exitCode":7`) {
		t.Fatalf("failed check reported success: %+v", failed)
	}
	for _, args := range [][]string{{"start", "--wait", "--dry-run", project.Path}, {"start", "--wait", "--timeout", "0s", project.Path}, {"stop-projects"}, {"remove"}, {"relocate", project.Path}, {"run", project.Path, "fail"}} {
		if response, code := cli(t, socket, args...); code != 2 || response.OK {
			t.Fatalf("bad arguments: %v %+v", args, response)
		}
	}
}

func TestControlOpenReusesOwner(t *testing.T) {
	t.Parallel()
	socket := shortSocket(t)
	service := workspace.NewService(filepath.Join(t.TempDir(), "projects.json"))
	server, err := control.Listen(socket, service)
	if err != nil {
		t.Fatal(err)
	}
	defer server.Close()
	defer service.Shutdown()
	response, code := cli(t, socket, "open")
	if code != 0 || !strings.Contains(string(response.Data), "already_running") {
		t.Fatalf("open duplicated owner: %+v", response)
	}
}

func TestControlResourceAndDoctorWorkflow(t *testing.T) {
	t.Parallel()
	socket := shortSocket(t)
	service := workspace.NewService(filepath.Join(t.TempDir(), "projects.json"))
	server, err := control.Listen(socket, service)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { service.Shutdown(); server.Close() })
	project, err := service.Add(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	source := filepath.Join(t.TempDir(), "shared.json")
	if err := os.WriteFile(source, []byte("{\"checks\":true}\n"), 0600); err != nil {
		t.Fatal(err)
	}
	if result, code := cli(t, socket, "resource-add", "checks", source, "shared-checks.json"); code != 0 {
		t.Fatal(result)
	}
	config := `{"services":[],"pipelines":[{"id":"check","name":"Check","kind":"ci","steps":[["/bin/echo","must-not-run"]]}],"resources":["checks"]}`
	if err := os.WriteFile(filepath.Join(project.Path, ".repsync.json"), []byte(config), 0600); err != nil {
		t.Fatal(err)
	}
	if result, code := cli(t, socket, "resources"); code != 0 || !strings.Contains(string(result.Data), "checks") {
		t.Fatal(result)
	}
	if result, code := cli(t, socket, "resource-status", project.Path); code != 0 || !strings.Contains(string(result.Data), `"status":"missing"`) {
		t.Fatal(result)
	}
	diagnosed, code := cli(t, socket, "doctor", project.Path)
	if code != 0 || !strings.Contains(string(diagnosed.Data), `"ready":true`) {
		t.Fatal(diagnosed)
	}
	previewResult, code := cli(t, socket, "resource-preview", project.Path, "checks")
	if code != 0 {
		t.Fatal(previewResult)
	}
	var preview workspace.UpdatePreview
	if err := json.Unmarshal(previewResult.Data, &preview); err != nil {
		t.Fatal(err)
	}
	if result, code := cli(t, socket, "sync-apply", preview.Token); code != 0 {
		t.Fatal(result)
	}
	historyResult, code := cli(t, socket, "resource-history", project.Path)
	var history []workspace.UpdateHistory
	if err := json.Unmarshal(historyResult.Data, &history); err != nil {
		t.Fatal(err)
	}
	if code != 0 || len(history) != 1 || history[0].Target != "shared-checks.json" {
		t.Fatal(historyResult)
	}
	if result, code := cli(t, socket, "resource-remove", "checks"); code != 0 {
		t.Fatal(result)
	}
	if _, err := os.Stat(filepath.Join(project.Path, "shared-checks.json")); err != nil {
		t.Fatal("catalog removal deleted adopted file")
	}
	if result, code := cli(t, socket, "sync-restore", history[0].ID); code != 0 {
		t.Fatal(result)
	}
	if _, err := os.Stat(filepath.Join(project.Path, "shared-checks.json")); !os.IsNotExist(err) {
		t.Fatal("restore did not use resource target")
	}
	diagnosed, code = cli(t, socket, "doctor", project.Path)
	if code != 1 || !strings.Contains(string(diagnosed.Data), `"ready":false`) {
		t.Fatal("doctor did not report missing resource registration")
	}
	runs, err := service.PipelineRuns(project.Path)
	if err != nil || len(runs) != 0 {
		t.Fatal("doctor executed checks")
	}
}
