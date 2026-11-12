//go:build integration

package workspace_test

import (
	"encoding/json"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"testing"

	"repsync/internal/workspace"
)

// Exercise discovery and the real batch runner together, without touching a user
// project, Docker, or credentials. The fake Moon only substitutes its CLI boundary.
func TestMoonRuntimeDiscoveryAndStartup(t *testing.T) {
	root := t.TempDir()
	bin := filepath.Join(root, "bin")
	if err := os.MkdirAll(filepath.Join(root, ".moon"), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(bin, 0700); err != nil {
		t.Fatal(err)
	}
	write := func(name, body string) {
		t.Helper()
		if err := os.WriteFile(filepath.Join(root, name), []byte(body), 0700); err != nil {
			t.Fatal(err)
		}
	}
	port := availableServicePort(t)
	db, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	dbPort := db.Addr().(*net.TCPAddr).Port
	write(".moon/workspace.yml", "projects: {}\n")
	write("README.md", "# Local development\n\n```bash\nmoon run garden:setup\nmoon run garden:dev-infra\n```\n\n```sh\nmoon run gateway:dev\n```\n\nOptional `moon run exporter:dev`.\n")
	write("Makefile", fmt.Sprintf("HTTP_PORT ?= %d\ndev-server:\n\tserver --port $(HTTP_PORT)\n", port))
	write(".env", fmt.Sprintf("DATABASE_URL=postgresql://secret-user:secret-password@127.0.0.1:%d/private-db?secret=value\nREDIS_URL=redis://remote.example:6379\n", dbPort))
	graph := map[string]any{"tasks": map[string]any{
		"garden":   map[string]any{"dev-infra": map[string]any{"command": "make", "args": []string{"dev-infra"}, "options": map[string]any{"persistent": false}}},
		"gateway":  map[string]any{"dev": map[string]any{"command": "make", "args": []string{"dev-server"}, "options": map[string]any{"persistent": true}}},
		"exporter": map[string]any{"dev": map[string]any{"command": "optional-client", "options": map[string]any{"persistent": true}}},
	}}
	data, _ := json.Marshal(graph)
	write("graph.json", string(data))
	quoted := []string{}
	for _, arg := range serviceCommand(t, strconv.Itoa(port)) {
		quoted = append(quoted, "'"+strings.ReplaceAll(arg, "'", "'\"'\"'")+"'")
	}
	write("bin/moon", "#!/bin/sh\nset -eu\ncase \"$1\" in\nquery) cat graph.json;;\nrun) case \"$2\" in\ngarden:dev-infra) test ! -f fail-setup; echo ready > prepared;;\ngateway:dev) test -f prepared; exec "+strings.Join(quoted, " ")+";;\n*) exit 99;;\nesac;;\n*) exit 98;;\nesac\n")
	t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))
	s := workspace.NewService(filepath.Join(t.TempDir(), "projects.json"))
	t.Cleanup(s.Shutdown)
	project, err := s.Add(root)
	if err != nil {
		t.Fatal(err)
	}
	root = project.Path
	overview, err := s.LocalServices(root)
	if err != nil || overview.Error != "" || len(overview.Services) != 3 {
		t.Fatalf("discovery: %+v %v", overview, err)
	}
	if overview.Services[1].Definition.Port != port || !overview.Services[0].Definition.OneShot || !slices.Contains(overview.Services[1].Definition.DependsOn, fmt.Sprintf("dependency-%d", dbPort)) {
		t.Fatalf("missing metadata: %+v", overview)
	}
	encoded, _ := json.Marshal(overview)
	if strings.Contains(string(encoded), "secret-") || strings.Contains(string(encoded), "private-db") {
		t.Fatal("credentials leaked into runtime metadata")
	}
	before := overview.Version
	dry, err := s.StartProjects([]string{root}, true)
	if err != nil || len(dry.Services) != 3 {
		t.Fatalf("dry run: %+v %v", dry, err)
	}
	if _, err := os.Stat(filepath.Join(root, "prepared")); !os.IsNotExist(err) {
		t.Fatal("discovery/dry run executed setup")
	}
	batch, err := s.StartProject(root, before)
	if err != nil {
		t.Fatalf("startup: %+v %v", batch, err)
	}
	if batch.Services[0].State.Status != "exited" || batch.Services[0].State.ExitCode != 0 {
		t.Fatalf("setup not completed: %+v", batch)
	}
	refs := []workspace.ServiceRunRef{{Path: root, ServiceID: batch.Services[0].ServiceID, RunID: batch.Services[0].State.RunID}}
	if ready, err := s.ServiceReadiness(refs); err != nil || ready.Services[0].Status != "ready" {
		t.Fatalf("completed setup rejected by CLI readiness: %+v %v", ready, err)
	}
	current, err := s.LocalServices(root)
	if err != nil || current.Version != before {
		t.Fatalf("unstable version: %+v %v", current, err)
	}
	if _, err := os.Stat(filepath.Join(root, ".repsync.json")); !os.IsNotExist(err) {
		t.Fatal("automatic startup wrote config")
	}
	if _, err := s.StopProjects([]string{root}); err != nil {
		t.Fatal(err)
	}
	write("README.md", "# No runtime selection\nOptional `moon run exporter:dev`.\n")
	fallback, err := s.LocalServices(root)
	if err != nil || fallback.Error != "" || len(fallback.Services) != 2 || fallback.Services[0].Definition.ID != "gateway:dev" {
		t.Fatalf("generic server discovery without README plan: %+v %v", fallback, err)
	}
	writeLocalServices(t, root, []workspace.LocalServiceDefinition{})
	explicit, err := s.LocalServices(root)
	if err != nil || explicit.Error != "" || len(explicit.Services) != 0 || explicit.Source != ".repsync.json" {
		t.Fatalf("explicit override lost: %+v %v", explicit, err)
	}
}

func TestOneShotSetupFailureBlocksConsumers(t *testing.T) {
	s := workspace.NewService(filepath.Join(t.TempDir(), "projects.json"))
	t.Cleanup(s.Shutdown)
	root := batchProject(t, s, workspace.LocalServiceDefinition{ID: "bootstrap", Name: "Bootstrap", OneShot: true, Command: []string{"/bin/sh", "-c", "exit 7"}})
	writeLocalServices(t, root, []workspace.LocalServiceDefinition{
		{ID: "bootstrap", Name: "Bootstrap", OneShot: true, Command: []string{"/bin/sh", "-c", "exit 7"}},
		{ID: "consumer", Name: "Consumer", DependsOn: []string{"bootstrap"}, Command: []string{"/bin/sh", "-c", "touch should-not-run"}},
	})
	batch, err := s.StartProjects([]string{root}, false)
	if err == nil || len(batch.Services) != 2 || batch.Services[1].Status != "not_started" {
		t.Fatalf("failed setup started consumer: %+v %v", batch, err)
	}
	if _, err := os.Stat(filepath.Join(root, "should-not-run")); !os.IsNotExist(err) {
		t.Fatal("consumer ran")
	}
}
