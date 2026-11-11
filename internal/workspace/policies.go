package workspace

import (
	"crypto/rand"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"
)

// PolicyRule is a declarative restriction; off keeps adoption explicit.
type PolicyRule struct {
	Cwd            string   `json:"cwd"`
	ID             string   `json:"id"`
	Level          string   `json:"level"`
	Scope          string   `json:"scope"`
	Include        []string `json:"include"`
	Exclude        []string `json:"exclude"`
	ExcludeMarkers []string `json:"excludeMarkers"`
}

type PolicyDefaults struct {
	Version string       `json:"version"`
	Rules   []PolicyRule `json:"rules"`
}

type EffectivePolicy struct {
	Rule    PolicyRule `json:"rule"`
	Origin  string     `json:"origin"`
	Applies bool       `json:"applies"`
}

type ProjectPolicies struct {
	Version         string            `json:"version"`
	ProjectVersion  string            `json:"projectVersion"`
	DefaultsVersion string            `json:"defaultsVersion"`
	Overrides       []PolicyRule      `json:"overrides"`
	Effective       []EffectivePolicy `json:"effective"`
}

func initialPolicies() []PolicyRule {
	return []PolicyRule{
		{ID: "no-unit-tests", Level: "off", Scope: "all", Include: []string{}, Exclude: []string{}, ExcludeMarkers: []string{}},
		{ID: "react-no-use-state", Level: "off", Scope: "react", Include: []string{}, Exclude: []string{}, ExcludeMarkers: []string{}},
	}
}

func policyDefaultsFile(registry string) string {
	return filepath.Join(filepath.Dir(registry), "policies", "defaults.json")
}

func readPolicyDefaults(registry string) (PolicyDefaults, error) {
	result := PolicyDefaults{Rules: initialPolicies()}
	data, err := os.ReadFile(policyDefaultsFile(registry))
	if errors.Is(err, os.ErrNotExist) {
		result.Version = digest("missing")
		return result, nil
	}
	if err != nil {
		return result, err
	}
	if len(data) > 256*1024 {
		return result, errors.New("shared rules configuration exceeds 256 KiB")
	}
	if err := json.Unmarshal(data, &result.Rules); err != nil {
		return result, fmt.Errorf("read shared rules: %w", err)
	}
	if result.Rules == nil {
		return result, errors.New("shared rules configuration must be an array")
	}
	if err := validatePolicies(result.Rules); err != nil {
		return result, err
	}
	result.Version = digest(string(data))
	return result, nil
}

func (s *Service) PolicyDefaults() (PolicyDefaults, error) { return readPolicyDefaults(s.path) }

func (s *Service) SavePolicyDefaults(version string, rules []PolicyRule) error {
	if rules == nil {
		rules = []PolicyRule{}
	}
	if err := validatePolicies(rules); err != nil {
		return err
	}
	s.syncMu.Lock()
	defer s.syncMu.Unlock()
	s.mu.Lock()
	defer s.mu.Unlock()
	current, err := readPolicyDefaults(s.path)
	if err != nil {
		return err
	}
	if current.Version != version {
		return errors.New("shared rules changed; reload before saving")
	}
	return saveJSON(policyDefaultsFile(s.path), rules)
}

func validatePolicies(rules []PolicyRule) error {
	if len(rules) > 2 {
		return errors.New("only unit test restrictions and React state management rules are supported")
	}
	ids := map[string]bool{}
	for i := range rules {
		r := &rules[i]
		if strings.ContainsAny(r.Cwd, "\r\n") || strings.Contains(r.Cwd, "<!-- repsync:") {
			return errors.New("rule working directory contains invalid characters")
		}
		if err := validateCommandOptions(r.Cwd, nil); err != nil {
			return err
		}
		if r.Include == nil {
			r.Include = []string{}
		}
		if r.Exclude == nil {
			r.Exclude = []string{}
		}
		if r.ExcludeMarkers == nil {
			r.ExcludeMarkers = []string{}
		}
		if !slices.Contains([]string{"no-unit-tests", "react-no-use-state"}, r.ID) || ids[r.ID] {
			return errors.New("invalid or duplicate rule IDs")
		}
		ids[r.ID] = true
		if !slices.Contains([]string{"off", "advice", "warn", "error"}, r.Level) || !slices.Contains([]string{"all", "react"}, r.Scope) {
			return errors.New("rules require valid enforcement and scope")
		}
		for _, patterns := range [][]string{r.Include, r.Exclude} {
			if len(patterns) > 32 {
				return errors.New("each rule allows up to 32 path patterns")
			}
			for _, pattern := range patterns {
				if strings.Contains(pattern, "<!-- repsync:") {
					return errors.New("rule paths cannot contain managed block markers")
				}
				if _, err := policyPattern(pattern); err != nil {
					return err
				}
			}
		}
		if len(r.ExcludeMarkers) > 16 {
			return errors.New("each rule allows up to 16 exception markers")
		}
		for _, marker := range r.ExcludeMarkers {
			if strings.TrimSpace(marker) == "" || len(marker) > 256 || strings.ContainsAny(marker, "\x00\r\n") || strings.Contains(marker, "<!-- repsync:") {
				return errors.New("exception markers must be single-line text of 1–256 bytes")
			}
		}
	}
	return nil
}

func projectPolicies(path, registry string) (ProjectPolicies, error) {
	defaults, err := readPolicyDefaults(registry)
	if err != nil {
		return ProjectPolicies{}, err
	}
	root, err := os.OpenRoot(path)
	if err != nil {
		return ProjectPolicies{}, err
	}
	defer root.Close()
	file, err := loadProjectConfigFile(root)
	if err != nil {
		return ProjectPolicies{}, err
	}
	result := ProjectPolicies{ProjectVersion: file.version, DefaultsVersion: defaults.Version, Overrides: []PolicyRule{}, Effective: []EffectivePolicy{}}
	if raw := file.fields["policies"]; len(raw) > 0 && string(raw) != "null" {
		if err := json.Unmarshal(raw, &result.Overrides); err != nil {
			return result, fmt.Errorf("read project rules: %w", err)
		}
		if err := validatePolicies(result.Overrides); err != nil {
			return result, err
		}
	}
	needReact := false
	for _, r := range initialPolicies() {
		for _, d := range defaults.Rules {
			if d.ID == r.ID {
				r = d
			}
		}
		for _, d := range result.Overrides {
			if d.ID == r.ID {
				r = d
			}
		}
		needReact = needReact || (r.Scope == "react" && r.Level != "off" && r.Cwd == "")
	}
	react := false
	data, readErr := readProjectConfig(root, "package.json")
	if needReact && readErr != nil && !errors.Is(readErr, os.ErrNotExist) {
		return result, fmt.Errorf("detect React project: %w", readErr)
	}
	if readErr == nil {
		var pkg struct{ Dependencies, DevDependencies, PeerDependencies map[string]json.RawMessage }
		if err := json.Unmarshal(data, &pkg); err != nil && needReact {
			return result, fmt.Errorf("detect React project: %w", err)
		}
		react = pkg.Dependencies["react"] != nil || pkg.DevDependencies["react"] != nil || pkg.PeerDependencies["react"] != nil
	}
	for _, rule := range initialPolicies() {
		origin := "default"
		for _, d := range defaults.Rules {
			if d.ID == rule.ID {
				rule = d
				break
			}
		}
		for _, override := range result.Overrides {
			if override.ID == rule.ID {
				rule = override
				origin = "project"
				break
			}
		}
		applies := rule.Scope == "all" || react
		if rule.Scope == "react" && rule.Cwd != "" {
			applies = false
			data, err := readProjectConfig(root, filepath.Join(rule.Cwd, "package.json"))
			if rule.Level != "off" && err != nil && !errors.Is(err, os.ErrNotExist) {
				return result, fmt.Errorf("detect React working directory: %w", err)
			}
			if err == nil {
				var pkg struct{ Dependencies, DevDependencies, PeerDependencies map[string]json.RawMessage }
				if err := json.Unmarshal(data, &pkg); err != nil && rule.Level != "off" {
					return result, err
				}
				applies = pkg.Dependencies["react"] != nil || pkg.DevDependencies["react"] != nil || pkg.PeerDependencies["react"] != nil
			}
		}
		result.Effective = append(result.Effective, EffectivePolicy{Rule: rule, Origin: origin, Applies: applies})
	}
	encoded, _ := json.Marshal(result)
	result.Version = digest(string(encoded))
	return result, nil
}

func (s *Service) ProjectPolicies(path string) (ProjectPolicies, error) {
	if _, err := s.registeredProject(path); err != nil {
		return ProjectPolicies{}, err
	}
	return projectPolicies(path, s.path)
}

func (s *Service) SaveProjectPolicies(path, version string, rules []PolicyRule) error {
	if err := validatePolicies(rules); err != nil {
		return err
	}
	if rules == nil {
		rules = []PolicyRule{}
	}
	s.syncMu.Lock()
	defer s.syncMu.Unlock()
	return s.saveProjectSection(path, version, "policies", false, rules)
}

const policyStart = "<!-- repsync:policies:start -->"
const policyEnd = "<!-- repsync:policies:end -->"

func policyInstructions(config ProjectPolicies) string {
	lines := []string{"## Project rules (repsync)", ""}
	for _, entry := range config.Effective {
		r := entry.Rule
		if r.Level == "off" || !entry.Applies {
			continue
		}
		title := "Disallow new unit tests"
		if r.ID == "react-no-use-state" {
			title = "React: do not import or use useState; use useReducer for state management"
		}
		level := map[string]string{"advice": "AI guidance", "warn": "Code check warning", "error": "New violations fail code checks"}[r.Level]
		lines = append(lines, "- "+title+"（"+level+"）。")
		if r.ID == "no-unit-tests" {
			if len(r.Include) == 0 {
				lines = append(lines, "  - Unit test paths are not configured. Declare paths and integration test exceptions before code checks; do not infer test semantics from file names.")
			} else {
				lines = append(lines, "  - Unit test paths: "+strings.Join(r.Include, ", ")+".")
			}
		}
		if r.Cwd != "" {
			lines = append(lines, "  - Working directory: "+r.Cwd+".")
		}
		if len(r.Exclude) > 0 {
			lines = append(lines, "  - Path exceptions: "+strings.Join(r.Exclude, ", ")+".")
		}
		if len(r.ExcludeMarkers) > 0 {
			lines = append(lines, "  - File exception markers (whole line): "+strings.Join(r.ExcludeMarkers, ", ")+".")
		}
	}
	if len(lines) == 2 {
		return ""
	}
	lines = append(lines, "", "Reuse existing project checks; the standalone entry is `repsync check-rules .`.")
	return strings.Join(lines, "\n") + "\n"
}

func (s *Service) PreviewPolicyGuidance(path string) (UpdatePreview, error) {
	project, err := s.registeredProject(path)
	if err != nil {
		return UpdatePreview{}, err
	}
	s.syncMu.Lock()
	defer s.syncMu.Unlock()
	config, err := s.ProjectPolicies(path)
	if err != nil {
		return UpdatePreview{}, err
	}
	before, err := readTarget(path)
	if err != nil {
		return UpdatePreview{}, err
	}
	if strings.Count(before.Content, policyStart) != strings.Count(before.Content, policyEnd) || strings.Count(before.Content, policyStart) > 1 {
		return UpdatePreview{}, errors.New("project rule block markers are incomplete or duplicated")
	}
	if start := strings.Index(before.Content, policyStart); start >= 0 {
		sharedA, sharedB := strings.Index(before.Content, sharedStart), strings.Index(before.Content, sharedEnd)
		if sharedA >= 0 && start > sharedA && start < sharedB {
			return UpdatePreview{}, errors.New("project rule blocks cannot be nested in shared rules blocks")
		}
	}
	instructions := policyInstructions(config)
	block := ""
	after := before.Content
	if instructions != "" {
		block = policyStart + "\n" + instructions + policyEnd
		after = block + "\n\n" + before.Content
	}
	if start := strings.Index(before.Content, policyStart); start >= 0 {
		end := strings.Index(before.Content, policyEnd)
		if end < start {
			return UpdatePreview{}, errors.New("project rule block markers are out of order")
		}
		after = before.Content[:start] + block + before.Content[end+len(policyEnd):]
	}
	if len(after) > 256*1024 {
		return UpdatePreview{}, errors.New("updated AGENTS.md exceeds 256 KiB")
	}
	diff, err := documentDiff(before.Content, after)
	if err != nil {
		return UpdatePreview{}, err
	}
	now := time.Now()
	for token, plan := range s.previews {
		if now.After(plan.Expires) {
			delete(s.previews, token)
		}
	}
	if s.previews == nil {
		s.previews = map[string]pendingUpdate{}
	}
	if len(s.previews) >= 32 {
		return UpdatePreview{}, errors.New("too many pending previews; try again later")
	}
	token := rand.Text()
	record := updateRecord{UpdateHistory: UpdateHistory{ID: token, Project: project, SourcePath: "Project rules", Version: config.Version, CreatedAt: now.UTC().Format(time.RFC3339Nano), State: "prepared"}, Before: before, After: fileSnapshot{Content: after, Exists: before.Exists || after != "", Mode: before.Mode}, PolicyVersion: config.Version}
	s.previews[token] = pendingUpdate{Record: record, Expires: now.Add(15 * time.Minute)}
	return UpdatePreview{Token: token, Project: project, SourcePath: "Project rules", Version: config.Version, Diff: diff, Changed: !sameSnapshot(before, record.After)}, nil
}
