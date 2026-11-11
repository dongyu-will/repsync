//go:build integration

package workspace_test

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"repsync/internal/workspace"
)

func pipelineFixture(t *testing.T, steps [][]string, timeout int) (*workspace.Service, string, workspace.PipelineConfig) {
	t.Helper()
	root := t.TempDir()
	project := filepath.Join(root, "private project")
	if err := os.Mkdir(project, 0700); err != nil {
		t.Fatal(err)
	}
	writePipeline(t, project, steps, timeout)
	s := workspace.NewService(filepath.Join(root, "state", "projects.json"))
	t.Cleanup(s.Shutdown)
	p, err := s.Add(project)
	if err != nil {
		t.Fatal(err)
	}
	project = p.Path
	config, err := s.Pipelines(project)
	if err != nil {
		t.Fatal(err)
	}
	return s, project, config
}

func writePipeline(t *testing.T, path string, steps [][]string, timeout int) {
	t.Helper()
	data, err := json.Marshal(map[string]any{"pipelines": []workspace.Pipeline{{ID: "ci", Name: "Private CI", Kind: "ci", Steps: steps, TimeoutSeconds: timeout}}})
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(path, ".repsync.json"), data, 0600); err != nil {
		t.Fatal(err)
	}
}

func waitPipeline(t *testing.T, s *workspace.Service, path, id string) workspace.PipelineOutput {
	t.Helper()
	deadline := time.Now().Add(8 * time.Second)
	for time.Now().Before(deadline) {
		output, err := s.PipelineLog(path, id)
		if err != nil {
			t.Fatal(err)
		}
		if output.Run.Status != "running" {
			return output
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatal("pipeline did not stop before deadline")
	return workspace.PipelineOutput{}
}

func TestLocalPipelineExecutionAndHistory(t *testing.T) {
	t.Parallel()
	for _, test := range []struct {
		name   string
		steps  [][]string
		status string
		code   int
	}{
		{"success", [][]string{{"/bin/sh", "-c", "pwd; printf \"CI=$CI\"; printf stderr >&2"}}, "success", 0},
		{"failure stops next step", [][]string{{"/bin/sh", "-c", "exit 7"}, {"/bin/sh", "-c", "touch should-not-exist"}}, "failure", 7},
		{"missing executable", [][]string{{"repsync-nonexistent-test-executable"}}, "failure", -1},
	} {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			s, project, config := pipelineFixture(t, test.steps, 10)
			if _, err := os.Stat(filepath.Join(project, "should-not-exist")); !os.IsNotExist(err) {
				t.Fatal("discovery ran commands")
			}
			run, err := s.StartPipeline(project, "ci", config.Version)
			if err != nil {
				t.Fatal(err)
			}
			output := waitPipeline(t, s, project, run.ID)
			if output.Run.Status != test.status || output.Run.ExitCode != test.code {
				t.Fatalf("unexpected result: %+v", output)
			}
			if test.status == "success" && (!strings.Contains(output.Log, project) || !strings.Contains(output.Log, "CI=true") || !strings.Contains(output.Log, "stderr")) {
				t.Fatalf("wrong cwd/environment/log: %s", output.Log)
			}
			if _, err := os.Stat(filepath.Join(project, "should-not-exist")); !os.IsNotExist(err) {
				t.Fatal("executed a step after failure")
			}
			reopened := workspace.NewService(filepath.Join(filepath.Dir(project), "state", "projects.json"))
			history, err := reopened.PipelineRuns(project)
			if err != nil || len(history) != 1 || history[0].Status != test.status {
				t.Fatalf("history not persisted: %+v %v", history, err)
			}
			persisted, err := reopened.PipelineLog(project, run.ID)
			if err != nil || persisted.Log != output.Log {
				t.Fatalf("log not persisted: %+v %v", persisted, err)
			}
		})
	}
}

func TestLocalPipelineRejectsChangedConfigAndWrongProject(t *testing.T) {
	t.Parallel()
	s, project, config := pipelineFixture(t, [][]string{{"/bin/sh", "-c", "touch must-not-run"}}, 10)
	writePipeline(t, project, [][]string{{"/bin/sh", "-c", "touch changed-must-not-run"}}, 10)
	if _, err := s.StartPipeline(project, "ci", config.Version); err == nil {
		t.Fatal("stale config accepted")
	}
	if _, err := s.StartPipeline(filepath.Dir(project), "ci", config.Version); err == nil {
		t.Fatal("unregistered project accepted")
	}
	if _, err := s.PipelineLog(project, "../projects"); err == nil {
		t.Fatal("run path traversal accepted")
	}
	for _, name := range []string{"must-not-run", "changed-must-not-run"} {
		if _, err := os.Stat(filepath.Join(project, name)); !os.IsNotExist(err) {
			t.Fatal("rejected command executed")
		}
	}
}

func TestLocalPipelineStopsChildrenAndRejectsDuplicate(t *testing.T) {
	t.Parallel()
	s, project, config := pipelineFixture(t, [][]string{{"/bin/sh", "-c", "(sleep 1; touch leaked-child) & echo started; wait"}}, 10)
	run, err := s.StartPipeline(project, "ci", config.Version)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.StartPipeline(project, "ci", config.Version); err == nil {
		t.Fatal("duplicate project run accepted")
	}
	deadline := time.Now().Add(3 * time.Second)
	for {
		output, err := s.PipelineLog(project, run.ID)
		if err != nil {
			t.Fatal(err)
		}
		if strings.Contains(output.Log, "\nstarted\n") {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("child did not start")
		}
		time.Sleep(10 * time.Millisecond)
	}
	if err := s.StopPipeline(project, run.ID); err != nil {
		t.Fatal(err)
	}
	if output := waitPipeline(t, s, project, run.ID); output.Run.Status != "cancelled" {
		t.Fatalf("not cancelled: %+v", output)
	}
	time.Sleep(1100 * time.Millisecond)
	if _, err := os.Stat(filepath.Join(project, "leaked-child")); !os.IsNotExist(err) {
		t.Fatal("child survived cancellation")
	}
}

func TestLocalPipelineTimeoutAndBoundedLog(t *testing.T) {
	t.Parallel()
	t.Run("timeout", func(t *testing.T) {
		t.Parallel()
		s, project, config := pipelineFixture(t, [][]string{{"/bin/sh", "-c", "sleep 10"}}, 1)
		run, err := s.StartPipeline(project, "ci", config.Version)
		if err != nil {
			t.Fatal(err)
		}
		if output := waitPipeline(t, s, project, run.ID); output.Run.Status != "timed_out" {
			t.Fatalf("not timed out: %+v", output)
		}
	})
	t.Run("log truncation preserves result", func(t *testing.T) {
		t.Parallel()
		s, project, config := pipelineFixture(t, [][]string{{"/bin/sh", "-c", "printf '%0300000d' 0"}}, 10)
		run, err := s.StartPipeline(project, "ci", config.Version)
		if err != nil {
			t.Fatal(err)
		}
		output := waitPipeline(t, s, project, run.ID)
		if output.Run.Status != "success" || len(output.Log) > 256*1024+256 || !strings.Contains(output.Log, "further output omitted") {
			t.Fatalf("wrong bounded log: status %s, length %d", output.Run.Status, len(output.Log))
		}
	})
}

func TestLocalPipelineProjectIsolationAndShutdown(t *testing.T) {
	t.Parallel()
	s, project, config := pipelineFixture(t, [][]string{{"/bin/sh", "-c", "echo project-one; sleep 10"}}, 15)
	first, err := s.StartPipeline(project, "ci", config.Version)
	if err != nil {
		t.Fatal(err)
	}
	paths, ids := []string{project}, []string{first.ID}
	for i := 0; i < 4; i++ {
		path := t.TempDir()
		writePipeline(t, path, [][]string{{"/bin/sh", "-c", "echo other-project; sleep 10"}}, 15)
		p, err := s.Add(path)
		if err != nil {
			t.Fatal(err)
		}
		config, err := s.Pipelines(p.Path)
		if err != nil {
			t.Fatal(err)
		}
		run, err := s.StartPipeline(p.Path, "ci", config.Version)
		if i == 3 {
			if err == nil {
				t.Fatal("allowed more than four active projects")
			}
			break
		}
		if err != nil {
			t.Fatal(err)
		}
		paths, ids = append(paths, p.Path), append(ids, run.ID)
	}
	if _, err := s.PipelineLog(paths[1], first.ID); err == nil {
		t.Fatal("read another project's log")
	}
	s.Shutdown()
	for i, path := range paths {
		output, err := s.PipelineLog(path, ids[i])
		if err != nil || output.Run.Status != "cancelled" {
			t.Fatalf("shutdown left a job running: %+v %v", output, err)
		}
		runs, err := s.PipelineRuns(path)
		if err != nil || len(runs) != 1 || runs[0].ID != ids[i] {
			t.Fatalf("project histories mixed: %+v %v", runs, err)
		}
	}
}

func TestLocalPipelineDiscoversOnlyExistingCI(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "package.json"), []byte(`{"packageManager":"pnpm@10.0.0","scripts":{"check:ci":"exit 0","deploy":"exit 1","test":"exit 0"}}`), 0600); err != nil {
		t.Fatal(err)
	}
	s := workspace.NewService(filepath.Join(t.TempDir(), "projects.json"))
	p, err := s.Add(root)
	if err != nil {
		t.Fatal(err)
	}
	config, err := s.Pipelines(p.Path)
	if err != nil || len(config.Pipelines) != 1 || config.Pipelines[0].ID != "check:ci" || config.Pipelines[0].Steps[0][0] != "pnpm" {
		t.Fatalf("incorrect CI discovery: %+v %v", config, err)
	}
	outside := filepath.Join(t.TempDir(), "external.json")
	if err := os.WriteFile(outside, []byte(`{"pipelines":[]}`), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, filepath.Join(root, ".repsync.json")); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Pipelines(p.Path); err == nil {
		t.Fatal("read config outside registered root")
	}
}

func TestGitHubActionsUsesProjectOriginAndLocalCLI(t *testing.T) {
	bin := t.TempDir()
	gh := "#!/bin/sh\nprintf '%s\\n' \"$*\" > args\nprintf '%s' '[{\"databaseId\":1,\"status\":\"completed\",\"conclusion\":\"success\",\"url\":\"https://untrusted.example/\"}]'\n"
	if err := os.WriteFile(filepath.Join(bin, "gh"), []byte(gh), 0700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))
	s, project, _ := pipelineFixture(t, [][]string{{"/bin/true"}}, 10)
	for _, args := range [][]string{{"init"}, {"remote", "add", "origin", "git@github.com:private-owner/private-repo.git"}} {
		cmd := exec.Command("git", append([]string{"-C", project}, args...)...)
		if output, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git fixture: %s %v", output, err)
		}
	}
	result, err := s.GitHubActions(project)
	if err != nil || result.Repository != "private-owner/private-repo" || len(result.Runs) != 1 || result.Runs[0].URL != "" {
		t.Fatalf("incorrect Actions result: %+v %v", result, err)
	}
	args, err := os.ReadFile(filepath.Join(project, "args"))
	if err != nil || !strings.Contains(string(args), "--repo private-owner/private-repo --limit 20 --json") {
		t.Fatalf("CLI targeted wrong repository: %s %v", args, err)
	}
}
