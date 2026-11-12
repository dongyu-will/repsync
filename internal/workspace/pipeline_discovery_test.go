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

func TestPipelinesDiscoverNestedCommandsWithoutExecuting(t *testing.T) {
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
	write("package.json", `{"packageManager":"pnpm@10","scripts":{"ci":"echo lint && echo test","release:staging":"touch must-not-publish","dev":"serve","lint:fix":"eslint --fix ."}}`)
	write("apps/site/package.json", `{"scripts":{"build":"build-site","check":"typecheck-site"}}`)
	write("Makefile", "check-native:\n\t@echo native-check\ndeploy-local:\n\t@echo publish\n")
	s := workspace.NewService(filepath.Join(t.TempDir(), "projects.json"))
	t.Cleanup(s.Shutdown)
	p, err := s.Add(root)
	if err != nil {
		t.Fatal(err)
	}
	root = p.Path
	config, err := s.Pipelines(root)
	if err != nil || len(config.Warnings) > 0 || len(config.Pipelines) != 6 {
		t.Fatalf("discovery: %+v %v", config, err)
	}
	nested := false
	cd := false
	for _, p := range config.Pipelines {
		if p.Name == "ci" && (len(p.Steps) != 1 || p.Definition != "echo lint && echo test") {
			t.Fatalf("invented stages: %+v", p)
		}
		if p.Cwd == "apps/site" {
			nested = true
			if p.Steps[0][0] != "pnpm" {
				t.Fatalf("lost inherited package manager: %+v", p)
			}
		}
		if p.Name == "release:staging" {
			cd = p.Kind == "cd"
		}
	}
	if !nested || !cd {
		t.Fatal("missing nested or delivery entry")
	}
	if _, err := os.Stat(filepath.Join(root, "must-not-publish")); !os.IsNotExist(err) {
		t.Fatal("discovery published")
	}
	before := config.Version
	write("apps/site/package.json", `{"scripts":{"build":"different-build","check":"typecheck-site"}}`)
	if _, err := s.StartPipeline(root, config.Pipelines[0].ID, before); err == nil {
		t.Fatal("stale detected declaration accepted")
	}
	write(".repsync.json", `{"pipelines":[]}`)
	explicit, err := s.Pipelines(root)
	if err != nil || len(explicit.Pipelines) != 0 || explicit.Source != ".repsync.json" {
		t.Fatalf("explicit override changed: %+v %v", explicit, err)
	}
}

func TestPipelineWorkflowsReadActualDeclarationsWithoutGitHub(t *testing.T) {
	root := t.TempDir()
	dir := filepath.Join(root, ".github", "workflows")
	if err := os.MkdirAll(dir, 0700); err != nil {
		t.Fatal(err)
	}
	body := `name: Existing delivery
on:
  push:
    branches: [main]
  workflow_dispatch:
defaults:
  run:
    working-directory: apps/site
jobs:
  quality:
    runs-on: ubuntu-latest
    strategy:
      matrix:
        node: [22, 24]
    steps:
      - uses: actions/checkout@v4
      - name: Existing check
        run: |
          echo first
          echo second
  publish:
    needs: quality
    if: github.ref == 'refs/heads/main'
    uses: ./.github/workflows/reusable.yml
  after:
    needs: [quality, publish]
    runs-on: ubuntu-latest
    steps:
      - run: echo done
        working-directory: tools
`
	if err := os.WriteFile(filepath.Join(dir, "delivery.yml"), []byte(body), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "broken.yaml"), []byte("jobs: ["), 0600); err != nil {
		t.Fatal(err)
	}
	s := workspace.NewService(filepath.Join(t.TempDir(), "projects.json"))
	t.Cleanup(s.Shutdown)
	p, err := s.Add(root)
	if err != nil {
		t.Fatal(err)
	}
	result, err := s.PipelineWorkflows(p.Path)
	if err != nil || len(result.Workflows) != 1 || len(result.Warnings) != 1 {
		t.Fatalf("workflow read: %+v %v", result, err)
	}
	w := result.Workflows[0]
	if w.Content != body || w.Name != "Existing delivery" || !strings.Contains(w.Triggers, "workflow_dispatch") || len(w.Jobs) != 3 {
		t.Fatalf("lost source: %+v", w)
	}
	if w.Jobs[0].ID != "quality" || !strings.Contains(w.Jobs[0].Strategy, "24") || w.Jobs[0].Steps[1].Run != "echo first\necho second\n" || w.Jobs[0].Steps[1].WorkingDirectory != "apps/site" {
		t.Fatalf("lost job or declared steps: %+v", w.Jobs[0])
	}
	if !slices.Equal(w.Jobs[1].Needs, []string{"quality"}) || w.Jobs[1].Uses != "./.github/workflows/reusable.yml" || len(w.Jobs[1].Steps) != 0 {
		t.Fatalf("expanded reusable job or lost dependency: %+v", w.Jobs[1])
	}
	if !slices.Equal(w.Jobs[2].Needs, []string{"quality", "publish"}) || w.Jobs[2].Steps[0].WorkingDirectory != "tools" {
		t.Fatalf("wrong dependencies or working directory: %+v", w.Jobs[2])
	}
}
