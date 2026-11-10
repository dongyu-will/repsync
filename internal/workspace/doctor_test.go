//go:build integration

package workspace_test

import (
	"os"
	"path/filepath"
	"repsync/internal/workspace"
	"testing"
)

func TestDoctorReportsActionableProblemsWithoutExecuting(t *testing.T) {
	t.Parallel()
	s := workspace.NewService(filepath.Join(t.TempDir(), "projects.json"))
	project, err := s.Add(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	marker := filepath.Join(project.Path, "must-not-run")
	writeLocalServices(t, project.Path, []workspace.LocalServiceDefinition{
		{ID: "setup", Name: "Setup", Command: []string{"/bin/sh", "-c", "touch '" + marker + "'"}},
		{ID: "missing", Name: "Missing", Command: []string{filepath.Join(project.Path, "missing-binary")}},
	})
	before := readSyncFile(t, filepath.Join(project.Path, ".repsync.json"))
	report, err := s.DiagnoseProject(project.Path)
	if err != nil || report.Ready || report.ConfigVersion == "" {
		t.Fatalf("doctor: %+v %v", report, err)
	}
	found := false
	for _, issue := range report.Issues {
		if issue.Kind == "executable" && issue.ServiceID == "missing" {
			found = true
		}
	}
	if !found {
		t.Fatal("missing executable not explained")
	}
	if _, err := os.Stat(marker); !os.IsNotExist(err) {
		t.Fatal("doctor ran project command")
	}
	if before != readSyncFile(t, filepath.Join(project.Path, ".repsync.json")) {
		t.Fatal("doctor rewrote config")
	}
}
