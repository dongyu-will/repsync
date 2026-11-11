//go:build integration

package workspace_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"repsync/internal/workspace"
)

func TestCodexProjectResourcesPreviewAndWithdraw(t *testing.T) {
	f := newGitFixture(t)
	apply := func(change workspace.CodexResourceChange) {
		t.Helper()
		preview, err := f.service.PreviewCodexResource(f.repo, change)
		if err != nil {
			t.Fatal(err)
		}
		if err := f.service.ApplyCodexResource(f.repo, change, preview.Version); err != nil {
			t.Fatal(err)
		}
	}
	source := filepath.Join(f.root, "shared-skill")
	if err := os.Mkdir(source, 0700); err != nil {
		t.Fatal(err)
	}
	manifest := filepath.Join(source, "SKILL.md")
	const original = "---\nname: doc-audit\ndescription: Audit docs\n---\nCheck the docs.\n"
	reviewWrite(t, manifest, original)
	skill := workspace.CodexResourceChange{Kind: "skill", Name: "doc-audit", Source: manifest, Action: "add"}
	preview, err := f.service.PreviewCodexResource(f.repo, skill)
	if err != nil {
		t.Fatal(err)
	}
	target := filepath.Join(f.repo, preview.Target)
	if _, err := os.Lstat(filepath.Join(f.repo, ".agents")); !os.IsNotExist(err) {
		t.Fatalf("preview created a directory: %v", err)
	}
	reviewWrite(t, manifest, original+"More guidance.\n")
	if err := f.service.ApplyCodexResource(f.repo, skill, preview.Version); err == nil {
		t.Fatal("stale skill preview applied")
	}
	apply(skill)
	if link, err := os.Readlink(target); err != nil || link != source {
		t.Fatalf("skill link: %q %v", link, err)
	}
	if _, err := f.service.PreviewCodexResource(f.repo, skill); err == nil {
		t.Fatal("duplicate skill accepted")
	}
	skill.Action, skill.Source = "remove", ""
	apply(skill)
	if _, err := os.Lstat(target); !os.IsNotExist(err) {
		t.Fatalf("link remains: %v", err)
	}
	if _, err := os.Stat(manifest); err != nil {
		t.Fatal("shared source removed:", err)
	}
	skill.Action, skill.Source = "add", manifest
	apply(skill)
	if err := os.Remove(target); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(target, 0700); err != nil {
		t.Fatal(err)
	}
	skill.Action = "remove"
	if _, err := f.service.PreviewCodexResource(f.repo, skill); err == nil {
		t.Fatal("manual replacement could be removed")
	}

	config := filepath.Join(f.repo, ".codex", "config.toml")
	if err := os.Mkdir(filepath.Dir(config), 0700); err != nil {
		t.Fatal(err)
	}
	const base = "# user settings\nmodel = 'example'\n\n[mcp_servers.existing]\ncommand = 'old'\n"
	reviewWrite(t, config, base)
	first := workspace.CodexResourceChange{Kind: "mcp", Name: "docs", URL: "https://example.invalid/mcp", TokenEnv: "DOCS_TOKEN", Action: "add"}
	second := workspace.CodexResourceChange{Kind: "mcp", Name: "local", Command: "/usr/bin/true", Args: []string{"--example"}, Action: "add"}
	preview, err = f.service.PreviewCodexResource(f.repo, first)
	if err != nil || strings.Contains(preview.After, "user settings") {
		t.Fatalf("MCP preview: %+v %v", preview, err)
	}
	apply(second)
	if err := f.service.ApplyCodexResource(f.repo, first, preview.Version); err == nil {
		t.Fatal("stale project config preview applied")
	}
	apply(first)
	items, err := f.service.ProjectCodexResources(f.repo)
	if err != nil || items.Issue != "" || len(items.Items) != 3 {
		t.Fatalf("placements: %+v %v", items, err)
	}
	data, err := os.ReadFile(config)
	if err != nil {
		t.Fatal(err)
	}
	reviewWrite(t, config, string(data)+"\n[features]\nexample = true\n")
	first.Action, second.Action = "remove", "remove"
	apply(first)
	apply(second)
	data, err = os.ReadFile(config)
	if err != nil || string(data) != base+"\n[features]\nexample = true\n" {
		t.Fatalf("withdraw overwrote unrelated config: %q %v", data, err)
	}
	first.Action = "add"
	apply(first)
	data, err = os.ReadFile(config)
	if err != nil {
		t.Fatal(err)
	}
	reviewWrite(t, config, strings.Replace(string(data), "https://example.invalid/mcp", "https://example.invalid/manual", 1))
	first.Action = "remove"
	if _, err := f.service.PreviewCodexResource(f.repo, first); err == nil {
		t.Fatal("externally changed MCP could be removed")
	}
}
