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

func TestControlProjectMemoryProposal(t *testing.T) {
	root := t.TempDir()
	project := t.TempDir()
	service := workspace.NewService(filepath.Join(root, "projects.json"))
	p, err := service.Add(project)
	if err != nil {
		t.Fatal(err)
	}
	socket := shortSocket(t)
	server, err := control.Listen(socket, service)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { service.Shutdown(); server.Close() })
	input := filepath.Join(root, "proposal.json")
	if err := os.WriteFile(input, []byte(`{"statement":"Legacy export retired","source":"release #3","keywords":["legacy export"]}`), 0600); err != nil {
		t.Fatal(err)
	}
	response, code := cli(t, socket, "project-memory-propose", p.Path, input)
	var proposal workspace.ProjectMemory
	if err := json.Unmarshal(response.Data, &proposal); code != 0 || err != nil || proposal.Status != "pending" {
		t.Fatalf("CLI did not create a pending proposal: %+v %v", response, err)
	}
	response, code = cli(t, socket, "project-memory", p.Path)
	if code != 0 || string(response.Data) != "[]" {
		t.Fatalf("CLI exposed an unconfirmed proposal: %+v", response)
	}
	if _, err := service.SetProjectMemoryState(p.Path, proposal.ID, proposal.Version, "confirm"); err != nil {
		t.Fatal(err)
	}
	response, code = cli(t, socket, "project-memory", p.Path)
	var confirmed []workspace.ProjectMemory
	if err := json.Unmarshal(response.Data, &confirmed); code != 0 || err != nil || len(confirmed) != 1 ||
		confirmed[0].ID != proposal.ID || confirmed[0].Source != "release #3" {
		t.Fatalf("CLI did not read the confirmed record and source: %+v %v", response, err)
	}
}
