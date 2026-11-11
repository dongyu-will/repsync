package workspace

import (
	"crypto/rand"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"
)

type ReviewIssue struct {
	Kind            string                 `json:"kind"`
	Message         string                 `json:"message"`
	Section         string                 `json:"section"`
	Key             string                 `json:"key,omitempty"`
	Acknowledgement *ReviewAcknowledgement `json:"acknowledgement,omitempty"`
}
type ReviewCheckInput struct {
	Command     string           `json:"command"`
	Status      string           `json:"status"`
	Scope       string           `json:"scope"`
	Evidence    string           `json:"evidence"`
	Coverage    []ReviewCoverage `json:"coverage,omitempty"`
	Environment string           `json:"environment,omitempty"`
	Target      string           `json:"target,omitempty"`
}
type ReviewCheck struct {
	ID          string           `json:"id"`
	Command     string           `json:"command"`
	Status      string           `json:"status"`
	Scope       string           `json:"scope"`
	Evidence    string           `json:"evidence"`
	Source      string           `json:"source"`
	CreatedAt   string           `json:"createdAt"`
	Commit      string           `json:"commit"`
	Fingerprint string           `json:"fingerprint"`
	Current     string           `json:"current"`
	Reason      string           `json:"reason"`
	Coverage    []ReviewCoverage `json:"coverage,omitempty"`
	Environment string           `json:"environment,omitempty"`
	Target      string           `json:"target,omitempty"`
}
type ReviewEvidenceInput struct {
	Stage  string `json:"stage"`
	Target string `json:"target"`
	Status string `json:"status"`
	Note   string `json:"note"`
}
type ReviewEvidence struct {
	ID              string `json:"id"`
	Stage           string `json:"stage"`
	Target          string `json:"target"`
	Status          string `json:"status"`
	Note            string `json:"note"`
	CreatedAt       string `json:"createdAt"`
	Commit          string `json:"commit"`
	Fingerprint     string `json:"fingerprint"`
	ResourceVersion string `json:"resourceVersion"`
}
type ReviewStage struct {
	Stage      string `json:"stage"`
	Name       string `json:"name"`
	Status     string `json:"status"`
	Detail     string `json:"detail"`
	RecordedAt string `json:"recordedAt"`
}
type ReviewResourceResult struct {
	State      ReviewResourceState `json:"state"`
	Status     string              `json:"status"`
	Note       string              `json:"note"`
	RecordedAt string              `json:"recordedAt"`
}
type ReviewTaskPoint struct {
	Head        string   `json:"head"`
	Branch      string   `json:"branch"`
	Fingerprint string   `json:"fingerprint"`
	Staged      int      `json:"staged"`
	Unstaged    int      `json:"unstaged"`
	Untracked   []string `json:"untracked"`
	Stashes     []string `json:"stashes"`
	Known       bool     `json:"known"`
}
type ReviewTask struct {
	Owner        string          `json:"owner,omitempty"`
	Areas        []string        `json:"areas,omitempty"`
	Paths        []string        `json:"paths,omitempty"`
	Environment  string          `json:"environment,omitempty"`
	Target       string          `json:"target,omitempty"`
	Worktree     string          `json:"worktree,omitempty"`
	ID           string          `json:"id"`
	Title        string          `json:"title"`
	StartedAt    string          `json:"startedAt"`
	EndedAt      string          `json:"endedAt"`
	Start        ReviewTaskPoint `json:"start"`
	End          ReviewTaskPoint `json:"end"`
	Summary      string          `json:"summary"`
	Checks       string          `json:"checks"`
	Blockers     string          `json:"blockers"`
	NewStashes   []GitStash      `json:"newStashes"`
	NewUntracked []string        `json:"newUntracked"`
	Handover     string          `json:"handover"`
}
type ReviewFinishInput struct {
	Summary  string `json:"summary"`
	Checks   string `json:"checks"`
	Blockers string `json:"blockers"`
}
type ProjectReview struct {
	TaskWarnings      []string               `json:"taskWarnings"`
	Project           Project                `json:"project"`
	CheckedAt         string                 `json:"checkedAt"`
	Snapshot          ReviewSnapshot         `json:"snapshot"`
	Issues            []ReviewIssue          `json:"issues"`
	Checks            []ReviewCheck          `json:"checks"`
	Delivery          []ReviewStage          `json:"delivery"`
	Resources         []ReviewResourceResult `json:"resources"`
	Diagnosis         []DiagnosisIssue       `json:"diagnosis"`
	Tasks             []ReviewTask           `json:"tasks"`
	ConfigVersion     string                 `json:"configVersion"`
	Version           string                 `json:"version"`
	Profile           ReviewProfile          `json:"profile"`
	ProfileConfigured bool                   `json:"profileConfigured"`
	CoverageOptions   []ReviewRequirement    `json:"coverageOptions"`
	Coverage          []ReviewCoverageState  `json:"coverage"`
	Acknowledged      []ReviewIssue          `json:"acknowledged"`
}

func (s *Service) reviewDirectory(path string) string {
	return filepath.Join(filepath.Dir(s.path), "review", gitDigest(path))
}
func reviewRecordID() string { return fmt.Sprintf("%019d-%s", time.Now().UnixMilli(), rand.Text()) }

func readReviewData(path string) ([]byte, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer file.Close()
	data, err := io.ReadAll(io.LimitReader(file, 1024*1024+1))
	if err != nil {
		return nil, err
	}
	if len(data) > 1024*1024 {
		return nil, errors.New("review record exceeds 1 MiB")
	}
	return data, nil
}

func saveReviewRecord(path string, value any) error {
	data, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return err
	}
	if len(data) > 1024*1024 {
		return errors.New("review record exceeds 1 MiB; shorten the handoff")
	}
	return saveJSON(path, value)
}

func readReviewRecords[T any](directory string) ([]T, error) {
	return readReviewRecordsLimit[T](directory, 20)
}

func readReviewRecordsLimit[T any](directory string, limit int) ([]T, error) {
	values := []T{}
	entries, err := os.ReadDir(directory)
	if errors.Is(err, os.ErrNotExist) {
		return values, nil
	}
	if err != nil {
		return values, err
	}
	slices.Reverse(entries)
	for _, entry := range entries {
		if len(values) >= limit {
			if limit > 20 {
				return values, errors.New("records exceed read limits; history review is incomplete")
			}
			break
		}
		if !strings.HasSuffix(entry.Name(), ".json") {
			continue
		}
		data, err := readReviewData(filepath.Join(directory, entry.Name()))
		if err != nil {
			return values, err
		}
		var value T
		if err := json.Unmarshal(data, &value); err != nil {
			return values, fmt.Errorf("invalid review record: %w", err)
		}
		values = append(values, value)
	}
	return values, nil
}

// ReviewProject reads local facts. It never fetches, executes project checks or changes repositories.
func (s *Service) ReviewProject(path string) (ProjectReview, error) {
	s.reviewMu.Lock()
	defer s.reviewMu.Unlock()
	return s.projectReview(path)
}

func (s *Service) projectReview(path string) (ProjectReview, error) {
	report := ProjectReview{CheckedAt: time.Now().UTC().Format(time.RFC3339), Issues: []ReviewIssue{}, Checks: []ReviewCheck{}, Delivery: []ReviewStage{}, Resources: []ReviewResourceResult{}, Diagnosis: []DiagnosisIssue{}, Tasks: []ReviewTask{}}
	project, err := s.registeredProject(path)
	if err != nil {
		return report, err
	}
	report.Project = project
	if report.Project.ID == "" {
		report.Project.ID = gitDigest(path)
	}
	profile, configured, profileErr := s.reviewProfile(path)
	if profileErr != nil {
		profile, configured = ReviewProfile{Kind: "unconfigured"}, false
	}
	report.Profile, report.ProfileConfigured, report.CoverageOptions = profile, configured, slices.Clone(reviewRequirements)
	report.Snapshot = reviewSnapshot(path)
	issue := func(kind, section, message string) {
		report.Issues = append(report.Issues, ReviewIssue{Kind: kind, Section: section, Message: message})
	}
	if profileErr != nil {
		issue("profile_unknown", "settings", profileErr.Error())
	} else if !configured {
		issue("profile_missing", "settings", "Frontend, backend, database, and delivery stage applicability not declared")
	}
	x := report.Snapshot
	if x.Error != "" && profile.Kind != "notes" {
		issue("git_unknown", "changes", x.Error)
	} else {
		if x.Untracked > 0 {
			issue("untracked", "changes", fmt.Sprintf("%d untracked files not included in Git", x.Untracked))
		}
		if x.Staged > 0 || x.Unstaged > 0 {
			issue("uncommitted", "changes", fmt.Sprintf("%d staged and %d unstaged files pending", x.Staged, x.Unstaged))
		}
		if x.Conflicts > 0 {
			issue("conflicts", "changes", fmt.Sprintf("%d files still have conflicts", x.Conflicts))
		}
		seenStashes := map[string]bool{}
		for _, stash := range x.Stashes {
			if !seenStashes[stash.OID] {
				issue("stash_"+stash.OID, "changes", stash.Selector+" needs review: "+stash.Subject)
				seenStashes[stash.OID] = true
			}
		}
		if !x.UpstreamKnown && (!configured || slices.Contains(profile.Stages, "push")) {
			issue("upstream_unknown", "changes", "Upstream ref missing or unreadable; push status not verified")
		} else {
			if x.Ahead > 0 {
				issue("ahead", "changes", fmt.Sprintf("%d local commits need push verification", x.Ahead))
			}
			if x.Behind > 0 {
				issue("behind", "changes", fmt.Sprintf("%d commits behind the local upstream ref", x.Behind))
			}
		}
	}
	if x.FingerprintError != "" && (len(profile.Requirements) > 0 || !configured) {
		issue("content_unknown", "checks", x.FingerprintError)
	}
	checks, err := readReviewRecordsLimit[ReviewCheck](filepath.Join(s.reviewDirectory(path), "checks"), 1000)
	if err != nil {
		return report, err
	}
	for _, check := range checks {
		check.Current, check.Reason = reviewCheckCurrent(check.Fingerprint, x.Fingerprint)
		report.Checks = append(report.Checks, check)
	}
	runs, err := s.PipelineRuns(path)
	if err != nil {
		issue("checks_unknown", "checks", err.Error())
	} else {
		config, configErr := s.Pipelines(path)
		for _, run := range runs {
			if run.Kind != "ci" {
				continue
			}
			check := ReviewCheck{ID: run.ID, Command: run.Name, Status: run.Status, Scope: "Check scope declared by the pipeline", Evidence: "Local pipeline log: " + run.ID, Source: "pipeline", CreatedAt: run.StartedAt, Commit: run.Commit, Fingerprint: run.Fingerprint}
			check.Coverage, check.Environment, check.Target = run.Coverage, run.ReviewEnvironment, run.ReviewTarget
			check.Current, check.Reason = reviewCheckCurrent(run.Fingerprint, x.Fingerprint)
			if run.FingerprintError != "" || run.EndFingerprint == "" {
				check.Current, check.Reason = "unknown", "File contents before and after execution were not fully recorded"
			} else if run.Fingerprint != run.EndFingerprint {
				check.Current, check.Reason = "stale", "File contents changed during execution"
			} else if configErr != nil || run.ConfigVersion != config.Version {
				check.Current, check.Reason = "stale", "Check command configuration changed or cannot be verified"
			}
			report.Checks = append(report.Checks, check)
		}
	}
	slices.SortFunc(report.Checks, func(a, b ReviewCheck) int {
		at, _ := time.Parse(time.RFC3339Nano, a.CreatedAt)
		bt, _ := time.Parse(time.RFC3339Nano, b.CreatedAt)
		if order := bt.Compare(at); order != 0 {
			return order
		}
		return strings.Compare(b.ID, a.ID)
	})
	if len(report.Checks) == 0 && !configured {
		issue("no_checks", "checks", "No check records available for verification")
	} else if len(report.Checks) > 0 && (report.Checks[0].Status != "success" || report.Checks[0].Current != "current") && !configured {
		issue("check_attention", "checks", "Recent checks failed, content changed, or results cannot be verified")
	}
	report.Coverage = reviewCoverageStates(profile, report.Checks)
	if len(report.Checks) > 20 {
		report.Checks = report.Checks[:20]
	}
	for _, state := range report.Coverage {
		if state.Status != "passed" && state.Status != "recorded" {
			issue("coverage_"+state.Requirement.ID, "checks", state.Requirement.Name+" needs verification: "+state.Reason)
		}
	}
	for _, area := range profile.Areas {
		if !slices.ContainsFunc(report.Coverage, func(c ReviewCoverageState) bool { return c.Requirement.Area == area }) {
			issue("coverage_unspecified_"+area, "settings", "Enabled "+area+", but required checks have not been declared")
		}
	}
	evidence, err := readReviewRecordsLimit[ReviewEvidence](filepath.Join(s.reviewDirectory(path), "evidence"), 1000)
	if err != nil {
		return report, err
	}
	report.Delivery = reviewDelivery(x, report.Checks, evidence)
	for i := range report.Delivery {
		stage := &report.Delivery[i]
		if !slices.Contains([]string{"working", "commit"}, stage.Stage) {
			if configured && !slices.Contains(profile.Stages, stage.Stage) {
				stage.Status, stage.Detail = "not_applicable", "Not required by project applicability settings"
			} else if !configured && stage.RecordedAt == "" {
				stage.Status, stage.Detail = "unknown", "Applicability of this stage not declared"
			}
		}
	}
	for _, stage := range report.Delivery {
		if configured && slices.Contains([]string{"deploy", "acceptance"}, stage.Stage) && stage.Status != "recorded" && stage.Status != "not_applicable" {
			issue("delivery_"+stage.Stage, "delivery", stage.Name+" is unverified or its record is stale")
		}
	}
	resources, version, err := s.reviewResources(path)
	report.ConfigVersion = version
	if err != nil {
		issue("resources_unknown", "resources", err.Error())
	} else {
		if len(resources) == 0 && slices.Contains(profile.Areas, "database") {
			issue("resources_missing", "resources", "Local data, backup, and environment resources not declared")
		}
		for _, state := range resources {
			result := ReviewResourceResult{State: state, Status: "pending"}
			for _, entry := range evidence {
				if entry.Stage != "resource" || entry.Target != state.Definition.ID {
					continue
				}
				result.Note, result.RecordedAt = entry.Note, entry.CreatedAt
				if entry.ResourceVersion == state.Version && entry.Fingerprint == x.Fingerprint && x.Fingerprint != "" {
					result.Status = entry.Status
				} else {
					result.Status = "stale"
				}
				break
			}
			if result.Status != "success" && result.Status != "not_applicable" {
				issue("resource_"+state.Definition.ID, "resources", state.Definition.Name+" needs verification")
			}
			report.Resources = append(report.Resources, result)
		}
	}
	diagnosis, err := s.DiagnoseProject(path)
	if err != nil {
		issue("environment_unknown", "resources", err.Error())
	} else {
		report.Diagnosis = diagnosis.Issues
		for _, d := range diagnosis.Issues {
			issue("environment_"+d.Kind, "resources", d.Message)
		}
	}
	report.Tasks, err = s.reviewTaskHistory(path)
	if err != nil {
		return report, err
	}
	for _, task := range report.Tasks {
		if task.EndedAt == "" {
			issue("task_"+task.ID, "handover", "Task not handed off: "+task.Title)
		}
	}
	report.TaskWarnings = s.reviewTaskWarnings(path, report.Tasks)
	for index, warning := range report.TaskWarnings {
		issue(fmt.Sprintf("task_warning_%d", index), "handover", warning)
	}
	acknowledgements, ackErr := s.reviewAcknowledgements(&report)
	if ackErr != nil {
		return report, ackErr
	}
	report.Version = gitDigest(struct {
		Snapshot         ReviewSnapshot
		Config           string
		Checks           []ReviewCheck
		Evidence         []ReviewEvidence
		Resources        []ReviewResourceState
		Tasks            []ReviewTask
		Coverage         []ReviewCoverageState
		Acknowledgements []ReviewAcknowledgement
		TaskWarnings     []string
	}{x, version, report.Checks, evidence, resources, report.Tasks, report.Coverage, acknowledgements, report.TaskWarnings})
	return report, nil
}

func reviewCheckCurrent(record, current string) (string, string) {
	if record == "" || current == "" {
		return "unknown", "Content not recorded or current content cannot be verified"
	}
	if record != current {
		return "stale", "File contents changed after checks"
	}
	return "current", "Matches current files; excludes ignored dependencies, external services, and environment changes"
}

func reviewDelivery(x ReviewSnapshot, checks []ReviewCheck, evidence []ReviewEvidence) []ReviewStage {
	stages := []ReviewStage{{Stage: "working", Name: "Local changes", Status: "observed", Detail: "Current file state loaded"}, {Stage: "commit", Name: "Commit", Status: "pending", Detail: "No commit found"}, {Stage: "push", Name: "Push", Status: "pending", Detail: "Local information alone cannot verify remote state"}, {Stage: "ci", Name: "CI / checks", Status: "pending", Detail: "Full check coverage not verified"}, {Stage: "deploy", Name: "Deploy", Status: "pending", Detail: "Packaging or CD completion does not prove live deployment"}, {Stage: "acceptance", Name: "Live acceptance", Status: "pending", Detail: "Business behavior requires separate verification"}}
	if x.Error != "" {
		stages[0].Status, stages[0].Detail = "unknown", x.Error
	}
	if x.Head != "" {
		stages[1].Status, stages[1].Detail = "observed", x.Head
		if len(x.Files) > 0 {
			stages[1].Detail += "; uncommitted files remain"
		}
	}
	if x.UpstreamKnown {
		stages[2].Detail = fmt.Sprintf("Local upstream ref: %d ahead / %d behind; not verified online", x.Ahead, x.Behind)
	}
	if len(checks) > 0 {
		stages[3].Detail = "Latest record: " + checks[0].Command + "; " + checks[0].Reason
	}
	for i := range stages {
		for _, entry := range evidence {
			if entry.Stage != stages[i].Stage || entry.Stage == "resource" {
				continue
			}
			stages[i].RecordedAt = entry.CreatedAt
			if entry.Commit == x.Head && entry.Fingerprint == x.Fingerprint && x.Fingerprint != "" {
				stages[i].Status = "recorded"
				if entry.Status == "failure" {
					stages[i].Status = "failed"
				}
				if entry.Status == "not_applicable" {
					stages[i].Status = "not_applicable"
				}
			} else {
				stages[i].Status = "stale"
			}
			stages[i].Detail = "Manual verification: " + entry.Note
			break
		}
	}
	return stages
}

func (s *Service) checkedReview(path, version string) (ProjectReview, error) {
	report, err := s.projectReview(path)
	if err != nil {
		return report, err
	}
	if report.Version != version {
		return report, errors.New("project state or records changed; check again before saving")
	}
	return report, nil
}

// RecordReviewCheck records an external assertion, clearly separated from executed pipelines.
func (s *Service) RecordReviewCheck(path, version string, input ReviewCheckInput) (ReviewCheck, error) {
	s.reviewMu.Lock()
	defer s.reviewMu.Unlock()
	check := ReviewCheck{}
	if strings.TrimSpace(input.Command) == "" || len(input.Command) > 2000 || strings.TrimSpace(input.Scope) == "" || len(input.Scope) > 2000 || strings.TrimSpace(input.Evidence) == "" || len(input.Evidence) > 8192 || !slices.Contains([]string{"success", "failure"}, input.Status) {
		return check, errors.New("provide check command, scope, result, and notes (up to 8 KiB)")
	}
	if err := validateReviewCoverage(input.Coverage, input.Environment, input.Target); err != nil {
		return check, err
	}
	report, err := s.checkedReview(path, version)
	if err != nil {
		return check, err
	}
	check = ReviewCheck{ID: reviewRecordID(), Command: input.Command, Status: input.Status, Scope: input.Scope, Evidence: input.Evidence, Source: "external", CreatedAt: time.Now().UTC().Format(time.RFC3339Nano), Commit: report.Snapshot.Head, Fingerprint: report.Snapshot.Fingerprint}
	check.Coverage, check.Environment, check.Target = input.Coverage, input.Environment, input.Target
	err = saveReviewRecord(filepath.Join(s.reviewDirectory(path), "checks", check.ID+".json"), check)
	return check, err
}

func (s *Service) RecordReviewEvidence(path, version string, input ReviewEvidenceInput) (ReviewEvidence, error) {
	s.reviewMu.Lock()
	defer s.reviewMu.Unlock()
	record := ReviewEvidence{}
	if !slices.Contains([]string{"push", "ci", "deploy", "acceptance", "resource"}, input.Stage) || !slices.Contains([]string{"success", "failure", "not_applicable"}, input.Status) || strings.TrimSpace(input.Note) == "" || len(input.Note) > 8192 {
		return record, errors.New("select a verification stage and result, and provide notes (up to 8 KiB)")
	}
	report, err := s.checkedReview(path, version)
	if err != nil {
		return record, err
	}
	record = ReviewEvidence{ID: reviewRecordID(), Stage: input.Stage, Target: input.Target, Status: input.Status, Note: input.Note, CreatedAt: time.Now().UTC().Format(time.RFC3339), Commit: report.Snapshot.Head, Fingerprint: report.Snapshot.Fingerprint}
	if input.Stage == "resource" {
		index := slices.IndexFunc(report.Resources, func(r ReviewResourceResult) bool { return r.State.Definition.ID == input.Target })
		if index < 0 {
			return record, errors.New("data or environment check not found")
		}
		state := report.Resources[index].State
		if input.Status == "success" && (state.Observed == "missing" || state.Error != "") {
			return record, errors.New("verification resource is missing or unreadable; resolve it or explicitly mark not applicable")
		}
		record.ResourceVersion = report.Resources[index].State.Version
	}
	err = saveReviewRecord(filepath.Join(s.reviewDirectory(path), "evidence", record.ID+".json"), record)
	return record, err
}

func reviewTaskPoint(x ReviewSnapshot) ReviewTaskPoint {
	point := ReviewTaskPoint{Head: x.Head, Branch: x.Branch, Fingerprint: x.Fingerprint, Staged: x.Staged, Unstaged: x.Unstaged, Known: x.Error == "", Untracked: []string{}, Stashes: []string{}}
	for _, f := range x.Files {
		if f.Untracked {
			point.Untracked = append(point.Untracked, f.Path)
		}
	}
	for _, stash := range x.Stashes {
		point.Stashes = append(point.Stashes, stash.OID)
	}
	return point
}

func (s *Service) StartReviewTask(path, version, title string) (ReviewTask, error) {
	return s.StartReviewTaskWithContext(path, version, ReviewTaskContext{Title: title})
}

func (s *Service) StartReviewTaskWithContext(path, version string, input ReviewTaskContext) (ReviewTask, error) {
	s.reviewMu.Lock()
	defer s.reviewMu.Unlock()
	task := ReviewTask{}
	if err := validateReviewTaskContext(input); err != nil {
		return task, err
	}
	report, err := s.checkedReview(path, version)
	if err != nil {
		return task, err
	}
	task = ReviewTask{ID: reviewRecordID(), Title: input.Title, StartedAt: time.Now().UTC().Format(time.RFC3339Nano), Start: reviewTaskPoint(report.Snapshot), NewStashes: []GitStash{}, NewUntracked: []string{}, Owner: input.Owner, Areas: input.Areas, Paths: input.Paths, Environment: input.Environment, Target: input.Target, Worktree: path}
	err = saveReviewRecord(filepath.Join(s.reviewDirectory(path), "tasks", task.ID+".json"), task)
	return task, err
}

func (s *Service) FinishReviewTask(path, id, version string, input ReviewFinishInput) (ReviewTask, error) {
	s.reviewMu.Lock()
	defer s.reviewMu.Unlock()
	task := ReviewTask{}
	if !pipelineRunID.MatchString(id) || strings.TrimSpace(input.Summary) == "" || len(input.Summary) > 8192 || len(input.Checks) > 8192 || len(input.Blockers) > 8192 {
		return task, errors.New("describe completed work; each handoff section allows up to 8 KiB")
	}
	report, err := s.checkedReview(path, version)
	if err != nil {
		return task, err
	}
	file := filepath.Join(s.reviewDirectory(path), "tasks", id+".json")
	data, err := readReviewData(file)
	if err != nil {
		return task, err
	}
	if err = json.Unmarshal(data, &task); err != nil {
		return task, err
	}
	if task.ID != id || task.EndedAt != "" {
		return task, errors.New("task already handed off or ID does not match")
	}
	task.End = reviewTaskPoint(report.Snapshot)
	task.EndedAt = time.Now().UTC().Format(time.RFC3339)
	task.Summary, task.Checks, task.Blockers = input.Summary, input.Checks, input.Blockers
	if task.Start.Known && task.End.Known {
		for _, stash := range report.Snapshot.Stashes {
			if !slices.Contains(task.Start.Stashes, stash.OID) {
				task.NewStashes = append(task.NewStashes, stash)
			}
		}
		for _, name := range task.End.Untracked {
			if !slices.Contains(task.Start.Untracked, name) {
				task.NewUntracked = append(task.NewUntracked, name)
			}
		}
	}
	task.Handover = reviewHandover(report, task)
	err = saveReviewRecord(file, task)
	return task, err
}

func reviewHandover(report ProjectReview, task ReviewTask) string {
	var text strings.Builder
	fmt.Fprintf(&text, "# %s\n\nProject: %s\nWorktree: %s\nTask: %s\nStarted: %s\nHandoff: %s\n\n## Completed work\n\n%s\n\n## Git state\n\nBranch: %s\nStarting commit: %s\nCurrent commit: %s\nStaged %d / unstaged %d / untracked %d / stash %d\n", task.Title, report.Project.Name, report.Project.Path, task.ID, task.StartedAt, task.EndedAt, task.Summary, task.End.Branch, task.Start.Head, task.End.Head, task.End.Staged, task.End.Unstaged, len(task.End.Untracked), len(task.End.Stashes))
	fmt.Fprintf(&text, "\nTask reporter/session: %s\nDeclared scope: %s\nPaths: %s\nEnvironment and target: %s / %s\n", task.Owner, strings.Join(task.Areas, ", "), strings.Join(task.Paths, ", "), task.Environment, task.Target)
	if !task.Start.Known || !task.End.Known {
		text.WriteString("\nStart or end state is incomplete; additions during the task cannot be reliably identified.\n")
	} else {
		text.WriteString("\nStashes added during the task (creator not inferred):\n")
		for _, stash := range task.NewStashes {
			fmt.Fprintf(&text, "- %s %s %s\n", stash.Selector, stash.OID, stash.Subject)
		}
		text.WriteString("\nUntracked files added during the task:\n")
		for _, name := range task.NewUntracked {
			fmt.Fprintf(&text, "- %s\n", name)
		}
	}
	fmt.Fprintf(&text, "\n## Check notes (reported)\n\n%s\n\n", task.Checks)
	for _, item := range report.Coverage {
		fmt.Fprintf(&text, "- %s [%s]: %s; environment %s / target %s; %s\n", item.Requirement.Name, item.Status, item.Reason, item.Environment, item.Target, item.Evidence)
	}
	text.WriteString("\n## Reviewed: kept and deferred\n\n")
	for _, item := range report.Acknowledged {
		fmt.Fprintf(&text, "- %s [%s]: %s\n", item.Message, item.Acknowledgement.State, item.Acknowledgement.Reason)
	}
	text.WriteString("\n## Delivery state\n\n")
	for _, stage := range report.Delivery {
		fmt.Fprintf(&text, "- %s [%s]: %s\n", stage.Name, stage.Status, stage.Detail)
	}
	fmt.Fprintf(&text, "\n## Open issues\n\n%s\n\nThis record does not prove commit, push, deployment, or live acceptance completion. Manual verification and app execution are recorded separately.\n", task.Blockers)
	return text.String()
}

func (s *Service) ReviewHandover(path, id string) (string, error) {
	if _, err := s.registeredProject(path); err != nil {
		return "", err
	}
	if !pipelineRunID.MatchString(id) {
		return "", errors.New("invalid handoff record ID")
	}
	data, err := readReviewData(filepath.Join(s.reviewDirectory(path), "tasks", id+".json"))
	if err != nil {
		return "", err
	}
	var task ReviewTask
	if err := json.Unmarshal(data, &task); err != nil {
		return "", err
	}
	if task.EndedAt == "" {
		return "", errors.New("task has not been handed off")
	}
	return task.Handover, nil
}
