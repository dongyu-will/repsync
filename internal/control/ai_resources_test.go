//go:build integration

package control_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"repsync/internal/control"
	"repsync/internal/workspace"
	"strings"
	"testing"
	"time"
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

func TestControlAIResourceRecordRoundTrip(t *testing.T) {
	root := t.TempDir()
	t.Setenv("HOME", root)
	t.Setenv("CODEX_HOME", filepath.Join(root, ".codex"))
	service := workspace.NewService(filepath.Join(root, "projects.json"))
	project, err := service.Add(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	manifest := filepath.Join(project.Path, ".agents", "skills", "guide", "SKILL.md")
	if err := os.MkdirAll(filepath.Dir(manifest), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(manifest, []byte("---\nname: guide\n---\nWrite docs"), 0600); err != nil {
		t.Fatal(err)
	}
	socket := shortSocket(t)
	server, err := control.Listen(socket, service)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { service.Shutdown(); server.Close() })
	response, code := cli(t, socket, "ai-resources", project.Path)
	var report workspace.AIResourceReport
	if err := json.Unmarshal(response.Data, &report); code != 0 || err != nil {
		t.Fatalf("inventory: %d %+v %v", code, response, err)
	}
	var resource workspace.AIResource
	for _, item := range report.Resources {
		if item.Location == manifest {
			resource = item
		}
	}
	if resource.ID == "" {
		t.Fatal("project skill not inventoried")
	}
	input := workspace.AIResourceNoteInput{AIResourceNoteData: workspace.AIResourceNoteData{
		ResourceID: resource.ID, ResourceRevision: resource.Revision,
		CheckedAt: time.Now().UTC().Format(time.RFC3339), UpdateStatus: "unknown",
		Summary: "本地说明；未检查上游", Manual: "对 AI 说：整理当前文档。",
		Sources: []string{manifest},
	}}
	file := filepath.Join(root, "report.json")
	data, err := json.Marshal(input)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(file, data, 0600); err != nil {
		t.Fatal(err)
	}
	response, code = cli(t, socket, "ai-resource-record", project.Path, file)
	if code != 0 || !response.OK {
		t.Fatalf("record: %d %+v", code, response)
	}
	response, code = cli(t, socket, "ai-resources", project.Path)
	if err := json.Unmarshal(response.Data, &report); code != 0 || err != nil {
		t.Fatalf("reread: %d %+v %v", code, response, err)
	}
	found := false
	for _, item := range report.Resources {
		if item.ID == resource.ID {
			found = item.Note != nil && item.Note.Manual == input.Manual && !item.Note.Stale
		}
	}
	if !found {
		t.Fatal("saved manual missing from the CLI inventory")
	}
	assessment := workspace.AIResourceAssessmentInput{AIResourceAssessmentData: workspace.AIResourceAssessmentData{
		ContextVersion: report.ContextVersion,
		Candidate:      &workspace.AISkillCandidate{Name: "candidate", Source: "https://example.invalid/candidate"},
		Verdict:        "unknown", Summary: "正文暂不可读，无法判断适配", Fit: "待核对任务条件",
		AdoptionCost: "尚未确认安装渠道和撤回方式", Coverage: "仅收到来源链接，尚未读取正文",
		Findings: []workspace.AIResourceFinding{}, Sources: []string{}, CheckedAt: time.Now().UTC().Format(time.RFC3339),
	}}
	assessmentFile := filepath.Join(root, "assessment.json")
	encoded, err := json.Marshal(assessment)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(assessmentFile, encoded, 0600); err != nil {
		t.Fatal(err)
	}
	assessmentResponse, assessmentCode := cli(t, socket, "ai-resource-assess", project.Path, assessmentFile)
	var savedAssessment workspace.AIResourceAssessment
	if err := json.Unmarshal(assessmentResponse.Data, &savedAssessment); assessmentCode != 0 || err != nil || savedAssessment.Version == "" {
		t.Fatalf("assessment writeback failed: %d %+v %v", assessmentCode, assessmentResponse, err)
	}
	assessmentResponse, assessmentCode = cli(t, socket, "ai-resources", project.Path)
	if err := json.Unmarshal(assessmentResponse.Data, &report); assessmentCode != 0 || err != nil {
		t.Fatal("cannot reread candidate assessment")
	}
	found = false
	for _, item := range report.Resources {
		if item.ID == savedAssessment.ResourceID {
			found = item.Availability == "candidate" && item.Assessment != nil && item.Assessment.Verdict == "unknown" && !item.Assessment.Stale
		}
	}
	if !found {
		t.Fatal("candidate not returned by the CLI inventory")
	}
	_, assessmentCode = cli(t, socket, "ai-resource-assess")
	if assessmentCode != 2 {
		t.Fatal("missing assessment JSON file accepted")
	}
	response, code = cli(t, socket, "ai-resource-record", project.Path, file)
	if code != 1 || !strings.Contains(response.Error, "report changed") {
		t.Fatalf("stale record overwritten through CLI: %d %+v", code, response)
	}
	_, code = cli(t, socket, "ai-resource-record")
	if code != 2 {
		t.Fatal("missing JSON file accepted")
	}
}
