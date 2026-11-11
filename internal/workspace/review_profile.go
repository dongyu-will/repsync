package workspace

import (
	"encoding/json"
	"errors"
	"os"
	"slices"
	"strings"
)

type ReviewProfile struct {
	Kind         string   `json:"kind"`
	Areas        []string `json:"areas"`
	Requirements []string `json:"requirements"`
	Stages       []string `json:"stages"`
}

type ReviewRequirement struct {
	ID     string `json:"id"`
	Name   string `json:"name"`
	Area   string `json:"area"`
	Method string `json:"method"`
}

type ReviewCoverage struct {
	Area   string `json:"area"`
	Method string `json:"method"`
}

type ReviewCoverageState struct {
	Requirement ReviewRequirement `json:"requirement"`
	Status      string            `json:"status"`
	Reason      string            `json:"reason"`
	CheckID     string            `json:"checkId"`
	Source      string            `json:"source"`
	Evidence    string            `json:"evidence"`
	Environment string            `json:"environment"`
	Target      string            `json:"target"`
	RecordedAt  string            `json:"recordedAt"`
}

var reviewRequirements = []ReviewRequirement{
	{ID: "frontend-static", Name: "Frontend type and static checks", Area: "frontend", Method: "static"},
	{ID: "frontend-behavior", Name: "Browser and interaction acceptance", Area: "frontend", Method: "behavior"},
	{ID: "backend-static", Name: "Backend static checks", Area: "backend", Method: "static"},
	{ID: "backend-api", Name: "Real HTTP / API integration", Area: "backend", Method: "integration"},
	{ID: "database-integration", Name: "Real database integration", Area: "database", Method: "integration"},
	{ID: "database-migrations", Name: "Migration status and compatibility", Area: "database", Method: "migration"},
	{ID: "database-consistency", Name: "Data consistency and key business constraints", Area: "database", Method: "consistency"},
	{ID: "database-recovery", Name: "Backup and recovery drills", Area: "database", Method: "recovery"},
}

func (s *Service) reviewProfile(path string) (ReviewProfile, bool, error) {
	profile := ReviewProfile{Kind: "unconfigured", Areas: []string{}, Requirements: []string{}, Stages: []string{}}
	root, err := os.OpenRoot(path)
	if err != nil {
		return profile, false, err
	}
	defer root.Close()
	file, err := loadProjectConfigFile(root)
	if err != nil {
		return profile, false, err
	}
	data := file.fields["reviewProfile"]
	if len(data) == 0 || string(data) == "null" {
		return profile, false, nil
	}
	if err := json.Unmarshal(data, &profile); err != nil {
		return profile, false, errors.New("invalid project applicability configuration")
	}
	return profile, true, validateReviewProfile(profile)
}

func validateReviewProfile(profile ReviewProfile) error {
	if !slices.Contains([]string{"web", "backend", "desktop", "library", "notes", "custom"}, profile.Kind) {
		return errors.New("invalid project type")
	}
	seen := map[string]bool{}
	for _, area := range profile.Areas {
		if !slices.Contains([]string{"frontend", "backend", "database"}, area) || seen["area:"+area] {
			return errors.New("invalid or duplicate check scopes")
		}
		seen["area:"+area] = true
	}
	for _, id := range profile.Requirements {
		index := slices.IndexFunc(reviewRequirements, func(r ReviewRequirement) bool { return r.ID == id })
		if index < 0 || seen["requirement:"+id] || !slices.Contains(profile.Areas, reviewRequirements[index].Area) {
			return errors.New("required checks are invalid, duplicated, or outside the applicable scope")
		}
		seen["requirement:"+id] = true
	}
	for _, stage := range profile.Stages {
		if !slices.Contains([]string{"push", "ci", "deploy", "acceptance"}, stage) || seen["stage:"+stage] {
			return errors.New("invalid or duplicate delivery stages")
		}
		seen["stage:"+stage] = true
	}
	return nil
}

func (s *Service) SaveReviewProfile(path, version string, profile ReviewProfile) error {
	if err := validateReviewProfile(profile); err != nil {
		return err
	}
	return s.saveProjectSection(path, version, "reviewProfile", false, profile)
}

func validateReviewCoverage(coverage []ReviewCoverage, environment, target string) error {
	if len(coverage) > len(reviewRequirements)+1 || len(environment) > 200 || len(target) > 200 {
		return errors.New("check scope or environment notes exceed limits")
	}
	seen := map[string]bool{}
	for _, c := range coverage {
		valid := slices.ContainsFunc(reviewRequirements, func(r ReviewRequirement) bool { return r.Area == c.Area && r.Method == c.Method })
		// SQL/static checks can be recorded, but do not satisfy real database requirements.
		if c.Area == "database" && c.Method == "static" {
			valid = true
		}
		if !valid || seen[c.Area+":"+c.Method] {
			return errors.New("invalid or duplicate check scopes or methods")
		}
		seen[c.Area+":"+c.Method] = true
		if c.Area == "database" && (strings.TrimSpace(environment) == "" || strings.TrimSpace(target) == "") {
			return errors.New("database checks require environment and target aliases; command success alone is insufficient")
		}
	}
	for _, value := range []string{environment, target} {
		lower := strings.ToLower(value)
		if strings.Contains(lower, "://") || strings.Contains(lower, "password=") || strings.Contains(lower, "pwd=") {
			return errors.New("use aliases for environment and target, never connection strings or passwords")
		}
	}
	return nil
}

func reviewCoverageStates(profile ReviewProfile, checks []ReviewCheck) []ReviewCoverageState {
	states := []ReviewCoverageState{}
	for _, requirement := range reviewRequirements {
		if !slices.Contains(profile.Requirements, requirement.ID) {
			continue
		}
		state := ReviewCoverageState{Requirement: requirement, Status: "pending", Reason: "No check records for this scope and method"}
		for _, check := range checks {
			if !slices.ContainsFunc(check.Coverage, func(c ReviewCoverage) bool { return c.Area == requirement.Area && c.Method == requirement.Method }) {
				continue
			}
			state.CheckID, state.Source, state.Evidence, state.Environment, state.Target, state.RecordedAt = check.ID, check.Source, check.Evidence, check.Environment, check.Target, check.CreatedAt
			state.Status, state.Reason = check.Current, check.Reason
			if check.Current == "current" {
				if check.Status == "success" {
					state.Status = "recorded"
					state.Reason = "Record matches current content; coverage and environment are declared information"
					if check.Source == "pipeline" {
						state.Status = "passed"
						state.Reason = "Command passed; verify whether the declared scope actually executed"
					}
				} else if check.Status == "failure" {
					state.Status = "failed"
					state.Reason = "This check failed"
				} else {
					state.Status = "pending"
					state.Reason = "This check is incomplete"
				}
			}
			if requirement.Area == "database" && (check.Environment == "" || check.Target == "") {
				state.Status, state.Reason = "unknown", "Database environment or target not recorded; cannot verify"
			}
			break
		}
		states = append(states, state)
	}
	return states
}
