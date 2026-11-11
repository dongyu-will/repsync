//go:build integration

package control_test

import (
	"path/filepath"
	"repsync/internal/control"
	"repsync/internal/workspace"
	"strings"
	"testing"
)

func TestControlAICommandsGuardProjectScope(t *testing.T) {
	t.Parallel()
	socket := shortSocket(t)
	service := workspace.NewService(filepath.Join(t.TempDir(), "projects.json"))
	server, err := control.Listen(socket, service)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { service.Shutdown(); server.Close() })
	for _, action := range []string{"ai-resources", "ai-usage"} {
		t.Run(action, func(t *testing.T) {
			response, code := cli(t, socket, action, t.TempDir())
			if code != 1 || response.OK || !strings.Contains(response.Error, "register") {
				t.Fatalf("project scope guard bypassed: %d %+v", code, response)
			}
			_, code = cli(t, socket, action, "/one", "/two")
			if code != 2 {
				t.Fatal("extra arguments accepted")
			}
		})
	}
}
