//go:build integration

package workspace_test

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"repsync/internal/workspace"
)

func TestProjectChecksDiscoverSourcesAndRunExistingCommand(t *testing.T) {
	root := t.TempDir()
	write := func(name, body string) {
		t.Helper()
		p := filepath.Join(root, name)
		if err := os.MkdirAll(filepath.Dir(p), 0700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(body), 0600); err != nil {
			t.Fatal(err)
		}
	}
	write("AGENTS.md", "Use the existing checks; do not invent rules.\n")
	write("eslint.config.mjs", "throw new Error('discovery must not evaluate JS')\n")
	write(".repsync.json", `{"pipelines":[{"id":"quality","name":"Existing quality gate","kind":"ci","steps":[["/bin/sh","-c","printf checked > result; printf check-log"]]}],"policies":[{"id":"project-owned-rule","level":"error"}]}`)
	write("packages/site/package.json", `{"scripts":{"lint":"eslint .","lint:fix":"eslint . --fix","test:watch":"vitest --watch","check":"tsc --noEmit","deploy":"publish","verify":"node verify.mjs"}}`)
	write("packages/site/tsconfig.json", `{"compilerOptions":{"strict":true}}`)
	write("pyproject.toml", "[tool.ruff.lint]\nselect = ['E']\n[tool.pytest.ini_options]\ntestpaths=['tests']\n")
	write("Makefile", "lint-native:\n\t@echo lint\nlint-fix:\n\t@echo fix\nrelease:\n\t@echo release\n")
	write("node_modules/bad/package.json", `{"scripts":{"check":"touch should-not-run"}}`)
	s := workspace.NewService(filepath.Join(t.TempDir(), "projects.json"))
	t.Cleanup(s.Shutdown)
	p, err := s.Add(root)
	if err != nil {
		t.Fatal(err)
	}
	root = p.Path
	overview, err := s.ProjectChecks(root)
	if err != nil {
		t.Fatal(err)
	}
	if len(overview.Configurations) != 4 || !slices.Contains(overview.Guidance, "AGENTS.md") {
		t.Fatalf("source discovery: %+v", overview)
	}
	if len(overview.Checks) != 5 {
		t.Fatalf("unexpected commands: %+v", overview.Checks)
	}
	for _, c := range overview.Checks {
		if strings.Contains(c.Source, "node_modules") || strings.Contains(c.Pipeline.Name, "fix") || strings.Contains(c.Pipeline.Name, "watch") {
			t.Fatalf("unexpected command: %+v", c)
		}
	}
	var selected workspace.ProjectCheck
	for _, c := range overview.Checks {
		if c.Source == ".repsync.json" {
			selected = c
		}
	}
	if selected.Pipeline.ID == "" {
		t.Fatal("explicit command missing")
	}
	if _, err := os.Stat(filepath.Join(root, "result")); !os.IsNotExist(err) {
		t.Fatal("discovery executed a command")
	}
	run, err := s.StartProjectCheck(root, selected.Pipeline.ID, overview.Version)
	if err != nil {
		t.Fatal(err)
	}
	output := waitPipeline(t, s, root, run.ID)
	if output.Run.Status != "success" || !strings.Contains(output.Log, "check-log") {
		t.Fatalf("existing check failed: %+v", output)
	}
	if _, err := os.Stat(filepath.Join(root, "result")); err != nil {
		t.Fatal(err)
	}
	write("packages/site/tsconfig.json", `{"compilerOptions":{"strict":false}}`)
	if _, err := s.StartProjectCheck(root, selected.Pipeline.ID, overview.Version); err == nil {
		t.Fatal("stale config accepted")
	}
}

func TestProjectChecksReportsMissingCommandsAndInvalidSources(t *testing.T) {
	root := t.TempDir()
	s := workspace.NewService(filepath.Join(t.TempDir(), "projects.json"))
	t.Cleanup(s.Shutdown)
	p, err := s.Add(root)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "eslint.config.js"), []byte("export default []"), 0600); err != nil {
		t.Fatal(err)
	}
	overview, err := s.ProjectChecks(p.Path)
	if err != nil || len(overview.Configurations) != 1 || len(overview.Checks) != 0 {
		t.Fatalf("configuration invented a runnable command: %+v %v", overview, err)
	}
	if err := os.WriteFile(filepath.Join(root, "package.json"), []byte("not json"), 0600); err != nil {
		t.Fatal(err)
	}
	overview, err = s.ProjectChecks(p.Path)
	if err != nil || len(overview.Warnings) == 0 || len(overview.Checks) != 0 {
		t.Fatalf("broken source was hidden: %+v %v", overview, err)
	}
}

func TestProjectChecksDiscoverMoonTaskTypes(t *testing.T) {
	root := t.TempDir()
	bin := filepath.Join(root, "bin")
	if err := os.MkdirAll(filepath.Join(root, ".moon"), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(bin, 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, ".moon", "workspace.yml"), []byte("projects: {}\n"), 0600); err != nil {
		t.Fatal(err)
	}
	graph := `{"tasks":{"orchard":{"quality-audit":{"type":"test","options":{}},"quality-mutate":{"type":"test","command":"eslint","args":["--fix","."],"options":{}},"lint-fix":{"type":"test","options":{}},"private-check":{"type":"test","options":{"internal":true}},"serve":{"type":"test","options":{"persistent":true}},"compile":{"type":"build","options":{}}}}}`
	if err := os.WriteFile(filepath.Join(root, "graph.json"), []byte(graph), 0600); err != nil {
		t.Fatal(err)
	}
	script := "#!/bin/sh\n[ \"$1 $2 $3 $4\" = \"query tasks --type test\" ] || exit 2\ncat graph.json\n"
	if err := os.WriteFile(filepath.Join(bin, "moon"), []byte(script), 0700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))
	s := workspace.NewService(filepath.Join(t.TempDir(), "projects.json"))
	t.Cleanup(s.Shutdown)
	p, err := s.Add(root)
	if err != nil {
		t.Fatal(err)
	}
	overview, err := s.ProjectChecks(p.Path)
	if err != nil || len(overview.Warnings) != 0 || len(overview.Checks) != 1 || overview.Checks[0].Pipeline.Name != "orchard:quality-audit" {
		t.Fatalf("Moon task discovery: %+v %v", overview, err)
	}
	if err := os.WriteFile(filepath.Join(bin, "moon"), []byte("#!/bin/sh\nexit 3\n"), 0700); err != nil {
		t.Fatal(err)
	}
	overview, err = s.ProjectChecks(p.Path)
	if err != nil || len(overview.Warnings) == 0 || len(overview.Checks) != 0 {
		t.Fatalf("failed query was presented as known checks: %+v %v", overview, err)
	}
}
