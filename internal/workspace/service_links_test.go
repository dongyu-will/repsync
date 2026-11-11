//go:build integration

package workspace_test

import (
	"fmt"
	"net"
	"os"
	"path/filepath"
	"repsync/internal/workspace"
	"strconv"
	"strings"
	"testing"
	"time"
)

func TestServiceLinksStartDependenciesAndInjectActualEndpoint(t *testing.T) {
	external, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer external.Close()
	port := external.Addr().(*net.TCPAddr).Port
	s := workspace.NewService(filepath.Join(t.TempDir(), "projects.json"))
	t.Cleanup(s.Shutdown)
	backend := batchProject(t, s, workspace.LocalServiceDefinition{ID: "api", Name: "API", Command: append(serviceCommand(t, "--port"), strconv.Itoa(port)), URL: fmt.Sprintf("http://127.0.0.1:%d", port)})
	frontend := batchProject(t, s, workspace.LocalServiceDefinition{ID: "web", Name: "Web", Command: serviceCommand(t, "client"), EnvFrom: map[string]workspace.ServiceEndpoint{"BASE_URL": {Project: backend, Service: "api"}, "API_PORT": {Project: backend, Service: "api", Field: "port"}}})
	config, err := s.LocalServices(frontend)
	if err != nil {
		t.Fatal(err)
	}
	before := readSyncFile(t, filepath.Join(frontend, ".repsync.json"))
	dry, err := s.StartProjects([]string{frontend}, true)
	if err != nil || len(dry.Services) != 2 || dry.Services[0].Path != backend || dry.Services[1].State.Definition.Env["BASE_URL"] == "" {
		t.Fatalf("dependency preflight: %+v %v", dry, err)
	}
	if _, err := os.Stat(filepath.Join(frontend, "received-url")); !os.IsNotExist(err) {
		t.Fatal("dry run executed consumer")
	}
	state, err := s.StartLocalService(frontend, "web", config.Version)
	if err != nil {
		t.Fatal(err)
	}
	deadline := time.Now().Add(4 * time.Second)
	for {
		if body, err := os.ReadFile(filepath.Join(frontend, "received-url")); err == nil {
			api, err := s.LocalServices(backend)
			if err != nil {
				t.Fatal(err)
			}
			actual := api.Services[0].Definition
			if actual.Port == port || string(body) != actual.URL || state.Definition.Env["API_PORT"] != strconv.Itoa(actual.Port) {
				t.Fatalf("consumer used wrong endpoint: %+v %+v %s", state, actual, body)
			}
			break
		}
		if time.Now().After(deadline) {
			output, _ := s.LocalServiceLog(frontend, "web", state.RunID)
			t.Fatalf("consumer did not connect to ready backend: %+v", output)
		}
		time.Sleep(20 * time.Millisecond)
	}
	if readSyncFile(t, filepath.Join(frontend, ".repsync.json")) != before {
		t.Fatal("injection rewrote project config")
	}
	repeated, err := s.StartProjects([]string{frontend}, false)
	if err != nil || repeated.Services[1].State.RunID != state.RunID {
		t.Fatalf("repeated start replaced runs: %+v %v", repeated, err)
	}
	api, err := s.LocalServices(backend)
	if err != nil {
		t.Fatal(err)
	}
	if err := s.StopLocalService(backend, "api", api.Services[0].RunID); err != nil {
		t.Fatal(err)
	}
	occupied, err := net.Listen("tcp", fmt.Sprintf("127.0.0.1:%d", api.Services[0].Definition.Port))
	if err != nil {
		t.Fatal(err)
	}
	defer occupied.Close()
	if _, err := s.StartProjects([]string{backend}, false); err != nil {
		t.Fatal(err)
	}
	diagnosis, err := s.DiagnoseProject(frontend)
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, item := range diagnosis.Issues {
		if item.Kind == "dependency_address_changed" {
			found = true
		}
	}
	if diagnosis.Ready || !found {
		t.Fatalf("stale linked environment not reported: %+v", diagnosis)
	}
	if _, err := s.StartProjects([]string{frontend}, false); err == nil {
		t.Fatal("reused a consumer with stale endpoint")
	}
}

func TestServiceLinksRejectCycleAndMissingDependency(t *testing.T) {
	t.Parallel()
	s := workspace.NewService(filepath.Join(t.TempDir(), "projects.json"))
	t.Cleanup(s.Shutdown)
	path := batchProject(t, s, workspace.LocalServiceDefinition{ID: "a", Name: "A", Command: []string{"/bin/sleep", "30"}})
	writeLocalServices(t, path, []workspace.LocalServiceDefinition{
		{ID: "a", Name: "A", Command: []string{"/bin/sleep", "30"}, EnvFrom: map[string]workspace.ServiceEndpoint{"API": {Service: "b"}}},
		{ID: "b", Name: "B", Command: []string{"/bin/sleep", "30"}, EnvFrom: map[string]workspace.ServiceEndpoint{"API": {Service: "a"}}},
	})
	if _, err := s.StartProjects([]string{path}, false); err == nil || !strings.Contains(err.Error(), "circular") {
		t.Fatalf("cycle accepted: %v", err)
	}
	writeLocalServices(t, path, []workspace.LocalServiceDefinition{{ID: "a", Name: "A", Command: []string{"/bin/sleep", "30"}, EnvFrom: map[string]workspace.ServiceEndpoint{"API": {Service: "missing"}}}})
	if _, err := s.StartProjects([]string{path}, false); err == nil {
		t.Fatal("missing dependency accepted")
	}
	overview, err := s.LocalServices(path)
	if err != nil || overview.Services[0].RunID != "" {
		t.Fatal("bad dependency started a process")
	}
}

func TestServiceLinksFailedDependencyDoesNotStartConsumer(t *testing.T) {
	t.Parallel()
	s := workspace.NewService(filepath.Join(t.TempDir(), "projects.json"))
	t.Cleanup(s.Shutdown)
	path := batchProject(t, s, workspace.LocalServiceDefinition{ID: "api", Name: "API", Command: []string{"/bin/sleep", "30"}})
	marker := filepath.Join(path, "consumer-ran")
	writeLocalServices(t, path, []workspace.LocalServiceDefinition{
		{ID: "web", Name: "Web", Command: []string{"/bin/sh", "-c", "touch '" + marker + "'"}, EnvFrom: map[string]workspace.ServiceEndpoint{"API": {Service: "api"}}},
		{ID: "api", Name: "API", Command: []string{"/bin/sh", "-c", "exit 7"}, AutoPort: true, URL: "http://127.0.0.1:{port}", Env: map[string]string{"PORT": "{port}"}},
	})
	batch, err := s.StartProjects([]string{path}, false)
	if err == nil || len(batch.Services) != 2 || batch.Services[1].Status != "failed" {
		t.Fatalf("dependency failure hidden: %+v %v", batch, err)
	}
	if _, err := os.Stat(marker); !os.IsNotExist(err) {
		t.Fatal("consumer ran before dependency readiness")
	}
}
