//go:build integration

package workspace_test

import (
	"math"
	"net"
	"path/filepath"
	"testing"
	"time"

	"repsync/internal/workspace"
)

func TestServiceUsageIncludesChildrenAndClearsAfterStop(t *testing.T) {
	t.Parallel()
	d := workspace.LocalServiceDefinition{
		ID: "worker", Name: "Worker with child",
		Command: []string{"/bin/sh", "-c", "sleep 60 & wait"},
	}
	s, path, config := localServiceFixture(t, d)
	if _, err := s.StartLocalService(path, d.ID, config.Version); err != nil {
		t.Fatal(err)
	}
	state := waitLocalService(t, s, path, func(state workspace.LocalServiceState) bool {
		return state.Usage != nil && state.Usage.ProcessCount >= 2
	})
	usage := state.Usage
	if usage.Scope != "process-group" || usage.MemoryBytes <= 0 || usage.CPUPercent < 0 || math.IsNaN(usage.CPUPercent) {
		t.Fatalf("invalid process group usage: %+v", usage)
	}
	if _, err := time.Parse(time.RFC3339, usage.SampledAt); err != nil {
		t.Fatal(err)
	}
	if err := s.StopLocalService(path, d.ID, state.RunID); err != nil {
		t.Fatal(err)
	}
	stopped, err := s.LocalServices(path)
	if err != nil || stopped.Services[0].Usage != nil || stopped.Services[0].UsageError != "" {
		t.Fatalf("stale usage after stop: %+v %v", stopped, err)
	}
}

func TestServiceUsageObservedListenerHasHostScope(t *testing.T) {
	t.Parallel()
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()
	s := workspace.NewService(filepath.Join(t.TempDir(), "projects.json"))
	t.Cleanup(s.Shutdown)
	path := batchProject(t, s, workspace.LocalServiceDefinition{
		ID: "db", Name: "Shared database", ObserveOnly: true,
		Port: listener.Addr().(*net.TCPAddr).Port,
	})
	result, err := s.LocalServices(path)
	if err != nil {
		t.Fatal(err)
	}
	usage := result.Services[0].Usage
	if usage == nil || usage.Scope != "listener" || usage.ProcessCount != 1 || usage.MemoryBytes <= 0 {
		t.Fatalf("listener usage must exclude unrelated group members: %+v", result.Services[0])
	}
	if err := listener.Close(); err != nil {
		t.Fatal(err)
	}
	result, err = s.LocalServices(path)
	if err != nil || result.Services[0].Usage != nil {
		t.Fatalf("unreachable dependency retained usage: %+v %v", result, err)
	}
}

func TestServiceUsageUnavailablePreservesComponentStatus(t *testing.T) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()
	s := workspace.NewService(filepath.Join(t.TempDir(), "projects.json"))
	t.Cleanup(s.Shutdown)
	path := batchProject(t, s, workspace.LocalServiceDefinition{
		ID: "db", Name: "Shared database", ObserveOnly: true,
		Port: listener.Addr().(*net.TCPAddr).Port,
	})
	t.Setenv("PATH", t.TempDir())
	result, err := s.LocalServices(path)
	if err != nil || result.Error != "" || len(result.Services) != 1 {
		t.Fatalf("metrics failure hid component status: %+v %v", result, err)
	}
	state := result.Services[0]
	if state.Status != "reachable" || state.Usage != nil || state.UsageError == "" {
		t.Fatalf("unavailable usage must not appear as zero: %+v", state)
	}
}
