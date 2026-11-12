//go:build integration

package control_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"repsync/internal/control"
	"repsync/internal/workspace"
)

func TestControlProjectChecksUseExistingRunner(t *testing.T) {
	root := t.TempDir()
	config := `{"pipelines":[{"id":"custom","name":"Custom check","kind":"ci","steps":[["/bin/sh","-c","echo project-check-output; exit 4"]]}]}`
	if err := os.WriteFile(filepath.Join(root, ".repsync.json"), []byte(config), 0600); err != nil {
		t.Fatal(err)
	}
	s := workspace.NewService(filepath.Join(t.TempDir(), "projects.json"))
	p, err := s.Add(root)
	if err != nil {
		t.Fatal(err)
	}
	socket := shortSocket(t)
	server, err := control.Listen(socket, s)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { s.Shutdown(); server.Close() })
	result, code := cli(t, socket, "checks", p.Path)
	var overview workspace.ProjectCheckOverview
	if err := json.Unmarshal(result.Data, &overview); err != nil || code != 0 || len(overview.Checks) != 1 {
		t.Fatalf("discovery: %+v %v", result, err)
	}
	result, code = cli(t, socket, "check", "--wait", p.Path, overview.Checks[0].Pipeline.ID, overview.Version)
	var output workspace.PipelineOutput
	if err := json.Unmarshal(result.Data, &output); err != nil || code != 1 || output.Run.Status != "failure" || output.Run.ExitCode != 4 {
		t.Fatalf("actual check result lost: %+v %v", result, err)
	}
	result, code = cli(t, socket, "check", p.Path, overview.Checks[0].Pipeline.ID, "stale")
	if code != 1 || result.OK {
		t.Fatalf("stale version accepted: %+v", result)
	}
}

func TestControlDiscoversPipelinesAndWorkflowFiles(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, ".github", "workflows"), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "package.json"), []byte(`{"scripts":{"build":"echo build","publish:staging":"touch must-not-run"}}`), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, ".github", "workflows", "existing.yml"), []byte("name: Existing workflow\non: workflow_dispatch\njobs:\n  verify:\n    runs-on: ubuntu-latest\n    steps:\n      - run: echo actual-step\n"), 0600); err != nil {
		t.Fatal(err)
	}
	s := workspace.NewService(filepath.Join(t.TempDir(), "projects.json"))
	p, err := s.Add(root)
	if err != nil {
		t.Fatal(err)
	}
	socket := shortSocket(t)
	server, err := control.Listen(socket, s)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { s.Shutdown(); server.Close() })
	result, code := cli(t, socket, "pipelines", p.Path)
	var pipelines workspace.PipelineConfig
	if err := json.Unmarshal(result.Data, &pipelines); err != nil || code != 0 || len(pipelines.Pipelines) != 2 || pipelines.Pipelines[1].Kind != "cd" {
		t.Fatalf("detected pipelines: %+v %v", result, err)
	}
	result, code = cli(t, socket, "workflows", p.Path)
	var workflows workspace.WorkflowOverview
	if err := json.Unmarshal(result.Data, &workflows); err != nil || code != 0 || len(workflows.Workflows) != 1 || workflows.Workflows[0].Jobs[0].Steps[0].Run != "echo actual-step" {
		t.Fatalf("workflow definitions: %+v %v", result, err)
	}
	if _, err := os.Stat(filepath.Join(root, "must-not-run")); !os.IsNotExist(err) {
		t.Fatal("discovery executed delivery")
	}
}
