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

func TestControlProjectMap(t *testing.T) {
	t.Parallel()
	service := workspace.NewService(filepath.Join(t.TempDir(), "projects.json"))
	project, err := service.Add(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	socket := shortSocket(t)
	server, err := control.Listen(socket, service)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { service.Shutdown(); server.Close() })
	for name, content := range map[string]string{
		"Page.tsx":          "export function Page() {}\n",
		".repsync-map.json": `{"version":1,"modules":[{"id":"feature","name":"User feature","implementations":[{"name":"Page","source":{"path":"Page.tsx"}}]}]}`,
	} {
		if err := os.WriteFile(filepath.Join(project.Path, name), []byte(content), 0600); err != nil {
			t.Fatal(err)
		}
	}
	response, code := cli(t, socket, "project-map", project.Path)
	var view workspace.ProjectMap
	if err := json.Unmarshal(response.Data, &view); code != 0 || err != nil || len(view.Modules) != 1 || view.Modules[0].Implementations[0].Source.Line != 1 {
		t.Fatalf("CLI map: %+v %v", response, err)
	}
}
