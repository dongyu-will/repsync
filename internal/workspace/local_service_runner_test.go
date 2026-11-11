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
	"slices"
	"strconv"
	"strings"
	"testing"
	"time"

	"repsync/internal/workspace"
)

// This helper runs in an owned subprocess, with no third-party runtime or real repository.
func TestLocalServiceProcess(t *testing.T) {
	index := slices.Index(os.Args, "--repsync-service")
	if index < 0 {
		return
	}
	mode := os.Args[index+1]
	if mode == "client" {
		endpoint := os.Getenv("BASE_URL")
		response, err := http.Get(endpoint)
		if err != nil {
			t.Fatal(err)
		}
		response.Body.Close()
		if err := os.WriteFile("received-url", []byte(endpoint), 0600); err != nil {
			t.Fatal(err)
		}
		mode = "0"
	}
	if mode == "--port" {
		mode = os.Args[index+2]
	} else if value, ok := strings.CutPrefix(mode, "--port="); ok {
		mode = value
	} else if mode == "env" {
		mode = os.Getenv("PORT")
	}
	if mode == "log" {
		fmt.Print("FIRST-OUTPUT\n" + strings.Repeat("x", 300000) + "\nLAST-OUTPUT\n")
		return
	}
	port, _ := strconv.Atoi(mode)
	listener, err := net.Listen("tcp", net.JoinHostPort("127.0.0.1", strconv.Itoa(port)))
	if err != nil {
		t.Fatal(err)
	}
	wd, _ := os.Getwd()
	fmt.Printf("CWD=%s CI=%s\n", wd, os.Getenv("CI"))
	fmt.Fprintln(os.Stderr, "service stderr")
	if err := http.Serve(listener, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { fmt.Fprint(w, "owned-service") })); err != nil {
		t.Fatal(err)
	}
}

func serviceCommand(t *testing.T, mode string) []string {
	t.Helper()
	binary, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	return []string{binary, "-test.run=^TestLocalServiceProcess$", "--", "--repsync-service", mode}
}

func availableServicePort(t *testing.T) int {
	t.Helper()
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	port := listener.Addr().(*net.TCPAddr).Port
	listener.Close()
	return port
}

func writeLocalServices(t *testing.T, path string, definitions []workspace.LocalServiceDefinition) {
	t.Helper()
	data, err := json.Marshal(map[string]any{"services": definitions})
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(path, ".repsync.json"), data, 0600); err != nil {
		t.Fatal(err)
	}
}

func localServiceFixture(t *testing.T, definition workspace.LocalServiceDefinition) (*workspace.Service, string, workspace.LocalServicesOverview) {
	t.Helper()
	root := t.TempDir()
	project := filepath.Join(root, "service project")
	if err := os.Mkdir(project, 0700); err != nil {
		t.Fatal(err)
	}
	writeLocalServices(t, project, []workspace.LocalServiceDefinition{definition})
	s := workspace.NewService(filepath.Join(root, "config", "projects.json"))
	t.Cleanup(s.Shutdown)
	p, err := s.Add(project)
	if err != nil {
		t.Fatal(err)
	}
	overview, err := s.LocalServices(p.Path)
	if err != nil || overview.Error != "" {
		t.Fatalf("cannot load service: %+v %v", overview, err)
	}
	return s, p.Path, overview
}

func waitLocalService(t *testing.T, s *workspace.Service, path string, predicate func(workspace.LocalServiceState) bool) workspace.LocalServiceState {
	t.Helper()
	deadline := time.Now().Add(8 * time.Second)
	for time.Now().Before(deadline) {
		overview, err := s.LocalServices(path)
		if err != nil {
			t.Fatal(err)
		}
		if len(overview.Services) != 1 {
			t.Fatalf("unexpected services: %+v", overview)
		}
		if predicate(overview.Services[0]) {
			return overview.Services[0]
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatal("service did not reach expected state")
	return workspace.LocalServiceState{}
}

func TestLocalServiceLifecycleAndConfigChanges(t *testing.T) {
	t.Parallel()
	port := availableServicePort(t)
	definition := workspace.LocalServiceDefinition{ID: "web", Name: "Private web", Command: serviceCommand(t, strconv.Itoa(port)), URL: fmt.Sprintf("http://127.0.0.1:%d", port)}
	s, path, config := localServiceFixture(t, definition)
	if _, err := s.StartLocalService(path, "web", "stale"); err == nil {
		t.Fatal("stale config started")
	}
	if _, err := s.StartLocalService(filepath.Dir(path), "web", config.Version); err == nil {
		t.Fatal("unregistered project started")
	}
	first, err := s.StartLocalService(path, "web", config.Version)
	if err != nil {
		t.Fatal(err)
	}
	if first.PID <= 0 {
		t.Fatalf("missing owned PID: %+v", first)
	}
	running := waitLocalService(t, s, path, func(state workspace.LocalServiceState) bool {
		return state.Status == "running" && state.PortState == "listening"
	})
	client := &http.Client{Timeout: time.Second}
	response, err := client.Get(definition.URL)
	if err != nil {
		t.Fatal(err)
	}
	body, err := io.ReadAll(response.Body)
	response.Body.Close()
	if err != nil || string(body) != "owned-service" {
		t.Fatalf("wrong server: %s %v", body, err)
	}
	output, err := s.LocalServiceLog(path, "web", first.RunID)
	if err != nil || !strings.Contains(output.Log, "CWD="+path+" CI=\n") || !strings.Contains(output.Log, "service stderr") {
		t.Fatalf("wrong cwd/env/log: %+v %v", output, err)
	}
	owners, err := s.InspectLocalServicePort(path, "web")
	if err != nil || !slices.ContainsFunc(owners, func(owner workspace.LocalPortOwner) bool { return owner.PID == running.PID }) {
		t.Fatalf("wrong port ownership: %+v %v", owners, err)
	}
	if _, err := s.StartLocalService(path, "web", config.Version); err == nil {
		t.Fatal("duplicate service started")
	}
	if err := s.StopLocalService(path, "web", "wrong-instance"); err == nil {
		t.Fatal("stale stop accepted")
	}
	definition.Command = append(definition.Command, "updated-command")
	writeLocalServices(t, path, []workspace.LocalServiceDefinition{definition})
	overview, err := s.LocalServices(path)
	if err != nil || !overview.Services[0].ConfigChanged || overview.Services[0].PID != first.PID {
		t.Fatalf("running config silently replaced: %+v %v", overview, err)
	}
	// A malformed config must still leave the owned service stoppable.
	if err := os.WriteFile(filepath.Join(path, ".repsync.json"), []byte("invalid json"), 0600); err != nil {
		t.Fatal(err)
	}
	overview, err = s.LocalServices(path)
	if err != nil || overview.Error == "" || len(overview.Services) != 1 || overview.Services[0].PID != first.PID {
		t.Fatalf("lost stop controls: %+v %v", overview, err)
	}
	if err := s.StopLocalService(path, "web", first.RunID); err != nil {
		t.Fatal(err)
	}
	writeLocalServices(t, path, []workspace.LocalServiceDefinition{definition})
	overview, err = s.LocalServices(path)
	if err != nil || overview.Services[0].Status != "stopped" || overview.Services[0].PortState != "available" {
		t.Fatalf("service did not stop: %+v %v", overview, err)
	}
	second, err := s.StartLocalService(path, "web", overview.Version)
	if err != nil || second.RunID == first.RunID {
		t.Fatalf("restart failed: %+v %v", second, err)
	}
	if err := s.StopLocalService(path, "web", first.RunID); err == nil {
		t.Fatal("old view stopped restarted service")
	}
	if err := s.StopLocalService(path, "web", second.RunID); err != nil {
		t.Fatal(err)
	}
	reopened := workspace.NewService(filepath.Join(filepath.Dir(path), "config", "projects.json"))
	persisted, err := reopened.LocalServiceLog(path, "web", second.RunID)
	if err != nil || persisted.State.Status != "stopped" {
		t.Fatalf("service log not persisted: %+v %v", persisted, err)
	}
}

func TestLocalServiceNeverTakesOverOccupiedPort(t *testing.T) {
	t.Parallel()
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()
	port := listener.Addr().(*net.TCPAddr).Port
	s, path, overview := localServiceFixture(t, workspace.LocalServiceDefinition{ID: "web", Name: "Web", Command: []string{"/bin/sh", "-c", "touch must-not-start"}, Port: port})
	if overview.Services[0].PortState != "occupied" {
		t.Fatalf("occupied port was not detected: %+v", overview)
	}
	if _, err := s.StartLocalService(path, "web", overview.Version); err == nil {
		t.Fatal("started on occupied port")
	}
	if err := s.StopLocalService(path, "web", strconv.Itoa(os.Getpid())); err == nil {
		t.Fatal("accepted an external PID as a run id")
	}
	connection, err := net.DialTimeout("tcp", listener.Addr().String(), time.Second)
	if err != nil {
		t.Fatalf("external listener stopped: %v", err)
	}
	connection.Close()
	if _, err := os.Stat(filepath.Join(path, "must-not-start")); !os.IsNotExist(err) {
		t.Fatal("conflicting service command executed")
	}
}

func TestLocalServiceExitFailuresAndRecentLogs(t *testing.T) {
	t.Parallel()
	for _, test := range []struct {
		name    string
		command func(*testing.T) []string
		status  string
		code    int
	}{
		{"missing binary", func(t *testing.T) []string { return []string{"repsync-nonexistent-service-binary"} }, "failure", -1},
		{"normal exit", func(t *testing.T) []string { return []string{"/bin/sh", "-c", "echo finished"} }, "exited", 0},
		{"failed exit", func(t *testing.T) []string { return []string{"/bin/sh", "-c", "echo failure >&2; exit 9"} }, "failure", 9},
		{"bounded recent log", func(t *testing.T) []string { return serviceCommand(t, "log") }, "exited", 0},
	} {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			s, path, config := localServiceFixture(t, workspace.LocalServiceDefinition{ID: "worker", Name: "Worker", Command: test.command(t)})
			run, err := s.StartLocalService(path, "worker", config.Version)
			if test.name == "missing binary" {
				if err == nil {
					t.Fatal("missing executable reported started")
				}
			} else if err != nil {
				t.Fatal(err)
			}
			state := waitLocalService(t, s, path, func(state workspace.LocalServiceState) bool { return state.Status == test.status })
			if state.ExitCode != test.code {
				t.Fatalf("wrong exit code: %+v", state)
			}
			output, err := s.LocalServiceLog(path, "worker", run.RunID)
			if err != nil {
				t.Fatal(err)
			}
			if test.name == "bounded recent log" && (len(output.Log) > 256*1024 || !strings.Contains(output.Log, "LAST-OUTPUT") || strings.Contains(output.Log, "FIRST-OUTPUT")) {
				t.Fatalf("log did not retain bounded recent output: %d bytes", len(output.Log))
			}
		})
	}
}

func TestLocalServiceShutdownStopsChildren(t *testing.T) {
	t.Parallel()
	s, path, config := localServiceFixture(t, workspace.LocalServiceDefinition{ID: "child", Name: "Child", Command: []string{"/bin/sh", "-c", "trap '' TERM; (sleep 3; touch leaked-child) & echo child-started; wait"}})
	run, err := s.StartLocalService(path, "child", config.Version)
	if err != nil {
		t.Fatal(err)
	}
	deadline := time.Now().Add(3 * time.Second)
	for {
		output, err := s.LocalServiceLog(path, "child", run.RunID)
		if err != nil {
			t.Fatal(err)
		}
		if strings.Contains(output.Log, "\nchild-started\n") {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("child did not start")
		}
		time.Sleep(10 * time.Millisecond)
	}
	s.Shutdown()
	state := waitLocalService(t, s, path, func(state workspace.LocalServiceState) bool { return state.Status == "stopped" })
	if state.PID != 0 {
		t.Fatalf("shutdown retained an owned PID: %+v", state)
	}
	time.Sleep(1100 * time.Millisecond)
	if _, err := os.Stat(filepath.Join(path, "leaked-child")); !os.IsNotExist(err) {
		t.Fatal("child survived shutdown")
	}
}

func TestLocalServiceConfigDiscoveryAndBoundaries(t *testing.T) {
	t.Parallel()
	s, path, _ := localServiceFixture(t, workspace.LocalServiceDefinition{ID: "web", Name: "Web", Command: []string{"/bin/true"}})
	if err := os.WriteFile(filepath.Join(path, "package.json"), []byte(`{"packageManager":"pnpm@10.0.0","scripts":{"dev":"vite --port 8123","start":"echo unused"}}`), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(path, ".repsync.json"), []byte(`{"pipelines":[]}`), 0600); err != nil {
		t.Fatal(err)
	}
	overview, err := s.LocalServices(path)
	if err != nil || overview.Error != "" || len(overview.Services) != 1 || overview.Source != "package.json" || overview.Services[0].Definition.Port != 8123 || overview.Services[0].Definition.Command[0] != "pnpm" || overview.Services[0].Definition.URL != "" {
		t.Fatalf("incorrect discovery: %+v %v", overview, err)
	}
	for _, test := range []struct{ name, body string }{
		{"external URL", `{"services":[{"id":"web","name":"Web","command":["echo"],"url":"http://example.com:8123"}]}`},
		{"port mismatch", `{"services":[{"id":"web","name":"Web","command":["echo"],"port":8123,"url":"http://localhost:8000"}]}`},
		{"invalid URL port", `{"services":[{"id":"web","name":"Web","command":["echo"],"url":"http://localhost:99999"}]}`},
		{"duplicate ids", `{"services":[{"id":"web","name":"Web","command":["echo"]},{"id":"web","name":"Other","command":["echo"]}]}`},
	} {
		t.Run(test.name, func(t *testing.T) {
			if err := os.WriteFile(filepath.Join(path, ".repsync.json"), []byte(test.body), 0600); err != nil {
				t.Fatal(err)
			}
			overview, err := s.LocalServices(path)
			if err != nil || overview.Error == "" || len(overview.Services) != 0 {
				t.Fatalf("invalid config was accepted: %+v %v", overview, err)
			}
		})
	}
	writeLocalServices(t, path, []workspace.LocalServiceDefinition{})
	overview, err = s.LocalServices(path)
	if err != nil || len(overview.Services) != 0 || overview.Source != ".repsync.json" {
		t.Fatalf("explicit empty services should disable discovery: %+v %v", overview, err)
	}
	outside := filepath.Join(t.TempDir(), "outside.json")
	if err := os.WriteFile(outside, []byte(`{"services":[]}`), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(filepath.Join(path, ".repsync.json")); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, filepath.Join(path, ".repsync.json")); err != nil {
		t.Fatal(err)
	}
	overview, err = s.LocalServices(path)
	if err != nil || overview.Error == "" {
		t.Fatalf("external config symlink read: %+v %v", overview, err)
	}
}
