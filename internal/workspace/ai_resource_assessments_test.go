//go:build integration

package workspace

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

type assessmentFixture struct {
	service  *Service
	project  Project
	manifest string
}

func newAssessmentFixture(t *testing.T) assessmentFixture {
	t.Helper()
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("CODEX_HOME", filepath.Join(home, ".codex"))
	bin := filepath.Join(home, "bin")
	aiFixture(t, filepath.Join(bin, "codex"), "#!/bin/sh\nprintf '[]'\n")
	if err := os.Chmod(filepath.Join(bin, "codex"), 0700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))
	service := NewService(filepath.Join(t.TempDir(), "projects.json"))
	project, err := service.Add(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	manifest := filepath.Join(project.Path, ".agents/skills/existing/SKILL.md")
	aiFixture(t, manifest, "---\nname: existing\n---\nReview interfaces")
	aiFixture(t, filepath.Join(project.Path, "AGENTS.md"), "Use targeted checks for daily changes.")
	return assessmentFixture{service: service, project: project, manifest: manifest}
}

func (f assessmentFixture) inventory(t *testing.T, scope string) AIResourceReport {
	t.Helper()
	report, err := f.service.AIResources(scope)
	if err != nil {
		t.Fatal(err)
	}
	return report
}

func candidateAssessment(context string) AIResourceAssessmentInput {
	return AIResourceAssessmentInput{AIResourceAssessmentData: AIResourceAssessmentData{
		ContextVersion: context,
		Candidate:      &AISkillCandidate{Name: "release-helper", Source: "https://example.invalid/release-helper/SKILL.md", SourceVersion: "v1"},
		Verdict:        "conditional", Summary: "建议按条件接入，保留项目门禁", Fit: "适合发布准备，界面检查已有覆盖",
		AdoptionCost: "通过原有渠道接入；仅限发布任务，撤回本次配置", Coverage: "检查候选正文、项目规则和 existing；未实测",
		Findings: []AIResourceFinding{{Kind: "conflict", Resource: "项目规则", Evidence: "候选要求每次完整 CI，项目允许针对性检查",
			Recommendation: "限定发布任务触发，并遵循项目门禁",
			Sources:        []string{"https://example.invalid/release-helper/SKILL.md#ci", "AGENTS.md:1"}}},
		Sources: []string{"https://example.invalid/release-helper/SKILL.md"}, CheckedAt: time.Now().UTC().Format(time.RFC3339),
	}}
}

func TestAIResourceAssessmentsCandidateLifecycle(t *testing.T) {
	f := newAssessmentFixture(t)
	input := candidateAssessment(f.inventory(t, f.project.Path).ContextVersion)
	item, err := f.service.RecordAIResourceAssessment(f.project.Path, input)
	if err != nil {
		t.Fatal(err)
	}
	input.ExpectedVersion = item.Version
	find := func(report AIResourceReport, id string) AIResource {
		t.Helper()
		for _, resource := range report.Resources {
			if resource.ID == id {
				return resource
			}
		}
		t.Fatalf("resource missing: %s", id)
		return AIResource{}
	}
	report := f.inventory(t, f.project.Path)
	candidate := find(report, item.ResourceID)
	if candidate.Availability != "candidate" || candidate.Assessment == nil || candidate.Assessment.Stale {
		t.Fatalf("candidate assessment not visible: %+v", candidate)
	}
	if _, err := os.Stat(filepath.Join(f.project.Path, ".agents/skills/release-helper")); !os.IsNotExist(err) {
		t.Fatal("assessment installed the candidate")
	}
	if _, err := f.service.RecordAIResourceAssessment(f.project.Path, candidateAssessment(report.ContextVersion)); err == nil {
		t.Fatal("concurrent assessment replacement accepted")
	}
	aiFixture(t, filepath.Join(f.project.Path, "AGENTS.md"), "Updated project validation rules.")
	if _, err := f.service.RecordAIResourceAssessment(f.project.Path, input); err == nil || !strings.Contains(err.Error(), "context changed") {
		t.Fatalf("outdated project context accepted: %v", err)
	}
	report = f.inventory(t, f.project.Path)
	if !find(report, item.ResourceID).Assessment.Stale {
		t.Fatal("changed project rules did not expire the assessment")
	}
	input.ContextVersion = report.ContextVersion
	item, err = f.service.RecordAIResourceAssessment(f.project.Path, input)
	if err != nil {
		t.Fatal(err)
	}
	input.ExpectedVersion = item.Version
	// Installing is a separate external action. Same names do not link its source.
	installedPath := filepath.Join(f.project.Path, ".agents/skills/release-helper/SKILL.md")
	aiFixture(t, installedPath, "---\nname: release-helper\n---\nRelease guidance")
	report = f.inventory(t, f.project.Path)
	installed := find(report, skillID(installedPath))
	if find(report, item.ResourceID).Availability != "candidate" || installed.Assessment != nil {
		t.Fatal("name matching falsely established installation")
	}
	input.Candidate.InstalledResourceID = installed.ID
	input.ResourceRevision, input.ContextVersion = installed.Revision, report.ContextVersion
	item, err = f.service.RecordAIResourceAssessment(f.project.Path, input)
	if err != nil {
		t.Fatal(err)
	}
	report = f.inventory(t, f.project.Path)
	installed = find(report, installed.ID)
	if installed.Assessment == nil || installed.Assessment.Stale || installed.Assessment.ResourceID != item.ResourceID {
		t.Fatal("explicit installation link did not preserve the assessment")
	}
	for _, resource := range report.Resources {
		if resource.ID == item.ResourceID {
			t.Fatal("linked candidate remained as a duplicate row")
		}
	}
	aiFixture(t, installedPath, "---\nname: release-helper\n---\nChanged installed instructions")
	if !find(f.inventory(t, f.project.Path), installed.ID).Assessment.Stale {
		t.Fatal("installed resource changes did not expire assessment")
	}
}

func TestAIResourceAssessmentsScopeAndValidation(t *testing.T) {
	f := newAssessmentFixture(t)
	input := candidateAssessment(f.inventory(t, "").ContextVersion)
	global, err := f.service.RecordAIResourceAssessment("", input)
	if err != nil {
		t.Fatal(err)
	}
	report := f.inventory(t, f.project.Path)
	var candidate AIResource
	for _, resource := range report.Resources {
		if resource.ID == global.ResourceID {
			candidate = resource
		}
	}
	if candidate.Assessment == nil || !candidate.Assessment.Stale || candidate.Assessment.Scope != "" {
		t.Fatal("global evaluation was treated as project approval")
	}
	input.ContextVersion, input.ExpectedVersion = report.ContextVersion, global.Version
	for _, test := range []struct {
		name   string
		change func(*AIResourceAssessmentInput)
	}{
		{name: "unconditional conflict", change: func(in *AIResourceAssessmentInput) { in.Verdict = "recommended" }},
		{name: "one sided conflict", change: func(in *AIResourceAssessmentInput) { in.Findings[0].Sources = []string{"only one side"} }},
		{name: "duplicate conflict sources", change: func(in *AIResourceAssessmentInput) { in.Findings[0].Sources = []string{"same source", "same source"} }},
		{name: "unread candidate", change: func(in *AIResourceAssessmentInput) { in.Candidate.SourceVersion = "" }},
		{name: "missing coverage", change: func(in *AIResourceAssessmentInput) { in.Coverage = "" }},
		{name: "oversized", change: func(in *AIResourceAssessmentInput) { in.Fit = strings.Repeat("x", 64*1024) }},
	} {
		t.Run(test.name, func(t *testing.T) {
			invalid := input
			candidate := *input.Candidate
			invalid.Candidate = &candidate
			invalid.Findings = append([]AIResourceFinding{}, input.Findings...)
			test.change(&invalid)
			if _, err := f.service.RecordAIResourceAssessment(f.project.Path, invalid); err == nil {
				t.Fatal("invalid assessment accepted")
			}
		})
	}
	input.Summary = "项目专属的条件建议"
	if _, err := f.service.RecordAIResourceAssessment(f.project.Path, input); err != nil {
		t.Fatal(err)
	}
	for _, resource := range f.inventory(t, "").Resources {
		if resource.ID == global.ResourceID && resource.Assessment.Summary == input.Summary {
			t.Fatal("project report replaced global assessment")
		}
	}
	// Corruption is visible, and never treated as an empty or approved assessment.
	aiFixture(t, f.service.aiResourceAssessmentPath(f.project.Path), "invalid JSON")
	report = f.inventory(t, f.project.Path)
	if !strings.Contains(strings.Join(report.Issues, " "), "assessments") {
		t.Fatal("corrupt assessment hidden")
	}
}

func TestAIResourceAssessmentsInstalledResource(t *testing.T) {
	f := newAssessmentFixture(t)
	report := f.inventory(t, f.project.Path)
	input := candidateAssessment(report.ContextVersion)
	input.Candidate = nil
	input.ResourceID = skillID(f.manifest)
	for _, resource := range report.Resources {
		if resource.ID == input.ResourceID {
			input.ResourceRevision = resource.Revision
		}
	}
	if _, err := f.service.RecordAIResourceAssessment(f.project.Path, input); err != nil {
		t.Fatal(err)
	}
	for _, resource := range f.inventory(t, f.project.Path).Resources {
		if resource.ID == input.ResourceID && (resource.Assessment == nil || resource.Assessment.Stale) {
			t.Fatal("installed reassessment missing")
		}
	}
}
