//go:build integration

package workspace_test

import (
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"testing"

	"repsync/internal/workspace"
)

func batchProject(t *testing.T, s *workspace.Service, d workspace.LocalServiceDefinition) string {
	t.Helper()
	root := t.TempDir()
	writeLocalServices(t, root, []workspace.LocalServiceDefinition{d})
	project, err := s.Add(root)
	if err != nil {
		t.Fatal(err)
	}
	return project.Path
}

func TestServiceBatchAutomaticPortsAndReuse(t *testing.T) {
	t.Parallel()
	external, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer external.Close()
	port := external.Addr().(*net.TCPAddr).Port
	s := workspace.NewService(filepath.Join(t.TempDir(), "projects.json"))
	t.Cleanup(s.Shutdown)
	d := workspace.LocalServiceDefinition{ID: "web", Name: "Web", Command: serviceCommand(t, "{port}"), Port: port, AutoPort: true, URL: "http://127.0.0.1:{port}", Env: map[string]string{"REPSYNC_TEST_PORT": "{port}"}}
	paths := []string{batchProject(t, s, d), batchProject(t, s, d)}
	preview, err := s.StartProjects(paths, true)
	if err != nil || len(preview.Services) != 2 {
		t.Fatalf("preflight: %+v %v", preview, err)
	}
	for _, entry := range preview.Services {
		if entry.State.PID != 0 || entry.State.RunID != "" {
			t.Fatal("dry-run spawned process")
		}
	}
	result, err := s.StartProjects(paths, false)
	if err != nil {
		t.Fatalf("start: %+v %v", result, err)
	}
	allocated := map[int]bool{port: true}
	for i, entry := range result.Services {
		actual := entry.State.Definition.Port
		if entry.Status != "started" || allocated[actual] || actual == 0 {
			t.Fatalf("port collision: %+v", result)
		}
		allocated[actual] = true
		state := waitLocalService(t, s, paths[i], func(state workspace.LocalServiceState) bool { return state.PortState == "listening" })
		if state.ConfigChanged || state.Definition.Env["REPSYNC_TEST_PORT"] != strconv.Itoa(actual) || state.Definition.URL != fmt.Sprintf("http://127.0.0.1:%d", actual) {
			t.Fatalf("effective configuration incorrect: %+v", state)
		}
		response, err := http.Get(state.Definition.URL)
		if err != nil {
			t.Fatal(err)
		}
		body, _ := io.ReadAll(response.Body)
		response.Body.Close()
		if string(body) != "owned-service" {
			t.Fatal("wrong process answered")
		}
		data, _ := os.ReadFile(filepath.Join(paths[i], ".repsync.json"))
		if !strings.Contains(string(data), "{port}") {
			t.Fatal("port allocation rewrote project config")
		}
	}
	repeated, err := s.StartProjects(paths, false)
	if err != nil {
		t.Fatal(err)
	}
	for i, entry := range repeated.Services {
		if entry.Status != "already_running" || entry.State.RunID != result.Services[i].State.RunID {
			t.Fatalf("repeated start created duplicate: %+v", repeated)
		}
	}
	if err := s.StopLocalService(paths[0], "web", "stale"); err == nil {
		t.Fatal("stale receipt stopped a service")
	}
}

func TestServiceBatchFixedPortsBeforeAutomatic(t *testing.T) {
	t.Parallel()
	port := availableServicePort(t)
	s := workspace.NewService(filepath.Join(t.TempDir(), "projects.json"))
	t.Cleanup(s.Shutdown)
	automatic := workspace.LocalServiceDefinition{ID: "web", Name: "Auto", Command: serviceCommand(t, "{port}"), Port: port, AutoPort: true, URL: "http://127.0.0.1:{port}"}
	fixed := workspace.LocalServiceDefinition{ID: "web", Name: "Fixed", Command: serviceCommand(t, strconv.Itoa(port)), Port: port}
	paths := []string{batchProject(t, s, automatic), batchProject(t, s, fixed)}
	result, err := s.StartProjects(paths, false)
	if err != nil || result.Services[0].State.Definition.Port == port || result.Services[1].State.Definition.Port != port {
		t.Fatalf("auto claimed fixed reservation: %+v %v", result, err)
	}
}

func TestServiceBatchConflictStartsNothing(t *testing.T) {
	t.Parallel()
	external, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer external.Close()
	s := workspace.NewService(filepath.Join(t.TempDir(), "projects.json"))
	t.Cleanup(s.Shutdown)
	marker := filepath.Join(t.TempDir(), "must-not-run")
	first := batchProject(t, s, workspace.LocalServiceDefinition{ID: "web", Name: "First", Command: []string{"/bin/sh", "-c", "touch '" + marker + "'; exec /bin/sleep 30"}})
	second := batchProject(t, s, workspace.LocalServiceDefinition{ID: "web", Name: "Conflict", Command: []string{"/bin/echo", "must-not-run"}, Port: external.Addr().(*net.TCPAddr).Port})
	result, err := s.StartProjects([]string{first, second}, false)
	if err == nil || len(result.Services) != 2 || result.Services[1].Status != "blocked" {
		t.Fatalf("missing conflict: %+v %v", result, err)
	}
	if _, err := os.Stat(marker); !os.IsNotExist(err) {
		t.Fatal("preflight failure executed a command")
	}
	for _, path := range []string{first, second} {
		overview, _ := s.LocalServices(path)
		for _, state := range overview.Services {
			if state.RunID != "" {
				t.Fatal("partial startup during preflight failure")
			}
		}
	}
}

func TestServiceBatchConcurrentRequestsShareRuns(t *testing.T) {
	t.Parallel()
	s := workspace.NewService(filepath.Join(t.TempDir(), "projects.json"))
	t.Cleanup(s.Shutdown)
	paths := []string{batchProject(t, s, workspace.LocalServiceDefinition{ID: "web", Name: "Web", Command: []string{"/bin/sleep", "30"}})}
	var wg sync.WaitGroup
	results := make(chan workspace.ServiceBatch, 2)
	failures := make(chan error, 2)
	for range 2 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			result, err := s.StartProjects(paths, false)
			results <- result
			failures <- err
		}()
	}
	wg.Wait()
	close(results)
	close(failures)
	for err := range failures {
		if err != nil {
			t.Fatal(err)
		}
	}
	runID := ""
	starts := 0
	for result := range results {
		entry := result.Services[0]
		if entry.Status == "started" {
			starts++
		}
		if runID != "" && entry.State.RunID != runID {
			t.Fatal("concurrent calls duplicated process")
		}
		runID = entry.State.RunID
	}
	if starts != 1 {
		t.Fatalf("expected one owner, got %d", starts)
	}
}

func TestServiceBatchPortConfigurationValidation(t *testing.T) {
	t.Parallel()
	for _, test := range []struct {
		name string
		d    workspace.LocalServiceDefinition
	}{
		{"missing command token", workspace.LocalServiceDefinition{ID: "web", Name: "Web", Command: []string{"/bin/echo", "5173"}, AutoPort: true}},
		{"fixed URL with auto", workspace.LocalServiceDefinition{ID: "web", Name: "Web", Command: []string{"/bin/echo", "{port}"}, AutoPort: true, URL: "http://127.0.0.1:5173"}},
	} {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			s := workspace.NewService(filepath.Join(t.TempDir(), "projects.json"))
			path := batchProject(t, s, test.d)
			if _, err := s.StartProjects([]string{path}, false); err == nil {
				t.Fatal("invalid automatic port configuration accepted")
			}
		})
	}
	// OS-selected ports are supported when there is no preferred port.
	s := workspace.NewService(filepath.Join(t.TempDir(), "projects.json"))
	t.Cleanup(s.Shutdown)
	d := workspace.LocalServiceDefinition{ID: "web", Name: "Web", Command: serviceCommand(t, "{port}"), AutoPort: true, URL: "http://127.0.0.1:{port}"}
	path := batchProject(t, s, d)
	result, err := s.StartProjects([]string{path}, false)
	if err != nil || result.Services[0].State.Definition.Port == 0 {
		t.Fatalf("OS port assignment: %+v %v", result, err)
	}
	data, _ := json.Marshal(result)
	if strings.Contains(string(data), `"port":0`) {
		t.Fatalf("unassigned active port: %s", data)
	}
}

func TestServiceBatchDetectsIPv6Conflict(t *testing.T) {
	t.Parallel()
	external, err := net.Listen("tcp6", "[::1]:0")
	if err != nil {
		t.Skip("IPv6 unavailable")
	}
	defer external.Close()
	port := external.Addr().(*net.TCPAddr).Port
	s := workspace.NewService(filepath.Join(t.TempDir(), "projects.json"))
	t.Cleanup(s.Shutdown)
	path := batchProject(t, s, workspace.LocalServiceDefinition{ID: "web", Name: "Web", Command: []string{"/bin/echo", "must-not-run"}, Port: port})
	result, err := s.StartProjects([]string{path}, false)
	if err == nil || result.Services[0].Status != "blocked" {
		t.Fatalf("IPv6 conflict missed: %+v %v", result, err)
	}
	connection, err := net.Dial("tcp6", external.Addr().String())
	if err != nil {
		t.Fatal("external listener was disturbed")
	}
	connection.Close()
}

func TestServiceBatchMissingExecutableStartsNothing(t *testing.T) {
	t.Parallel()
	s := workspace.NewService(filepath.Join(t.TempDir(), "projects.json"))
	t.Cleanup(s.Shutdown)
	first := batchProject(t, s, workspace.LocalServiceDefinition{ID: "web", Name: "First", Command: []string{"/bin/sleep", "30"}})
	missing := filepath.Join(t.TempDir(), "missing-executable")
	second := batchProject(t, s, workspace.LocalServiceDefinition{ID: "web", Name: "Second", Command: []string{missing}})
	result, err := s.StartProjects([]string{first, second}, false)
	if err == nil || result.Services[1].Status != "blocked" {
		t.Fatalf("missing executable passed preflight: %+v %v", result, err)
	}
	overview, _ := s.LocalServices(first)
	if overview.Services[0].RunID != "" {
		t.Fatal("first service started before invalid executable was detected")
	}
}

func TestServiceBatchReportsPartialSpawnFailure(t *testing.T) {
	t.Parallel()
	s := workspace.NewService(filepath.Join(t.TempDir(), "projects.json"))
	t.Cleanup(s.Shutdown)
	invalid := filepath.Join(t.TempDir(), "invalid-executable")
	if err := os.WriteFile(invalid, []byte("invalid executable format"), 0700); err != nil {
		t.Fatal(err)
	}
	first := batchProject(t, s, workspace.LocalServiceDefinition{ID: "web", Name: "First", Command: []string{"/bin/sleep", "30"}})
	second := batchProject(t, s, workspace.LocalServiceDefinition{ID: "web", Name: "Second", Command: []string{invalid}})
	third := batchProject(t, s, workspace.LocalServiceDefinition{ID: "web", Name: "Third", Command: []string{"/bin/sleep", "30"}})
	result, err := s.StartProjects([]string{first, second, third}, false)
	if err == nil || result.Services[0].Status != "started" || result.Services[1].Status != "failed" || result.Services[2].Status != "not_started" {
		t.Fatalf("partial result hidden: %+v %v", result, err)
	}
	if err := s.StopLocalService(first, "web", result.Services[0].State.RunID); err != nil {
		t.Fatal(err)
	}
}

func TestServiceBatchReadinessAndSelectedStop(t *testing.T) {
	t.Parallel()
	s := workspace.NewService(filepath.Join(t.TempDir(), "projects.json"))
	t.Cleanup(s.Shutdown)
	web := batchProject(t, s, workspace.LocalServiceDefinition{ID: "web", Name: "Web", Command: serviceCommand(t, "{port}"), AutoPort: true, URL: "http://127.0.0.1:{port}"})
	worker := batchProject(t, s, workspace.LocalServiceDefinition{ID: "worker", Name: "Worker", Command: []string{"/bin/sleep", "30"}})
	started, err := s.StartProjects([]string{web, worker}, false)
	if err != nil {
		t.Fatal(err)
	}
	waitLocalService(t, s, web, func(state workspace.LocalServiceState) bool { return state.PortState == "listening" })
	refs := []workspace.ServiceRunRef{}
	for _, item := range started.Services {
		refs = append(refs, workspace.ServiceRunRef{Path: item.Path, ServiceID: item.ServiceID, RunID: item.State.RunID})
	}
	readiness, err := s.ServiceReadiness(refs)
	if err != nil || readiness.Services[0].Status != "ready" || readiness.Services[1].Status != "running_unchecked" {
		t.Fatalf("readiness: %+v %v", readiness, err)
	}
	if _, err := s.StopProjects([]string{web, "/unregistered"}); err == nil {
		t.Fatal("partial stop before registration precheck")
	}
	readiness, err = s.ServiceReadiness(refs)
	if err != nil {
		t.Fatal("bad request stopped valid project:", err)
	}
	stopped, err := s.StopProjects([]string{web, web})
	if err != nil || len(stopped.Services) != 1 || stopped.Services[0].State.RunID != refs[0].RunID {
		t.Fatalf("selected stop: %+v %v", stopped, err)
	}
	if _, err := s.ServiceReadiness(refs[1:]); err != nil {
		t.Fatal("stopped another project:", err)
	}
	replaced, err := s.StartProjects([]string{web}, false)
	if err != nil || replaced.Services[0].State.RunID == refs[0].RunID {
		t.Fatalf("restart: %+v %v", replaced, err)
	}
	if _, err := s.ServiceReadiness(refs[:1]); err == nil {
		t.Fatal("wait followed replacement run")
	}
}
