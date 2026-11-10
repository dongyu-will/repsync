//go:build integration

package workspace_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"repsync/internal/workspace"
)

func configFixture(t *testing.T, content string) (*workspace.Service, string) {
	t.Helper()
	path := t.TempDir()
	if content != "" {
		if err := os.WriteFile(filepath.Join(path, ".repsync.json"), []byte(content), 0640); err != nil {
			t.Fatal(err)
		}
	}
	s := workspace.NewService(filepath.Join(t.TempDir(), "projects.json"))
	t.Cleanup(s.Shutdown)
	p, err := s.Add(path)
	if err != nil {
		t.Fatal(err)
	}
	return s, p.Path
}

func TestProjectConfigIndependentDiscovery(t *testing.T) {
	t.Parallel()
	for _, test := range []struct {
		name, content       string
		services, pipelines int
	}{
		{"missing", "", 1, 1}, {"services only", `{"services":[]}`, 0, 1}, {"pipelines only", `{"pipelines":[]}`, 1, 0},
		{"both disabled", `{"services":[],"pipelines":[]}`, 0, 0}, {"null discovers", `{"services":null,"pipelines":null}`, 1, 1},
	} {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			s, path := configFixture(t, test.content)
			if err := os.WriteFile(filepath.Join(path, "package.json"), []byte(`{"scripts":{"dev":"echo dev","check":"echo ci"}}`), 0600); err != nil {
				t.Fatal(err)
			}
			settings, err := s.ProjectConfiguration(path)
			if err != nil || len(settings.Services) != test.services || len(settings.Pipelines) != test.pipelines {
				t.Fatalf("discovery: %+v %v", settings, err)
			}
		})
	}
}

func TestProjectConfigSavingPreservesAndProtects(t *testing.T) {
	t.Parallel()
	s, path := configFixture(t, `{"services":[{"id":"web","name":"Original","command":["/bin/echo"],"extension":{"keep":true}}],"pipelines":[],"custom":{"keep":"value"}}`)
	settings, err := s.ProjectConfiguration(path)
	if err != nil {
		t.Fatal(err)
	}
	definitions := settings.Services
	definitions[0].Name = "Changed"
	if err := s.SaveServices(path, settings.Version, false, definitions); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(filepath.Join(path, ".repsync.json"))
	if err != nil {
		t.Fatal(err)
	}
	var file map[string]json.RawMessage
	if err := json.Unmarshal(data, &file); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(file["custom"]), "value") || string(file["pipelines"]) != "[]" || !strings.Contains(string(file["services"]), "extension") {
		t.Fatalf("lost other configuration: %s", data)
	}
	info, _ := os.Stat(filepath.Join(path, ".repsync.json"))
	if info.Mode().Perm() != 0640 {
		t.Fatalf("permissions changed: %v", info.Mode())
	}
	if err := s.SavePipelines(path, settings.Version, false, nil); err == nil {
		t.Fatal("stale editor overwrote config")
	}
	after, _ := os.ReadFile(filepath.Join(path, ".repsync.json"))
	if string(after) != string(data) {
		t.Fatal("stale save changed file")
	}
	settings, err = s.ProjectConfiguration(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := s.SaveServices(path, settings.Version, true, nil); err != nil {
		t.Fatal(err)
	}
	data, _ = os.ReadFile(filepath.Join(path, ".repsync.json"))
	if strings.Contains(string(data), `"services"`) {
		t.Fatalf("automatic mode did not remove section: %s", data)
	}
}

func TestProjectConfigConcurrentSaves(t *testing.T) {
	t.Parallel()
	s, path := configFixture(t, `{"services":[],"pipelines":[]}`)
	settings, err := s.ProjectConfiguration(path)
	if err != nil {
		t.Fatal(err)
	}
	var wg sync.WaitGroup
	results := make(chan error, 2)
	for range 2 {
		wg.Add(1)
		go func() { defer wg.Done(); results <- s.SaveServices(path, settings.Version, true, nil) }()
	}
	wg.Wait()
	close(results)
	success := 0
	for err := range results {
		if err == nil {
			success++
		}
	}
	if success != 1 {
		t.Fatalf("expected one save, got %d", success)
	}
}

func TestProjectConfigRejectsUnsafeInputs(t *testing.T) {
	t.Parallel()
	for _, test := range []struct {
		name, cwd string
		env       map[string]string
	}{
		{name: "outside", cwd: "../"}, {name: "absolute", cwd: "/tmp"}, {name: "missing", cwd: "missing"},
		{name: "invalid env name", env: map[string]string{"BAD=NAME": "value"}}, {name: "null value", env: map[string]string{"TEST": "a\x00b"}},
		{name: "runner environment", env: map[string]string{"CI": "false"}}, {name: "symlink escape", cwd: "outside"},
	} {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			s, path := configFixture(t, `{"services":[],"pipelines":[]}`)
			if err := os.Symlink(t.TempDir(), filepath.Join(path, "outside")); err != nil {
				t.Fatal(err)
			}
			settings, err := s.ProjectConfiguration(path)
			if err != nil {
				t.Fatal(err)
			}
			if err := s.SaveServices(path, settings.Version, false, []workspace.LocalServiceDefinition{{ID: "web", Name: "Web", Command: []string{"/bin/echo"}, Cwd: test.cwd, Env: test.env}}); err == nil {
				t.Fatal("saved invalid service")
			}
			if err := s.SavePipelines(path, settings.Version, false, []workspace.Pipeline{{ID: "ci", Name: "CI", Kind: "ci", Steps: [][]string{{"/bin/echo"}}, Cwd: test.cwd, Env: test.env}}); err == nil {
				t.Fatal("saved invalid pipeline")
			}
			data, _ := os.ReadFile(filepath.Join(path, ".repsync.json"))
			if string(data) != `{"services":[],"pipelines":[]}` {
				t.Fatalf("invalid save changed file: %s", data)
			}
		})
	}
	s, path := configFixture(t, "")
	outside := filepath.Join(t.TempDir(), "config.json")
	if err := os.WriteFile(outside, []byte(`{"services":[]}`), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, filepath.Join(path, ".repsync.json")); err != nil {
		t.Fatal(err)
	}
	if _, err := s.ProjectConfiguration(path); err == nil {
		t.Fatal("editor accepted symlink config")
	}
	if err := s.SaveServices(path, "anything", false, nil); err == nil {
		t.Fatal("overwrote symlink config")
	}
}

func TestProjectConfigExecutesDirectoryEnvironmentAndPath(t *testing.T) {
	t.Parallel()
	s, path := configFixture(t, `{"services":[],"pipelines":[]}`)
	directory := filepath.Join(path, "nested app")
	tools := filepath.Join(directory, "tools")
	if err := os.MkdirAll(tools, 0700); err != nil {
		t.Fatal(err)
	}
	executable := filepath.Join(tools, "project-tool")
	if err := os.WriteFile(executable, []byte("#!/bin/sh\nprintf 'directory=%s\\nvalue=%s\\nCI=%s\\n' \"$PWD\" \"$REPSYNC_TEST_VALUE\" \"$CI\"\nif [ \"$1\" = service ]; then exec /bin/sleep 30; fi\n"), 0700); err != nil {
		t.Fatal(err)
	}
	settings, err := s.ProjectConfiguration(path)
	if err != nil {
		t.Fatal(err)
	}
	env := map[string]string{"PATH": "tools", "REPSYNC_TEST_VALUE": "space = and\nline"}
	pipeline := workspace.Pipeline{ID: "ci", Name: "CI", Kind: "ci", Cwd: "nested app", Env: env, Steps: [][]string{{"project-tool"}}}
	if err := s.SavePipelines(path, settings.Version, false, []workspace.Pipeline{pipeline}); err != nil {
		t.Fatal(err)
	}
	config, err := s.Pipelines(path)
	if err != nil {
		t.Fatal(err)
	}
	run, err := s.StartPipeline(path, "ci", config.Version)
	if err != nil {
		t.Fatal(err)
	}
	output := waitPipeline(t, s, path, run.ID)
	if output.Run.Status != "success" || !strings.Contains(output.Log, "directory="+directory) || !strings.Contains(output.Log, "value=space = and\nline") || !strings.Contains(output.Log, "CI=true") {
		t.Fatalf("pipeline execution: %+v", output)
	}
	settings, err = s.ProjectConfiguration(path)
	if err != nil {
		t.Fatal(err)
	}
	pipeline.Steps = [][]string{{"/usr/bin/env"}}
	if err := s.SavePipelines(path, settings.Version, false, []workspace.Pipeline{pipeline}); err != nil {
		t.Fatal(err)
	}
	config, err = s.Pipelines(path)
	if err != nil {
		t.Fatal(err)
	}
	run, err = s.StartPipeline(path, "ci", config.Version)
	if err != nil {
		t.Fatal(err)
	}
	if output := waitPipeline(t, s, path, run.ID); !strings.Contains(output.Log, "PWD="+directory+"\n") {
		t.Fatalf("PWD differs from working directory: %+v", output)
	}
	settings, err = s.ProjectConfiguration(path)
	if err != nil {
		t.Fatal(err)
	}
	definition := workspace.LocalServiceDefinition{ID: "web", Name: "Web", Cwd: "nested app", Env: env, Command: []string{"project-tool", "service"}}
	if err := s.SaveServices(path, settings.Version, false, []workspace.LocalServiceDefinition{definition}); err != nil {
		t.Fatal(err)
	}
	overview, err := s.LocalServices(path)
	if err != nil {
		t.Fatal(err)
	}
	state, err := s.StartLocalService(path, "web", overview.Version)
	if err != nil {
		t.Fatal(err)
	}
	deadline := time.Now().Add(5 * time.Second)
	for {
		output, err := s.LocalServiceLog(path, "web", state.RunID)
		if err != nil {
			t.Fatal(err)
		}
		if strings.Contains(output.Log, "value=space = and\nline") {
			if !strings.Contains(output.Log, "directory="+directory) || !strings.Contains(output.Log, "CI=\n") {
				t.Fatalf("service execution: %+v", output)
			}
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("service output not ready")
		}
		time.Sleep(10 * time.Millisecond)
	}
	settings, err = s.ProjectConfiguration(path)
	if err != nil {
		t.Fatal(err)
	}
	definition.Env = map[string]string{"REPSYNC_TEST_VALUE": "new"}
	definition.Cwd = ""
	if err := s.SaveServices(path, settings.Version, false, []workspace.LocalServiceDefinition{definition}); err != nil {
		t.Fatal(err)
	}
	overview, err = s.LocalServices(path)
	if err != nil || len(overview.Services) != 1 || !overview.Services[0].ConfigChanged || overview.Services[0].Definition.Cwd != "nested app" {
		t.Fatalf("running config not retained: %+v %v", overview, err)
	}
	if err := s.StopLocalService(path, "web", state.RunID); err != nil {
		t.Fatal(err)
	}
}

func TestProjectConfigCanRepairMissingWorkingDirectory(t *testing.T) {
	t.Parallel()
	s, path := configFixture(t, `{"services":[{"id":"web","name":"Web","command":["/bin/echo"],"cwd":"removed"}],"pipelines":[{"id":"ci","name":"CI","kind":"ci","steps":[["/bin/echo"]],"cwd":"removed"}]}`)
	settings, err := s.ProjectConfiguration(path)
	if err != nil {
		t.Fatalf("editor locked by missing directory: %v", err)
	}
	if len(settings.Services) != 1 || settings.Services[0].Cwd != "removed" || len(settings.Pipelines) != 1 {
		t.Fatalf("lost editable definitions: %+v", settings)
	}
	if _, err := s.Pipelines(path); err == nil {
		t.Fatal("runtime accepted missing directory")
	}
	if err := s.SaveServices(path, settings.Version, false, nil); err != nil {
		t.Fatalf("could not remove broken service independently: %v", err)
	}
	settings, err = s.ProjectConfiguration(path)
	if err != nil {
		t.Fatal(err)
	}
	settings.Pipelines[0].Cwd = ""
	if err := s.SavePipelines(path, settings.Version, false, settings.Pipelines); err != nil {
		t.Fatalf("could not repair pipeline: %v", err)
	}
	if _, err := s.Pipelines(path); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(path, "package.json"), []byte("invalid JSON"), 0600); err != nil {
		t.Fatal(err)
	}
	settings, err = s.ProjectConfiguration(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := s.SaveServices(path, settings.Version, true, nil); err != nil {
		t.Fatal(err)
	}
	settings, err = s.ProjectConfiguration(path)
	if err != nil || len(settings.Warnings) != 1 {
		t.Fatalf("failed discovery locked editor or hid warning: %+v %v", settings, err)
	}
	if err := s.SaveServices(path, settings.Version, false, nil); err != nil {
		t.Fatalf("could not disable failed discovery: %v", err)
	}
}
