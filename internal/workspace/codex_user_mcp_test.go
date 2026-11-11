//go:build integration

package workspace

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestCodexUserMCPDefaultPreservesConfiguration(t *testing.T) {
	for _, tt := range []struct {
		name, original, expected string
	}{
		{"insert after table", "[mcp_servers.docs] # settings\r\nurl = 'https://example.invalid'\r\n", "[mcp_servers.docs] # settings\r\nenabled = false\r\nurl = 'https://example.invalid'\r\n"},
		{"replace existing flag", "[mcp_servers.'docs']\nenabled = true # keep this\ncommand = 'tool'\n", "[mcp_servers.'docs']\nenabled = false # keep this\ncommand = 'tool'\n"},
		{"inline server", "mcp_servers = { docs = { url = 'https://example.invalid' } }\n", "mcp_servers = { docs = {enabled = false,  url = 'https://example.invalid' } }\n"},
		{"dotted key", "mcp_servers.docs.command = 'tool'\n[profiles.work]\nmodel = 'example'\n", "mcp_servers.docs.command = 'tool'\nmcp_servers.docs.enabled = false\n[profiles.work]\nmodel = 'example'\n"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			directory := t.TempDir()
			file := filepath.Join(directory, "config.toml")
			if err := os.WriteFile(file, []byte(tt.original), 0600); err != nil {
				t.Fatal(err)
			}
			change := CodexResourceChange{Kind: "mcp", Name: "docs", Action: "disable"}
			plan, err := planCodexUserMCP(directory, change)
			if err != nil {
				t.Fatal(err)
			}
			if got, err := os.ReadFile(file); err != nil || string(got) != tt.original {
				t.Fatalf("preview changed config: %q %v", got, err)
			}
			if strings.Contains(plan.preview.After, "example.invalid") {
				t.Fatal("preview included connection settings")
			}
			if err := replaceUpdateTarget(directory, "config.toml", plan.before, plan.after); err != nil {
				t.Fatal(err)
			}
			if got, err := os.ReadFile(file); err != nil || string(got) != tt.expected {
				t.Fatalf("unexpected config: %q %v", got, err)
			}
		})
	}
}

func TestCodexProjectOverrideSwitchAndInherit(t *testing.T) {
	user, project := t.TempDir(), t.TempDir()
	global := "[mcp_servers.docs]\nurl = 'https://example.invalid'\n"
	if err := os.WriteFile(filepath.Join(user, "config.toml"), []byte(global), 0600); err != nil {
		t.Fatal(err)
	}
	root, err := os.OpenRoot(project)
	if err != nil {
		t.Fatal(err)
	}
	defer root.Close()
	if err := root.Mkdir(".codex", 0700); err != nil {
		t.Fatal(err)
	}
	for _, action := range []string{"disable", "enable", "remove"} {
		plan, err := planCodexMCP(project, root, user, CodexResourceChange{Kind: "mcp", Name: "docs", Action: action})
		if err != nil {
			t.Fatalf("%s: %v", action, err)
		}
		if err := replaceUpdateTarget(project, codexProjectConfig, plan.before, plan.after); err != nil {
			t.Fatalf("%s: %v", action, err)
		}
		entries, err := codexMCPEntries(plan.after.Content)
		if err != nil {
			t.Fatal(err)
		}
		entry, present := entries["docs"].(map[string]any)
		if action == "remove" {
			if present {
				t.Fatal("project override remains after restoring inheritance")
			}
		} else if !present || len(entry) != 1 || entry["enabled"] != (action == "enable") {
			t.Fatalf("project override did not contain only the desired enabled flag: %v", entry)
		}
	}
	if got, err := os.ReadFile(filepath.Join(user, "config.toml")); err != nil || string(got) != global {
		t.Fatalf("project operation changed the user configuration: %q %v", got, err)
	}
}
