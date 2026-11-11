//go:build integration

package workspace_test

import (
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"repsync/internal/workspace"
)

func TestServicePortAutomaticInputs(t *testing.T) {
	for _, name := range []string{"argument", "equals", "environment", "simple shell"} {
		t.Run(name, func(t *testing.T) {
			external, err := net.Listen("tcp", "127.0.0.1:0")
			if err != nil {
				t.Fatal(err)
			}
			defer external.Close()
			port := external.Addr().(*net.TCPAddr).Port
			d := workspace.LocalServiceDefinition{ID: "web", Name: "Web", URL: fmt.Sprintf("http://127.0.0.1:%d/page?original=%d", port, port)}
			switch name {
			case "argument":
				d.Command = append(serviceCommand(t, "--port"), strconv.Itoa(port))
			case "equals":
				d.Command = serviceCommand(t, "--port="+strconv.Itoa(port))
			case "environment":
				d.Command = serviceCommand(t, "env")
				d.Env = map[string]string{"PORT": strconv.Itoa(port)}
			case "simple shell":
				argv := append(serviceCommand(t, "--port"), strconv.Itoa(port))
				argv[1] = "-test.run=TestLocalServiceProcess"
				d.Command = []string{"/bin/sh", "-c", strings.Join(argv, " ")}
			}
			s, path, config := localServiceFixture(t, d)
			prepared := config.Services[0].Definition
			if !prepared.AutoPort || prepared.Port != port {
				t.Fatalf("not automatically prepared: %+v", prepared)
			}
			before := readSyncFile(t, filepath.Join(path, ".repsync.json"))
			started, err := s.StartLocalService(path, "web", config.Version)
			if err != nil {
				t.Fatal(err)
			}
			defer func() {
				if t.Failed() {
					output, err := s.LocalServiceLog(path, "web", started.RunID)
					t.Logf("service output: %+v %v", output, err)
				}
			}()
			actual := started.Definition.Port
			if actual == port || actual == 0 || started.Definition.URL != fmt.Sprintf("http://127.0.0.1:%d/page?original=%d", actual, port) {
				t.Fatalf("incorrect assigned endpoint: %+v", started)
			}
			state := waitLocalService(t, s, path, func(state workspace.LocalServiceState) bool { return state.PortState == "listening" })
			if state.ConfigChanged {
				t.Fatal("allocation changed project configuration")
			}
			response, err := http.Get(state.Definition.URL)
			if err != nil {
				t.Fatal(err)
			}
			body, _ := io.ReadAll(response.Body)
			response.Body.Close()
			if string(body) != "owned-service" {
				t.Fatal("wrong service answered")
			}
			if readSyncFile(t, filepath.Join(path, ".repsync.json")) != before {
				t.Fatal("rewrote registered project")
			}
			// Repeated CLI-style starts reuse the exact same run.
			batch, err := s.StartProjects([]string{path}, false)
			if err != nil || batch.Services[0].Status != "already_running" || batch.Services[0].State.RunID != started.RunID {
				t.Fatalf("reuse: %+v %v", batch, err)
			}
			connection, err := net.Dial("tcp", external.Addr().String())
			if err != nil {
				t.Fatal("external listener was changed")
			}
			connection.Close()
		})
	}
}

func TestServicePortFixedAndOpaqueCommands(t *testing.T) {
	t.Parallel()
	for _, test := range []struct {
		name    string
		command []string
		fixed   bool
	}{
		{"explicit fixed", []string{"/bin/echo", "--port", "8100"}, true},
		{"unrelated argument", []string{"/bin/echo", "8100"}, false},
		{"compound shell", []string{"/bin/sh", "-c", "echo setup && npm run dev -- --port 8100"}, false},
	} {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			s, path, config := localServiceFixture(t, workspace.LocalServiceDefinition{ID: "web", Name: "Web", Command: test.command, Port: 8100, FixedPort: test.fixed})
			if config.Services[0].Definition.AutoPort {
				t.Fatal("adapted a fixed or opaque command")
			}
			// Reading the normalized definition never edits the stored command.
			text, err := s.ProjectConfiguration(path)
			if err != nil || strings.Join(text.Services[0].Command, " ") != strings.Join(test.command, " ") {
				t.Fatalf("source definition changed: %+v %v", text, err)
			}
		})
	}
}

func TestServicePortAutomaticViteProjects(t *testing.T) {
	if _, err := exec.LookPath("npm"); err != nil {
		t.Skip("npm not installed")
	}
	vite, err := filepath.Abs("../../frontend/node_modules/vite/bin/vite.js")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(vite); err != nil {
		t.Skip("frontend dependencies not installed")
	}
	external, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer external.Close()
	port := external.Addr().(*net.TCPAddr).Port
	s := workspace.NewService(filepath.Join(t.TempDir(), "projects.json"))
	t.Cleanup(s.Shutdown)
	paths := []string{}
	originals := map[string]string{}
	for index := range 2 {
		root := t.TempDir()
		frontend := filepath.Join(root, "frontend")
		if index == 1 {
			frontend = root
		}
		if err := os.MkdirAll(filepath.Join(frontend, "node_modules", ".bin"), 0700); err != nil {
			t.Fatal(err)
		}
		if err := os.Symlink(vite, filepath.Join(frontend, "node_modules", ".bin", "vite")); err != nil {
			t.Fatal(err)
		}
		writeSyncFile(t, filepath.Join(frontend, "package.json"), fmt.Sprintf(`{"scripts":{"dev":"vite --host 127.0.0.1 --port %d --strictPort"}}`, port))
		writeSyncFile(t, filepath.Join(frontend, "index.html"), "<h1>automatic-project-port</h1>")
		// Cover both an existing config without switches and a project with only package.json.
		if index == 0 {
			writeSyncFile(t, filepath.Join(root, ".repsync.json"), fmt.Sprintf(`{"services":[{"id":"web","name":"Web","cwd":"frontend","command":["npm","run","dev"],"url":"http://127.0.0.1:%d"}]}`, port))
		}
		project, err := s.Add(root)
		if err != nil {
			t.Fatal(err)
		}
		paths = append(paths, project.Path)
		if index == 0 {
			originals[project.Path] = readSyncFile(t, filepath.Join(project.Path, ".repsync.json"))
		}
	}
	batch, err := s.StartProjects(paths, false)
	if err != nil {
		t.Fatalf("automatic multi-project start: %+v %v", batch, err)
	}
	ports := map[int]bool{port: true}
	for _, entry := range batch.Services {
		state := waitLocalService(t, s, entry.Path, func(state workspace.LocalServiceState) bool { return state.PortState == "listening" })
		if ports[state.Definition.Port] || state.ConfigChanged {
			t.Fatalf("port collision or false config change: %+v", state)
		}
		ports[state.Definition.Port] = true
		endpoint := state.Definition.URL
		if endpoint == "" {
			endpoint = fmt.Sprintf("http://127.0.0.1:%d", state.Definition.Port)
		}
		response, err := http.Get(endpoint)
		if err != nil {
			t.Fatal(err)
		}
		body, _ := io.ReadAll(response.Body)
		response.Body.Close()
		if !strings.Contains(string(body), "automatic-project-port") {
			t.Fatal("wrong page")
		}
		if original, exists := originals[entry.Path]; exists {
			if readSyncFile(t, filepath.Join(entry.Path, ".repsync.json")) != original {
				t.Fatal("modified project config")
			}
		} else if _, err := os.Stat(filepath.Join(entry.Path, ".repsync.json")); !os.IsNotExist(err) {
			t.Fatal("created project config during startup")
		}
	}
	// Changing a hidden script invalidates the service configuration version.
	before, err := s.LocalServices(paths[0])
	if err != nil {
		t.Fatal(err)
	}
	writeSyncFile(t, filepath.Join(paths[0], "frontend", "package.json"), `{"scripts":{"dev":"vite --host 127.0.0.1 --port 9000 --strictPort"}}`)
	after, err := s.LocalServices(paths[0])
	if err != nil {
		t.Fatal(err)
	}
	if before.Version == after.Version {
		t.Fatal("script change did not invalidate configuration")
	}
	if !after.Services[0].ConfigChanged {
		t.Fatal("running service did not report hidden script change")
	}
}
