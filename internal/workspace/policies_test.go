//go:build integration

package workspace_test

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"repsync/internal/workspace"
)

func policyFixture(t *testing.T) (*workspace.Service, string) {
	t.Helper()
	s, path := configFixture(t, `{"custom":{"keep":true}}`)
	cmd := exec.Command("git", "-C", path, "init", "-b", "main")
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git init: %v %s", err, out)
	}
	return s, path
}
func policyWrite(t *testing.T, path, name, content string) {
	t.Helper()
	file := filepath.Join(path, name)
	if err := os.MkdirAll(filepath.Dir(file), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(file, []byte(content), 0600); err != nil {
		t.Fatal(err)
	}
}
func policyCommit(t *testing.T, path string) {
	t.Helper()
	for _, args := range [][]string{{"add", "."}, {"-c", "user.name=Policy Test", "-c", "user.email=policy@example.invalid", "commit", "-m", "baseline"}} {
		cmd := exec.Command("git", append([]string{"-C", path}, args...)...)
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git commit: %v %s", err, out)
		}
	}
}
func savePolicies(t *testing.T, s *workspace.Service, path string, rules []workspace.PolicyRule) {
	t.Helper()
	config, err := s.ProjectPolicies(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := s.SaveProjectPolicies(path, config.ProjectVersion, rules); err != nil {
		t.Fatal(err)
	}
}

func TestPolicyInheritanceAndSaveProtection(t *testing.T) {
	t.Parallel()
	s, path := policyFixture(t)
	defaults, err := s.PolicyDefaults()
	if err != nil {
		t.Fatal(err)
	}
	for _, r := range defaults.Rules {
		if r.Level != "off" {
			t.Fatal("restriction enabled without adoption")
		}
	}
	rule := workspace.PolicyRule{ID: "no-unit-tests", Level: "error", Scope: "all", Include: []string{"**/*.unit.test.ts"}, Exclude: []string{}, ExcludeMarkers: []string{}}
	if err := s.SavePolicyDefaults(defaults.Version, []workspace.PolicyRule{rule}); err != nil {
		t.Fatal(err)
	}
	if err := s.SavePolicyDefaults(defaults.Version, []workspace.PolicyRule{}); err == nil {
		t.Fatal("stale defaults overwritten")
	}
	config, err := s.ProjectPolicies(path)
	if err != nil || config.Effective[0].Rule.Level != "error" || config.Effective[0].Origin != "default" {
		t.Fatalf("inheritance: %+v %v", config, err)
	}
	rule.Level = "off"
	savePolicies(t, s, path, []workspace.PolicyRule{rule})
	effective, err := s.ProjectPolicies(path)
	if err != nil || effective.Effective[0].Rule.Level != "off" || effective.Effective[0].Origin != "project" {
		t.Fatalf("override: %+v %v", effective, err)
	}
	if err := s.SaveProjectPolicies(path, config.ProjectVersion, nil); err == nil {
		t.Fatal("stale project config overwritten")
	}
	text, _ := os.ReadFile(filepath.Join(path, ".repsync.json"))
	if !strings.Contains(string(text), "custom") {
		t.Fatal("lost unrelated configuration")
	}
}

func TestPolicyGuidanceUsesProtectedPreviewAndRestore(t *testing.T) {
	t.Parallel()
	s, path := policyFixture(t)
	original := "# Project\r\nKeep local overrides.\r\n"
	policyWrite(t, path, "AGENTS.md", original)
	rules := []workspace.PolicyRule{{ID: "react-no-use-state", Level: "error", Scope: "all", Include: []string{}, Exclude: []string{}, ExcludeMarkers: []string{}}}
	savePolicies(t, s, path, rules)
	plan, err := s.PreviewPolicyGuidance(path)
	if err != nil || !strings.Contains(plan.Diff, "useReducer") {
		t.Fatalf("preview: %+v %v", plan, err)
	}
	rules[0].Level = "warn"
	savePolicies(t, s, path, rules)
	if _, err := s.ApplyUpdate(plan.Token); err == nil {
		t.Fatal("stale rule guidance applied")
	}
	plan, err = s.PreviewPolicyGuidance(path)
	if err != nil {
		t.Fatal(err)
	}
	record, err := s.ApplyUpdate(plan.Token)
	if err != nil {
		t.Fatal(err)
	}
	content, _ := os.ReadFile(filepath.Join(path, "AGENTS.md"))
	if !strings.HasSuffix(string(content), original) {
		t.Fatal("project overrides changed")
	}
	if _, err := s.RestoreUpdate(record.ID); err != nil {
		t.Fatal(err)
	}
	restored, _ := os.ReadFile(filepath.Join(path, "AGENTS.md"))
	if string(restored) != original {
		t.Fatal("restore did not preserve original bytes")
	}
}

func TestPolicyUnitTestPathsAndIntegrationExceptions(t *testing.T) {
	t.Parallel()
	s, path := policyFixture(t)
	policyWrite(t, path, "old.unit.test.ts", "test('existing', () => {})\n")
	policyCommit(t, path)
	policyWrite(t, path, "new.unit.test.ts", "test('new', () => {})\n")
	policyWrite(t, path, "new.integration.test.ts", "test('integration', () => {})\n")
	policyWrite(t, path, "server_test.go", "//go:build integration\npackage server\n")
	savePolicies(t, s, path, []workspace.PolicyRule{{ID: "no-unit-tests", Level: "error", Scope: "all", Include: []string{"**/*.test.ts", "**/*_test.go"}, Exclude: []string{"**/*.integration.test.ts"}, ExcludeMarkers: []string{"//go:build integration"}}})
	report, err := s.CheckPolicies(path)
	if err != nil {
		t.Fatal(err)
	}
	if report.Passed || !report.Complete || len(report.Findings) != 2 {
		t.Fatalf("test classification: %+v", report)
	}
	kinds := map[string]string{}
	for _, f := range report.Findings {
		kinds[f.Path] = f.Kind
	}
	if kinds["old.unit.test.ts"] != "historical" || kinds["new.unit.test.ts"] != "new" {
		t.Fatalf("incorrect baseline: %v", kinds)
	}
	policyWrite(t, path, "old.unit.test.ts", "test('existing', () => {})\n\ntest('additional', () => {})\n")
	report, err = s.CheckPolicies(path)
	if err != nil {
		t.Fatal(err)
	}
	for _, f := range report.Findings {
		if f.Path == "old.unit.test.ts" && (f.Kind != "new" || f.Line < 2) {
			t.Fatalf("new code in existing unit test not detected: %+v", f)
		}
	}
}

func TestPolicyMissingClassificationIsNotSuccess(t *testing.T) {
	t.Parallel()
	s, path := policyFixture(t)
	savePolicies(t, s, path, []workspace.PolicyRule{{ID: "no-unit-tests", Level: "error", Scope: "all", Include: []string{}, Exclude: []string{}, ExcludeMarkers: []string{}}})
	report, err := s.CheckPolicies(path)
	if err != nil {
		t.Fatal(err)
	}
	if report.Passed || report.Complete || len(report.Errors) == 0 {
		t.Fatalf("unconfigured test classification passed: %+v", report)
	}
}

func TestPolicyESLintASTAndHistoricalFindings(t *testing.T) {
	t.Parallel()
	s, path := policyFixture(t)
	cwd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	modules := filepath.Join(cwd, "..", "..", "frontend", "node_modules")
	if _, err := os.Stat(filepath.Join(modules, "eslint", "package.json")); err != nil {
		t.Fatal("install frontend dependencies for the ESLint integration test")
	}
	if err := os.Symlink(modules, filepath.Join(path, "node_modules")); err != nil {
		t.Fatal(err)
	}
	policyWrite(t, path, ".gitignore", "node_modules\n")
	policyWrite(t, path, "package.json", `{"dependencies":{"react":"19"}}`)
	policyWrite(t, path, "eslint.config.cjs", `const parser=require('typescript-eslint').parser; module.exports=[{ignores:['Ignored.tsx']},{files:['**/*.{ts,tsx}'],languageOptions:{parser,parserOptions:{ecmaFeatures:{jsx:true}}}}];`)
	original := "import {useState as state} from 'react';\nconst old = state(0);\n\n// useState in comments is allowed\nconst note = 'useState';\nfunction local(useState: (n:number)=>number) { return useState(1); }\n"
	policyWrite(t, path, "App.tsx", original)
	policyCommit(t, path)
	policyWrite(t, path, "App.tsx", original+"\nconst added = state(2);\n")
	policyWrite(t, path, "Other.tsx", "import R from 'react';\nconst x=R['useState'](1);\nconst {useState: useValue}=R;\nconst y=useValue(2);\nconst text='useState';\n")
	policyWrite(t, path, "Ignored.tsx", "import {useState} from 'react';\n")
	savePolicies(t, s, path, []workspace.PolicyRule{{ID: "react-no-use-state", Level: "error", Scope: "react", Include: []string{}, Exclude: []string{}, ExcludeMarkers: []string{}}})
	report, err := s.CheckPolicies(path)
	if err != nil {
		t.Fatal(err)
	}
	if report.Passed || !report.Complete {
		t.Fatalf("AST check: %+v", report)
	}
	old, newCount := 0, 0
	for _, f := range report.Findings {
		if f.Path == "App.tsx" && f.Line == 8 && f.Kind == "new" {
			newCount++
		}
		if f.Path == "App.tsx" && f.Kind == "historical" {
			old++
		}
		if f.Path == "App.tsx" && (f.Line == 4 || f.Line == 5 || f.Line == 6) {
			t.Fatalf("false positive in comment, string or shadowed binding: %+v", f)
		}
	}
	if old != 2 || newCount != 1 {
		t.Fatalf("historical/new findings: %+v", report)
	}
	if len(report.Skipped) != 1 || report.Skipped[0] != "Ignored.tsx" {
		t.Fatalf("ignored files not disclosed: %+v", report)
	}
	other := 0
	for _, f := range report.Findings {
		if f.Path == "Other.tsx" {
			other++
		}
	}
	if other < 3 {
		t.Fatalf("namespace and destructuring aliases missed: %+v", report)
	}
	policyWrite(t, path, "frontend/package.json", `{"dependencies":{"react":"19"}}`)
	policyWrite(t, path, "frontend/eslint.config.cjs", `const parser=require('typescript-eslint').parser;module.exports=[{files:['**/*.tsx'],languageOptions:{parser}}];`)
	policyWrite(t, path, "frontend/App.tsx", "import {useState} from 'react';\n")
	savePolicies(t, s, path, []workspace.PolicyRule{{ID: "react-no-use-state", Cwd: "frontend", Level: "error", Scope: "react", Include: []string{"frontend/**"}}})
	report, err = s.CheckPolicies(path)
	if err != nil || !report.Complete || report.Passed || len(report.Findings) != 1 || report.Findings[0].Path != "frontend/App.tsx" {
		t.Fatalf("frontend ESLint directory: %+v %v", report, err)
	}
}

func TestPolicyCIGateStopsBeforeExistingCommands(t *testing.T) {
	t.Parallel()
	s, path, config := pipelineFixture(t, [][]string{{"/bin/sh", "-c", "touch must-not-run"}}, 20)
	policyWrite(t, path, "new.unit.test.ts", "test('new',()=>{})\n")
	savePolicies(t, s, path, []workspace.PolicyRule{{ID: "no-unit-tests", Level: "error", Scope: "all", Include: []string{"**/*.unit.test.ts"}, Exclude: []string{}, ExcludeMarkers: []string{}}})
	config, err := s.Pipelines(path)
	if err != nil {
		t.Fatal(err)
	}
	run, err := s.StartPipeline(path, "ci", config.Version)
	if err != nil {
		t.Fatal(err)
	}
	output := waitPipeline(t, s, path, run.ID)
	if output.Run.Status != "failure" || !strings.Contains(output.Log, "项目规则检查失败") {
		t.Fatalf("gate result: %+v", output)
	}
	if _, err := os.Stat(filepath.Join(path, "must-not-run")); !os.IsNotExist(err) {
		t.Fatal("CI commands ran after rule failure")
	}
}

func TestPolicyDisabledGuidanceDoesNotCreateFile(t *testing.T) {
	t.Parallel()
	s, path := policyFixture(t)
	preview, err := s.PreviewPolicyGuidance(path)
	if err != nil || preview.Changed {
		t.Fatalf("disabled rules created instructions: %+v %v", preview, err)
	}
	if _, err := os.Stat(filepath.Join(path, "AGENTS.md")); !os.IsNotExist(err) {
		t.Fatal("preview created a file")
	}
}

func TestPolicyNestedProjectBaseline(t *testing.T) {
	t.Parallel()
	s, path := policyFixture(t)
	policyWrite(t, path, "frontend/old.unit.test.ts", "test('old',()=>{})\n")
	policyCommit(t, path)
	nested := filepath.Join(path, "frontend")
	project, err := s.Add(nested)
	if err != nil {
		t.Fatal(err)
	}
	savePolicies(t, s, project.Path, []workspace.PolicyRule{{ID: "no-unit-tests", Level: "error", Scope: "all", Include: []string{"**/*.unit.test.ts"}}})
	report, err := s.CheckPolicies(project.Path)
	if err != nil || !report.Complete || !report.Passed || len(report.Findings) != 1 || report.Findings[0].Kind != "historical" {
		t.Fatalf("subdirectory baseline: %+v %v", report, err)
	}
}

func TestPolicyMissingESLintReportsIncomplete(t *testing.T) {
	t.Parallel()
	s, path := policyFixture(t)
	policyWrite(t, path, "package.json", `{"dependencies":{"react":"19"}}`)
	policyWrite(t, path, "App.tsx", "import {useState} from 'react';\n")
	savePolicies(t, s, path, []workspace.PolicyRule{{ID: "react-no-use-state", Level: "error", Scope: "react"}})
	report, err := s.CheckPolicies(path)
	if err != nil || report.Passed || report.Complete || len(report.Errors) == 0 {
		t.Fatalf("missing ESLint passed: %+v %v", report, err)
	}
	rule := workspace.PolicyRule{ID: "react-no-use-state", Level: "warn", Scope: "react"}
	savePolicies(t, s, path, []workspace.PolicyRule{rule})
	report, err = s.CheckPolicies(path)
	if err != nil || !report.Passed || report.Complete {
		t.Fatalf("warning level should retain diagnostics and permit continuing: %+v %v", report, err)
	}
	savePolicies(t, s, path, []workspace.PolicyRule{rule, {ID: "no-unit-tests", Level: "error", Scope: "all", Include: []string{"**/*.unit.test.ts"}}})
	report, err = s.CheckPolicies(path)
	if err != nil || !report.Passed || report.Complete {
		t.Fatalf("warning diagnostics incorrectly blocked an unrelated error-level rule: %+v %v", report, err)
	}
}

func TestPolicyCheckShutdownCancelsESLint(t *testing.T) {
	t.Parallel()
	s, path := policyFixture(t)
	cwd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(filepath.Join(cwd, "..", "..", "frontend", "node_modules"), filepath.Join(path, "node_modules")); err != nil {
		t.Fatal(err)
	}
	policyWrite(t, path, ".gitignore", "node_modules\n")
	policyWrite(t, path, "package.json", `{"dependencies":{"react":"19"}}`)
	policyWrite(t, path, "App.js", "import {useState} from 'react';\n")
	policyWrite(t, path, "eslint.config.mjs", `import fs from 'node:fs';fs.writeFileSync(new URL('./ready',import.meta.url),'ready');await new Promise(resolve=>setTimeout(resolve,30000));export default [];`)
	savePolicies(t, s, path, []workspace.PolicyRule{{ID: "react-no-use-state", Level: "error", Scope: "react"}})
	done := make(chan error, 1)
	go func() {
		report, err := s.CheckPolicies(path)
		if err == nil && report.Complete {
			done <- fmt.Errorf("cancelled check was reported complete")
			return
		}
		done <- err
	}()
	deadline := time.Now().Add(5 * time.Second)
	for {
		if _, err := os.Stat(filepath.Join(path, "ready")); err == nil {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("ESLint never entered the cancellable config")
		}
		time.Sleep(10 * time.Millisecond)
	}
	stopped := make(chan struct{})
	go func() { s.Shutdown(); close(stopped) }()
	select {
	case <-stopped:
	case <-time.After(3 * time.Second):
		t.Fatal("shutdown did not cancel the rule checker")
	}
	if err := <-done; err != nil {
		t.Fatal(err)
	}
	if _, err := s.CheckPolicies(path); err == nil {
		t.Fatal("check accepted after shutdown")
	}
}
