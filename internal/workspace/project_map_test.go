//go:build integration

package workspace_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"repsync/internal/workspace"
)

func TestProjectMapCurrentReferences(t *testing.T) {
	t.Parallel()
	service := workspace.NewService(filepath.Join(t.TempDir(), "projects.json"))
	project, err := service.Add(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(service.Shutdown)
	view, err := service.ProjectMap(project.Path)
	if err != nil || view.Exists || len(view.Modules) != 0 {
		t.Fatalf("missing map: %+v %v", view, err)
	}
	write := func(name, content string) {
		t.Helper()
		if err := os.WriteFile(filepath.Join(project.Path, name), []byte(content), 0600); err != nil {
			t.Fatal(err)
		}
	}
	write("Page.tsx", "export function Page() {}\n")
	write("Legacy.tsx", "export function Legacy() {}\n")
	write("App.tsx", "// entry\nconst page = <Page />;\n")
	const document = `{"version":1,"modules":[{"id":"settings","name":"Settings","overlap":"Both pages edit project preferences","implementations":[{"name":"Current","source":{"path":"Page.tsx","find":"function Page"},"entry":{"path":"App.tsx","find":"<Page />"}},{"name":"Older","source":{"path":"Legacy.tsx"}}]},{"id":"intentional","name":"Intentional variants","implementations":[{"name":"Variant one","source":{"path":"Page.tsx"}},{"name":"Variant two","source":{"path":"Legacy.tsx"}}]}]}`
	write(".repsync-map.json", document)
	view, err = service.ProjectMap(project.Path)
	if err != nil || !view.Exists || len(view.Modules) != 2 {
		t.Fatalf("read map: %+v %v", view, err)
	}
	items := view.Modules[0].Implementations
	if items[0].Source.Line != 1 || items[0].Entry.Line != 2 || items[0].Entry.Issue != "" || items[1].Entry != nil {
		t.Fatalf("lost located or unmounted implementation: %+v", items)
	}
	if view.Modules[1].Overlap != "" {
		t.Fatal("multiple implementations were automatically called duplicates")
	}
	write("App.tsx", "// inserted line\n// entry\nconst page = <Page />;\n")
	view, err = service.ProjectMap(project.Path)
	if err != nil || view.Modules[0].Implementations[0].Entry.Line != 3 {
		t.Fatalf("line moved: %+v %v", view, err)
	}
	write("App.tsx", "const first = <Page />;\nconst second = <Page />;\n")
	if err := os.Remove(filepath.Join(project.Path, "Legacy.tsx")); err != nil {
		t.Fatal(err)
	}
	view, err = service.ProjectMap(project.Path)
	if err != nil {
		t.Fatal(err)
	}
	items = view.Modules[0].Implementations
	if items[0].Entry.Line != 0 || items[0].Entry.Issue == "" || items[1].Source.Issue == "" || len(view.Modules) != 2 {
		t.Fatalf("stale evidence hidden or marked located: %+v", view)
	}
	write("App.tsx", "const page = <DifferentPage />;\n")
	view, err = service.ProjectMap(project.Path)
	if err != nil || view.Modules[0].Implementations[0].Entry.Issue == "" {
		t.Fatalf("removed entry still located: %+v %v", view, err)
	}
	actual, err := os.ReadFile(filepath.Join(project.Path, ".repsync-map.json"))
	if err != nil || string(actual) != document {
		t.Fatal("reading the map rewrote its declarations")
	}
}

func TestProjectMapBoundaries(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name     string
		document string
	}{
		{"unsupported version", `{"version":2,"modules":[]}`},
		{"invalid JSON", `{"version":1,`},
		{"trailing object", `{"version":1,"modules":[]} {}`},
		{"unknown field", `{"version":1,"modulez":[]}`},
		{"empty implementation", `{"version":1,"modules":[{"id":"a","name":"A","implementations":[]}]}`},
		{"escape path", `{"version":1,"modules":[{"id":"a","name":"A","implementations":[{"name":"A","source":{"path":"../outside"}}]}]}`},
		{"unbounded map", strings.Repeat(" ", 256*1024+1)},
	}
	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			service := workspace.NewService(filepath.Join(t.TempDir(), "projects.json"))
			t.Cleanup(service.Shutdown)
			project, err := service.Add(t.TempDir())
			if err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(filepath.Join(project.Path, ".repsync-map.json"), []byte(test.document), 0600); err != nil {
				t.Fatal(err)
			}
			if _, err := service.ProjectMap(project.Path); err == nil {
				t.Fatal("invalid map accepted")
			}
		})
	}
}

func TestProjectMapDoesNotTrustOutsideOrDeclaredEvidence(t *testing.T) {
	t.Parallel()
	service := workspace.NewService(filepath.Join(t.TempDir(), "projects.json"))
	t.Cleanup(service.Shutdown)
	project, err := service.Add(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	outside := filepath.Join(t.TempDir(), "outside.tsx")
	if err := os.WriteFile(outside, []byte("<HiddenPage />\n"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, filepath.Join(project.Path, "linked.tsx")); err != nil {
		t.Fatal(err)
	}
	mapData := map[string]any{"version": 1, "modules": []any{map[string]any{
		"id": "hidden", "name": "Hidden", "implementations": []any{map[string]any{
			"name": "Hidden page", "source": map[string]any{"path": "linked.tsx", "line": 999},
			"entry": map[string]any{"path": "linked.tsx", "find": "<HiddenPage />", "line": 999},
		}},
	}}}
	data, err := json.Marshal(mapData)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(project.Path, ".repsync-map.json"), data, 0600); err != nil {
		t.Fatal(err)
	}
	view, err := service.ProjectMap(project.Path)
	if err != nil {
		t.Fatal(err)
	}
	item := view.Modules[0].Implementations[0]
	if item.Source.Line != 0 || item.Source.Issue == "" || item.Entry.Line != 0 || item.Entry.Issue == "" {
		t.Fatalf("outside or forged evidence accepted: %+v", item)
	}
	if _, err := service.ProjectMap(t.TempDir()); err == nil {
		t.Fatal("unregistered project read accepted")
	}
}
