//go:build integration

package workspace_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"repsync/internal/workspace"
)

func resourceProject(t *testing.T, report workspace.ResourceOverview, path string) workspace.ResourceProject {
	t.Helper()
	for _, project := range report.Projects {
		if project.Project.Path == path {
			return project
		}
	}
	t.Fatal("project missing from resource overview")
	return workspace.ResourceProject{}
}

func TestResourceManagementAdoptsAcrossProjectsAndPreservesConfig(t *testing.T) {
	t.Parallel()
	s, first, source := resourceFixture(t)
	second, err := s.Add(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	configPath := filepath.Join(second.Path, ".repsync.json")
	writeSyncFile(t, configPath, `{"services":[],"custom":{"keep":true}}`)
	if err := os.Chmod(configPath, 0640); err != nil {
		t.Fatal(err)
	}
	read := func() workspace.ResourceOverview {
		t.Helper()
		report, err := s.SharedResources("")
		if err != nil {
			t.Fatal(err)
		}
		return report
	}
	report := read()
	if len(report.Resources) != 1 || report.Resources[0].Version == "" || len(report.Projects) != 2 {
		t.Fatalf("incomplete overview: %+v", report)
	}
	project := resourceProject(t, report, second.Path)
	if err := s.SetResourceAdoption(second.Path, project.ConfigVersion, report.Version, "check", true); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(second.Path, "shared-check.sh")); !os.IsNotExist(err) {
		t.Fatal("adoption wrote the shared file before preview/apply")
	}
	var fields map[string]json.RawMessage
	if err := json.Unmarshal([]byte(readSyncFile(t, configPath)), &fields); err != nil || string(fields["services"]) != "[]" || fields["custom"] == nil {
		t.Fatal("adoption damaged project differences")
	}
	info, err := os.Stat(configPath)
	if err != nil || info.Mode().Perm() != 0640 {
		t.Fatal("adoption changed project configuration permissions")
	}
	if err := s.SetResourceAdoption(second.Path, project.ConfigVersion, report.Version, "check", false); err == nil {
		t.Fatal("accepted stale project configuration")
	}
	for _, path := range []string{first, second.Path} {
		preview, err := s.PreviewResource(path, "check")
		if err != nil {
			t.Fatal(err)
		}
		if _, err := s.ApplyUpdate(preview.Token); err != nil {
			t.Fatal(err)
		}
	}
	writeSyncFile(t, source, "#!/bin/sh\necho updated-common-source\n")
	report = read()
	for _, path := range []string{first, second.Path} {
		project := resourceProject(t, report, path)
		if project.Error != "" || len(project.Adoptions) != 1 || project.Adoptions[0].Status != "outdated" {
			t.Fatalf("did not identify affected consumer: %+v", project)
		}
	}
	writeSyncFile(t, filepath.Join(second.Path, "shared-check.sh"), "# independent project change\n")
	preview, err := s.PreviewResource(second.Path, "check")
	if err != nil || !preview.Conflict {
		t.Fatalf("project difference not protected: %+v %v", preview, err)
	}
	if _, err := s.ApplyUpdate(preview.Token); err == nil {
		t.Fatal("overwrote project changes")
	}
	report = read()
	project = resourceProject(t, report, second.Path)
	if err := s.SetResourceAdoption(second.Path, project.ConfigVersion, report.Version, "check", false); err != nil {
		t.Fatal(err)
	}
	if readSyncFile(t, filepath.Join(second.Path, "shared-check.sh")) != "# independent project change\n" {
		t.Fatal("cancelling adoption removed the project file")
	}
	history, err := s.ResourceHistory(second.Path)
	if err != nil || len(history) != 1 {
		t.Fatal("cancelling adoption removed update history")
	}
}

func TestResourceManagementProtectsCatalogAndShowsUnavailableSources(t *testing.T) {
	t.Parallel()
	s, path, source := resourceFixture(t)
	report, err := s.SharedResources(path)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.SaveResource(report.Version, "other", source, "shared-check.sh"); err != nil {
		t.Fatal(err)
	}
	if err := s.ForgetResource(report.Version, "check"); err == nil {
		t.Fatal("removed registration with a stale catalog")
	}
	if _, err := s.SaveResource(report.Version, "check", source, "new.sh"); err == nil {
		t.Fatal("edited registration with a stale catalog")
	}
	report, err = s.SharedResources(path)
	if err != nil {
		t.Fatal(err)
	}
	project := resourceProject(t, report, path)
	if err := s.SetResourceAdoption(path, project.ConfigVersion, report.Version, "other", true); err == nil {
		t.Fatal("adopted conflicting target destinations")
	}
	if err := os.Remove(source); err != nil {
		t.Fatal(err)
	}
	report, err = s.SharedResources(path)
	if err != nil || report.Resources[0].Error == "" || report.Projects[0].Adoptions[0].Status != "error" {
		t.Fatalf("unavailable source hidden: %+v %v", report, err)
	}
	if err := s.ForgetResource(report.Version, "check"); err != nil {
		t.Fatal(err)
	}
	report, err = s.SharedResources(path)
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, resource := range report.Resources {
		if resource.ID == "check" && resource.Error != "" {
			found = true
		}
	}
	if !found {
		t.Fatal("dangling project adoption disappeared from the overview")
	}
	for _, invalid := range []string{"invalid-json", `{"resources":{}}`} {
		writeSyncFile(t, filepath.Join(path, ".repsync.json"), invalid)
		report, err = s.SharedResources(path)
		if err != nil || report.Projects[0].Error == "" || report.Projects[0].Adoptions == nil {
			t.Fatalf("project read failure must retain an error and an array: %+v %v", report, err)
		}
	}
}
