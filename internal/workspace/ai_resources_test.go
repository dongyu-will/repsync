//go:build integration

package workspace

import (
	"encoding/json"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

func aiFixture(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0600); err != nil {
		t.Fatal(err)
	}
}

func TestAIResourcesInventoryPreservesSourcesAndProjectIntent(t *testing.T) {
	t.Parallel()
	home, project := t.TempDir(), t.TempDir()
	codex := filepath.Join(home, ".codex")
	goSkill := filepath.Join(codex, "skills", "golang-how-to", "SKILL.md")
	aiFixture(t, goSkill, "---\nname: golang-how-to\ndescription: Go task routing\n---\nUse only on Go tasks.\n")
	alias := filepath.Join(home, ".agents", "skills", "go-alias")
	if err := os.MkdirAll(filepath.Dir(alias), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(filepath.Dir(goSkill), alias); err != nil {
		t.Fatal(err)
	}
	for _, source := range []string{".agents/skills/first", ".claude/skills/second"} {
		aiFixture(t, filepath.Join(home, source, "SKILL.md"), "---\nname: same-name\ndescription: |\n  A shared workflow\n  with task conditions\n---\n")
	}
	aiFixture(t, filepath.Join(project, "go.mod"), "module fixture\n")
	aiFixture(t, filepath.Join(project, "AGENTS.md"), "Go work uses golang-how-to; unrelated-golang-how-to-extra is another name.\n")
	config := `{"services":[],"aiResources":[{"kind":"skill","name":"same-name","reason":"Project intent","tasks":"Only release"},{"kind":"skill","name":"missing-skill","source":"/missing/SKILL.md","reason":"Required"}]}`
	aiFixture(t, filepath.Join(project, ".repsync.json"), config)
	projects := []Project{{Name: "fixture", Path: project}}
	report := inventoryAIResources(aiPaths{home, codex}, projects, project)
	completeAIInventory(&report, projects)
	count := 0
	for _, resource := range report.Resources {
		if resource.Name == "golang-how-to" {
			count++
			if resource.Applicability != "recommended" || resource.ID != skillID(goSkill) || resource.Version == "" {
				t.Fatalf("guide or source missing: %+v", resource)
			}
		}
		if resource.Name == "same-name" && resource.Applicability == "adopted" {
			t.Fatal("ambiguous same-name skill was treated as adopted")
		}
	}
	if count != 1 {
		t.Fatalf("symlink duplicate counted %d times", count)
	}
	if !slices.ContainsFunc(report.Resources, func(resource AIResource) bool {
		return resource.Name == "missing-skill" && resource.Availability == "missing"
	}) {
		t.Fatal("declared missing skill hidden")
	}
	if !strings.Contains(strings.Join(report.Issues, " "), "同名资源") {
		t.Fatal("missing ambiguity warning")
	}
	if after, err := os.ReadFile(filepath.Join(project, ".repsync.json")); err != nil || string(after) != config {
		t.Fatal("inventory changed project configuration")
	}
}

func TestAIResourcesInvalidConfigurationAndSkillBounds(t *testing.T) {
	t.Parallel()
	home, project := t.TempDir(), t.TempDir()
	aiFixture(t, filepath.Join(home, ".codex/skills/oversized/SKILL.md"), strings.Repeat("x", 256*1024+1))
	aiFixture(t, filepath.Join(project, ".repsync.json"), `{"aiResources":[{"kind":"cli","name":"go; touch marker"}]}`)
	report := inventoryAIResources(aiPaths{home, filepath.Join(home, ".codex")}, []Project{{Path: project}}, project)
	if len(report.Issues) == 0 {
		t.Fatal("invalid declaration silently accepted")
	}
	if !slices.ContainsFunc(report.Resources, func(resource AIResource) bool {
		return resource.Name == "oversized" && resource.Availability == "error"
	}) {
		t.Fatal("oversized skill was not bounded")
	}
}

func TestAIResourcesDoctorChecksDeclarationWithoutExecuting(t *testing.T) {
	t.Parallel()
	service := NewService(filepath.Join(t.TempDir(), "projects.json"))
	project, err := service.Add(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	marker := filepath.Join(project.Path, "must-not-exist")
	binary := filepath.Join(project.Path, "dangerous")
	aiFixture(t, binary, "#!/bin/sh\ntouch '"+marker+"'\n")
	if err := os.Chmod(binary, 0700); err != nil {
		t.Fatal(err)
	}
	config := map[string]any{"services": []LocalServiceDefinition{{ID: "dangerous", Name: "Must not run", Command: []string{binary}}}, "aiResources": []AIResourceAdoption{{Kind: "skill", Name: "missing", Source: filepath.Join(project.Path, "missing/SKILL.md")}, {Kind: "cli", Name: "repsync-not-a-real-command-123"}}}
	data, err := json.Marshal(config)
	if err != nil {
		t.Fatal(err)
	}
	aiFixture(t, filepath.Join(project.Path, ".repsync.json"), string(data))
	report, err := service.DiagnoseProject(project.Path)
	if err != nil || len(report.AIResources) != 2 {
		t.Fatalf("doctor: %+v %v", report, err)
	}
	for _, resource := range report.AIResources {
		if resource.Availability != "missing" {
			t.Fatalf("missing prerequisite hidden: %+v", resource)
		}
	}
	if _, err := os.Stat(marker); !os.IsNotExist(err) {
		t.Fatal("diagnosis executed a project command")
	}
}
