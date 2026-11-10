//go:build integration

package workspace_test

import (
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"repsync/internal/workspace"
)

func cloneGit(t *testing.T, args ...string) string {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Env = append(os.Environ(), "GIT_CONFIG_GLOBAL=/dev/null", "GIT_CONFIG_SYSTEM=/dev/null")
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %v: %v %s", args, err, out)
	}
	return strings.TrimSpace(string(out))
}

func cloneService(t *testing.T) (*workspace.Service, string) {
	t.Helper()
	t.Setenv("GIT_CONFIG_GLOBAL", "/dev/null")
	t.Setenv("GIT_CONFIG_SYSTEM", "/dev/null")
	root := t.TempDir()
	parent := filepath.Join(root, "projects")
	if err := os.Mkdir(parent, 0700); err != nil {
		t.Fatal(err)
	}
	service := workspace.NewService(filepath.Join(root, "config", "projects.json"))
	t.Cleanup(service.Shutdown)
	return service, parent
}

func cloneRemoteServer(t *testing.T) *httptest.Server {
	t.Helper()
	root := t.TempDir()
	work, bare := filepath.Join(root, "work"), filepath.Join(root, "repo.git")
	if err := os.Mkdir(work, 0700); err != nil {
		t.Fatal(err)
	}
	cloneGit(t, "-C", work, "init", "-b", "main")
	if err := os.WriteFile(filepath.Join(work, "README.md"), []byte("# Remote fixture\n"), 0600); err != nil {
		t.Fatal(err)
	}
	cloneGit(t, "-C", work, "add", "README.md")
	cloneGit(t, "-C", work, "-c", "user.name=Fixture", "-c", "user.email=fixture@example.invalid", "commit", "-m", "fixture")
	cloneGit(t, "clone", "--bare", work, bare)
	cloneGit(t, "--git-dir", bare, "update-server-info")
	server := httptest.NewServer(http.FileServer(http.Dir(root)))
	t.Cleanup(server.Close)
	return server
}

func waitForClone(t *testing.T, service *workspace.Service, id string) workspace.CloneState {
	t.Helper()
	deadline := time.Now().Add(8 * time.Second)
	for time.Now().Before(deadline) {
		state, err := service.CloneStatus(id)
		if err != nil {
			t.Fatal(err)
		}
		if state.Status != "running" {
			return state
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatal("clone did not stop")
	return workspace.CloneState{}
}

func TestCloneRemoteRepositoryRegistersOnSuccess(t *testing.T) {
	service, parent := cloneService(t)
	server := cloneRemoteServer(t)
	remote := server.URL + "/repo.git"
	initial, err := service.StartClone(remote, parent, "cloned project")
	if err != nil {
		t.Fatal(err)
	}
	state := waitForClone(t, service, initial.ID)
	if state.Status != "success" || !state.Cloned || state.Project.Path == "" {
		t.Fatalf("clone: %+v", state)
	}
	data, err := os.ReadFile(filepath.Join(state.Project.Path, "README.md"))
	if err != nil || string(data) != "# Remote fixture\n" {
		t.Fatalf("checkout content: %q %v", data, err)
	}
	if origin := cloneGit(t, "-C", state.Project.Path, "config", "--get", "remote.origin.url"); origin != remote {
		t.Fatalf("origin = %q", origin)
	}
	projects, err := service.List()
	if err != nil || len(projects) != 1 || projects[0] != state.Project {
		t.Fatalf("registration: %+v %v", projects, err)
	}
	if !strings.Contains(state.Log, "Cloning into") {
		t.Fatalf("missing progress output: %q", state.Log)
	}
}

func TestCloneProtectsExistingDestinations(t *testing.T) {
	service, parent := cloneService(t)
	for _, name := range []string{"existing folder", "empty folder", "linked folder"} {
		t.Run(name, func(t *testing.T) {
			target := filepath.Join(parent, name)
			if name == "linked folder" {
				if err := os.Symlink(parent, target); err != nil {
					t.Fatal(err)
				}
			} else {
				if err := os.Mkdir(target, 0700); err != nil {
					t.Fatal(err)
				}
				if name == "existing folder" {
					if err := os.WriteFile(filepath.Join(target, "keep.txt"), []byte("Keep"), 0600); err != nil {
						t.Fatal(err)
					}
				}
			}
			if _, err := service.StartClone("https://example.invalid/repo.git", parent, name); err == nil {
				t.Fatal("existing destination accepted")
			}
			if _, err := os.Lstat(target); err != nil {
				t.Fatal("existing destination changed")
			}
		})
	}
	if data, err := os.ReadFile(filepath.Join(parent, "existing folder", "keep.txt")); err != nil || string(data) != "Keep" {
		t.Fatal("existing data changed")
	}
}

func TestCloneRejectsUnsafeInputs(t *testing.T) {
	service, parent := cloneService(t)
	cases := []struct{ name, remote, folder string }{
		{"option", "--upload-pack=bad", "repo"},
		{"local path", "/tmp/repo.git", "repo"},
		{"file transport", "file:///tmp/repo.git", "repo"},
		{"remote helper", "ext::sh -c bad", "repo"},
		{"embedded password", "https://user:secret@example.invalid/repo.git", "repo"},
		{"embedded token", "https://token@example.invalid/repo.git", "repo"},
		{"query token", "https://example.invalid/repo.git?token=secret", "repo"},
		{"ssh host option", "ssh://git@-host/repo.git", "repo"},
		{"parent traversal", "https://example.invalid/repo.git", "../escape"},
		{"absolute destination", "https://example.invalid/repo.git", "/tmp/escape"},
		{"current directory", "https://example.invalid/repo.git", "."},
		{"nested destination", "https://example.invalid/repo.git", "nested/repo"},
	}
	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			if _, err := service.StartClone(test.remote, parent, test.folder); err == nil {
				t.Fatal("unsafe clone accepted")
			}
		})
	}
	entries, err := os.ReadDir(parent)
	if err != nil || len(entries) != 0 {
		t.Fatalf("rejected inputs changed parent: %v %v", entries, err)
	}
}

func TestCloneFailureDoesNotRegisterProject(t *testing.T) {
	service, parent := cloneService(t)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { http.Error(w, "Forbidden", http.StatusForbidden) }))
	defer server.Close()
	state, err := service.StartClone(server.URL+"/private.git", parent, "private")
	if err != nil {
		t.Fatal(err)
	}
	state = waitForClone(t, service, state.ID)
	if state.Status != "failure" || state.Error == "" || state.Cloned {
		t.Fatalf("failure: %+v", state)
	}
	projects, err := service.List()
	if err != nil || len(projects) != 0 {
		t.Fatalf("failed clone registered: %+v %v", projects, err)
	}
}

func TestCloneCancellationAndShutdown(t *testing.T) {
	for _, operation := range []string{"cancel", "shutdown"} {
		t.Run(operation, func(t *testing.T) {
			service, parent := cloneService(t)
			started := make(chan struct{}, 1)
			server := httptest.NewServer(http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
				select {
				case started <- struct{}{}:
				default:
				}
				<-r.Context().Done()
			}))
			defer func() {
				service.Shutdown()
				server.Close()
			}()
			state, err := service.StartClone(server.URL+"/slow.git", parent, "slow")
			if err != nil {
				t.Fatal(err)
			}
			select {
			case <-started:
			case <-time.After(5 * time.Second):
				t.Fatal("clone never contacted fixture")
			}
			if _, err := service.StartClone(server.URL+"/second.git", parent, "second"); err == nil {
				t.Fatal("concurrent clone accepted")
			}
			if err := service.CancelClone("wrong-id"); err == nil {
				t.Fatal("unknown clone cancellation accepted")
			}
			if operation == "cancel" {
				if err := service.CancelClone(state.ID); err != nil {
					t.Fatal(err)
				}
			} else {
				service.Shutdown()
			}
			state = waitForClone(t, service, state.ID)
			if state.Status != "cancelled" {
				t.Fatalf("cancellation state: %+v", state)
			}
			if projects, err := service.List(); err != nil || len(projects) != 0 {
				t.Fatalf("cancelled clone registered: %+v %v", projects, err)
			}
		})
	}
}

func TestCloneRegistrationFailureKeepsCheckout(t *testing.T) {
	service, parent := cloneService(t)
	server := cloneRemoteServer(t)
	config := filepath.Join(filepath.Dir(parent), "config", "projects.json")
	if err := os.MkdirAll(filepath.Dir(config), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(config, []byte("broken registration"), 0600); err != nil {
		t.Fatal(err)
	}
	initial, err := service.StartClone(server.URL+"/repo.git", parent, "recoverable")
	if err != nil {
		t.Fatal(err)
	}
	state := waitForClone(t, service, initial.ID)
	if state.Status != "failure" || !state.Cloned || !strings.Contains(state.Error, "登记失败") {
		t.Fatalf("registration failure: %+v", state)
	}
	if content, err := os.ReadFile(filepath.Join(state.Destination, "README.md")); err != nil || string(content) != "# Remote fixture\n" {
		t.Fatal("completed checkout was lost")
	}
	if data, err := os.ReadFile(config); err != nil || string(data) != "broken registration" {
		t.Fatal("corrupt registration overwritten")
	}
	if err := os.WriteFile(config, []byte("[]"), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := service.Add(state.Destination); err != nil {
		t.Fatal(err)
	}
}
