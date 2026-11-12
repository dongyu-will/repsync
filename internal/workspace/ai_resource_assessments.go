package workspace

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"
	"unicode/utf8"
)

type AISkillCandidate struct {
	Name                string `json:"name"`
	Source              string `json:"source"`
	SourceVersion       string `json:"sourceVersion"`
	Description         string `json:"description"`
	InstalledResourceID string `json:"installedResourceId,omitempty"`
}

type AIResourceFinding struct {
	Kind           string   `json:"kind"`
	Resource       string   `json:"resource"`
	Evidence       string   `json:"evidence"`
	Recommendation string   `json:"recommendation"`
	Sources        []string `json:"sources"`
}

// AIResourceAssessmentData describes an AI's scoped recommendation, not guaranteed compatibility.
type AIResourceAssessmentData struct {
	ResourceID       string              `json:"resourceId"`
	ResourceRevision string              `json:"resourceRevision"`
	ContextVersion   string              `json:"contextVersion"`
	Candidate        *AISkillCandidate   `json:"candidate,omitempty"`
	Verdict          string              `json:"verdict"`
	Summary          string              `json:"summary"`
	Fit              string              `json:"fit"`
	AdoptionCost     string              `json:"adoptionCost"`
	Coverage         string              `json:"coverage"`
	Findings         []AIResourceFinding `json:"findings"`
	Sources          []string            `json:"sources"`
	CheckedAt        string              `json:"checkedAt"`
}

type AIResourceAssessmentInput struct {
	AIResourceAssessmentData
	ExpectedVersion string `json:"expectedVersion"`
}

type AIResourceAssessment struct {
	AIResourceAssessmentData
	Scope      string `json:"scope"`
	RecordedAt string `json:"recordedAt"`
	Version    string `json:"version"`
	Stale      bool   `json:"stale"`
}

func (s *Service) aiResourceAssessmentPath(scope string) string {
	return filepath.Join(filepath.Dir(s.path), "ai-resource-assessments", gitDigest(scope)+".json")
}

func (s *Service) readAIResourceAssessments(scope string) ([]AIResourceAssessment, error) {
	items := []AIResourceAssessment{}
	data, err := readReviewData(s.aiResourceAssessmentPath(scope))
	if errors.Is(err, os.ErrNotExist) {
		return items, nil
	}
	if err != nil {
		return nil, err
	}
	if err := json.Unmarshal(data, &items); err != nil || items == nil || len(items) > 128 {
		return nil, errors.New("cannot read AI resource assessments")
	}
	seen := map[string]bool{}
	for i := range items {
		item := &items[i]
		if item.Scope != scope || seen[item.ResourceID] {
			return nil, errors.New("AI resource assessment identity does not match")
		}
		if err := validateAIResourceAssessment(item.AIResourceAssessmentData); err != nil {
			return nil, err
		}
		seen[item.ResourceID] = true
		item.Version, item.Stale = "", false
		item.Version = gitDigest(item)
	}
	return items, nil
}

func assessmentContext(report AIResourceReport, projects []Project, paths aiPaths) string {
	entries := []string{}
	for _, resource := range report.Resources {
		entries = append(entries, resource.ID+":"+resource.Revision)
	}
	for _, project := range projects {
		root, err := os.OpenRoot(project.Path)
		if err != nil {
			entries = append(entries, project.Path+":"+err.Error())
			continue
		}
		for _, name := range []string{
			"AGENTS.md", "README.md", "PRODUCT.md", "DESIGN.md", ".repsync.json", ".codex/config.toml", "package.json", "go.mod",
		} {
			data, err := readProjectConfig(root, name)
			value := textVersion(string(data))
			if err != nil {
				value = err.Error()
			}
			entries = append(entries, project.Path+"/"+name+":"+value)
		}
		root.Close()
	}
	for _, name := range []string{"AGENTS.md", "config.toml"} {
		path := filepath.Join(paths.codex, name)
		data, err := readText(path)
		value := textVersion(data)
		if err != nil {
			value = err.Error()
		}
		entries = append(entries, path+":"+value)
	}
	entries = append(entries, report.Issues...)
	slices.Sort(entries)
	return gitDigest([]any{report.Scope, entries})
}

func (s *Service) attachAIResourceAssessments(report *AIResourceReport, projects []Project, paths aiPaths) {
	report.ContextVersion = assessmentContext(*report, projects, paths)
	items, err := s.readAIResourceAssessments(report.Scope)
	if err != nil {
		report.Issues = append(report.Issues, "AI resource assessments: "+err.Error())
		return
	}
	if report.Scope != "" {
		global, err := s.readAIResourceAssessments("")
		if err != nil {
			report.Issues = append(report.Issues, "Global AI resource assessments: "+err.Error())
		} else {
			for _, item := range global {
				if !slices.ContainsFunc(items, func(local AIResourceAssessment) bool { return local.ResourceID == item.ResourceID }) {
					items = append(items, item)
				}
			}
		}
	}
	attached := map[string]bool{}
	for _, item := range items {
		target := item.ResourceID
		if item.Candidate != nil {
			target = item.Candidate.InstalledResourceID
		}
		index := slices.IndexFunc(report.Resources, func(resource AIResource) bool { return resource.ID == target })
		if index < 0 && item.Candidate == nil {
			continue
		}
		if index < 0 {
			candidate := item.Candidate
			report.Resources = append(report.Resources, AIResource{
				ID: item.ResourceID, Kind: "skill", Name: candidate.Name, Description: candidate.Description,
				Source: "Candidate Skill", Location: candidate.Source, Version: candidate.SourceVersion,
				Revision: gitDigest(candidate), Availability: "candidate", Applicability: "candidate",
				Detail: "Candidate source; installation not verified", Projects: []string{},
			})
			index = len(report.Resources) - 1
		}
		resource := &report.Resources[index]
		if attached[resource.ID] {
			if item.Scope == report.Scope {
				report.Issues = append(report.Issues, "Multiple assessments refer to the same installed resource: "+resource.Name)
			}
			continue
		}
		attached[resource.ID] = true
		item.Stale = item.Scope != report.Scope || item.ContextVersion != report.ContextVersion ||
			item.ResourceRevision != resource.Revision
		resource.Assessment = &item
	}
}

func validateAIResourceAssessment(data AIResourceAssessmentData) error {
	if data.ResourceID == "" || data.ResourceRevision == "" || data.ContextVersion == "" {
		return errors.New("provide resourceId, resourceRevision and contextVersion")
	}
	if !slices.Contains([]string{"recommended", "conditional", "not_recommended", "unknown"}, data.Verdict) {
		return errors.New("verdict must be recommended, conditional, not_recommended or unknown")
	}
	checked, err := time.Parse(time.RFC3339, data.CheckedAt)
	if err != nil || checked.After(time.Now().Add(5*time.Minute)) {
		return errors.New("checkedAt must be an RFC3339 time, not in the future")
	}
	for _, value := range []string{data.Summary, data.Fit, data.AdoptionCost, data.Coverage} {
		if strings.TrimSpace(value) == "" {
			return errors.New("provide summary, fit, adoptionCost and coverage, including any unknowns")
		}
	}
	if data.Candidate != nil {
		candidate := data.Candidate
		parsed, err := url.Parse(candidate.Source)
		remote := err == nil && (parsed.Scheme == "https" || parsed.Scheme == "http") && parsed.Host != "" && parsed.User == nil
		if candidate.Name == "" || len(candidate.Name) > 200 || (!remote && !filepath.IsAbs(candidate.Source)) {
			return errors.New("candidate requires a name and an HTTP(S) URL without credentials or an absolute source path")
		}
		if len(candidate.Source) > 2048 || len(candidate.SourceVersion) > 512 || len(candidate.Description) > 2000 {
			return errors.New("candidate source, version or description exceeds its size limit")
		}
		if data.ResourceID != aiID("candidate:skill", candidate.Source) {
			return errors.New("candidate resourceId does not match its source")
		}
		if candidate.SourceVersion == "" && data.Verdict != "unknown" {
			return errors.New("a known verdict requires the checked candidate source version or digest")
		}
	}
	if len(data.Sources) > 16 || len(data.Findings) > 32 || data.Findings == nil || data.Sources == nil {
		return errors.New("provide findings and sources arrays, with at most 32 findings and 16 sources")
	}
	if data.Verdict != "unknown" && len(data.Sources) == 0 {
		return errors.New("a known verdict requires original sources")
	}
	texts := []string{data.ResourceID, data.ResourceRevision, data.ContextVersion,
		data.Summary, data.Fit, data.AdoptionCost, data.Coverage, data.CheckedAt}
	if data.Candidate != nil {
		candidate := data.Candidate
		texts = append(texts, candidate.Name, candidate.Source, candidate.SourceVersion,
			candidate.Description, candidate.InstalledResourceID)
	}
	sources := slices.Clone(data.Sources)
	for _, finding := range data.Findings {
		if !slices.Contains([]string{"overlap", "conflict", "prerequisite", "unknown"}, finding.Kind) ||
			strings.TrimSpace(finding.Evidence) == "" || strings.TrimSpace(finding.Recommendation) == "" {
			return errors.New("each finding requires a kind, evidence and recommendation")
		}
		uniqueSources := map[string]bool{}
		for _, source := range finding.Sources {
			uniqueSources[strings.TrimSpace(source)] = true
		}
		if finding.Kind == "conflict" && len(uniqueSources) < 2 {
			return errors.New("conflict findings require sources for both conflicting requirements")
		}
		if data.Verdict == "recommended" && finding.Kind != "overlap" {
			return errors.New("unresolved conflicts, prerequisites or unknowns require a conditional or uncertain verdict")
		}
		if len(finding.Sources) > 16 {
			return errors.New("at most 16 sources per finding allowed")
		}
		sources = append(sources, finding.Sources...)
		texts = append(texts, finding.Kind, finding.Resource, finding.Evidence, finding.Recommendation)
	}
	for _, source := range sources {
		if strings.TrimSpace(source) == "" || len(source) > 2048 {
			return errors.New("assessment sources must be nonempty and at most 2048 bytes")
		}
	}
	texts = append(texts, sources...)
	for _, text := range texts {
		if !utf8.ValidString(text) || strings.ContainsRune(text, '\x00') {
			return errors.New("assessment text must be UTF-8 without NUL")
		}
	}
	encoded, err := json.Marshal(data)
	if err != nil || len(encoded) > 64*1024 {
		return errors.New("assessment must be UTF-8 without NUL and at most 64 KiB")
	}
	return nil
}

// RecordAIResourceAssessment saves a candidate or installed-resource assessment without installing anything.
func (s *Service) RecordAIResourceAssessment(path string, input AIResourceAssessmentInput) (AIResourceAssessment, error) {
	report, err := s.AIResources(path)
	if err != nil {
		return AIResourceAssessment{}, err
	}
	if input.ContextVersion != report.ContextVersion {
		return AIResourceAssessment{}, errors.New("assessment context changed; reread resources and project guides")
	}
	if input.Findings == nil {
		input.Findings = []AIResourceFinding{}
	}
	if input.Sources == nil {
		input.Sources = []string{}
	}
	target := input.ResourceID
	if input.Candidate != nil {
		input.ResourceID = aiID("candidate:skill", input.Candidate.Source)
		target = input.Candidate.InstalledResourceID
		if target == "" {
			input.ResourceRevision = gitDigest(input.Candidate)
		}
	}
	if input.Candidate == nil || target != "" {
		index := slices.IndexFunc(report.Resources, func(resource AIResource) bool { return resource.ID == target })
		if index < 0 {
			return AIResourceAssessment{}, errors.New("assessment target is not inventoried")
		}
		resource := report.Resources[index]
		if resource.Availability == "candidate" || resource.Revision != input.ResourceRevision {
			return AIResourceAssessment{}, errors.New("assessment resource changed; rescan before recording")
		}
		if input.Candidate != nil && resource.Kind != "skill" {
			return AIResourceAssessment{}, errors.New("a Skill candidate can only link to an inventoried Skill")
		}
	}
	if err := validateAIResourceAssessment(input.AIResourceAssessmentData); err != nil {
		return AIResourceAssessment{}, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	items, err := s.readAIResourceAssessments(path)
	if err != nil {
		return AIResourceAssessment{}, err
	}
	index := slices.IndexFunc(items, func(item AIResourceAssessment) bool { return item.ResourceID == input.ResourceID })
	expected := ""
	if index >= 0 {
		expected = items[index].Version
	} else if path != "" {
		global, err := s.readAIResourceAssessments("")
		if err != nil {
			return AIResourceAssessment{}, err
		}
		for _, item := range global {
			if item.ResourceID == input.ResourceID {
				expected = item.Version
			}
		}
	}
	if input.ExpectedVersion != expected {
		return AIResourceAssessment{}, errors.New("assessment changed; reread before replacing it")
	}
	item := AIResourceAssessment{
		AIResourceAssessmentData: input.AIResourceAssessmentData,
		Scope:                    path, RecordedAt: time.Now().UTC().Format(time.RFC3339Nano),
	}
	if index >= 0 {
		items[index] = item
	} else {
		items = append(items, item)
	}
	if len(items) > 128 {
		return AIResourceAssessment{}, errors.New("assessment limit of 128 records per scope reached")
	}
	// Persist only source data; versions and freshness are recomputed on read.
	for i := range items {
		items[i].Version, items[i].Stale = "", false
	}
	if err := saveReviewRecord(s.aiResourceAssessmentPath(path), items); err != nil {
		return AIResourceAssessment{}, fmt.Errorf("save resource assessment: %w", err)
	}
	item.Version = gitDigest(item)
	return item, nil
}
