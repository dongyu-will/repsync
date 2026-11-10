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
		return nil, errors.New("收尾记录超过 1 MiB")
	}
	return data, nil
}

func saveReviewRecord(path string, value any) error {
	data, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return err
	}
	if len(data) > 1024*1024 {
		return errors.New("收尾记录超过 1 MiB，请缩短交接内容")
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
				return values, errors.New("记录超过本次读取上限，未能完成历史核对")
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
			return values, fmt.Errorf("收尾记录损坏: %w", err)
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
	report.Profile, report.ProfileConfigured, report.CoverageOptions = profile, configured, slices.Clone(reviewRequirements)
	report.Snapshot = reviewSnapshot(path)
	issue := func(kind, section, message string) {
		report.Issues = append(report.Issues, ReviewIssue{Kind: kind, Section: section, Message: message})
	}
	if profileErr != nil {
		issue("profile_unknown", "settings", profileErr.Error())
	} else if !configured {
		issue("profile_missing", "settings", "尚未声明前端、后端、数据库及交付阶段的适用项")
	}
	x := report.Snapshot
	if x.Error != "" && profile.Kind != "notes" {
		issue("git_unknown", "changes", x.Error)
	} else {
		if x.Untracked > 0 {
			issue("untracked", "changes", fmt.Sprintf("%d 个未跟踪文件尚未纳入 Git", x.Untracked))
		}
		if x.Staged > 0 || x.Unstaged > 0 {
			issue("uncommitted", "changes", fmt.Sprintf("%d 个已暂存、%d 个未暂存文件待处理", x.Staged, x.Unstaged))
		}
		if x.Conflicts > 0 {
			issue("conflicts", "changes", fmt.Sprintf("%d 个文件仍有冲突", x.Conflicts))
		}
		seenStashes := map[string]bool{}
		for _, stash := range x.Stashes {
			if !seenStashes[stash.OID] {
				issue("stash_"+stash.OID, "changes", stash.Selector+" 需要核对："+stash.Subject)
				seenStashes[stash.OID] = true
			}
		}
		if !x.UpstreamKnown && (!configured || slices.Contains(profile.Stages, "push")) {
			issue("upstream_unknown", "changes", "未配置或无法读取上游引用，推送状态未核验")
		} else {
			if x.Ahead > 0 {
				issue("ahead", "changes", fmt.Sprintf("%d 个本地提交需要核对是否已推送", x.Ahead))
			}
			if x.Behind > 0 {
				issue("behind", "changes", fmt.Sprintf("比本地上游引用少 %d 个提交", x.Behind))
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
			check := ReviewCheck{ID: run.ID, Command: run.Name, Status: run.Status, Scope: "流水线配置的检查范围", Evidence: "本地流水线日志：" + run.ID, Source: "pipeline", CreatedAt: run.StartedAt, Commit: run.Commit, Fingerprint: run.Fingerprint}
			check.Coverage, check.Environment, check.Target = run.Coverage, run.ReviewEnvironment, run.ReviewTarget
			check.Current, check.Reason = reviewCheckCurrent(run.Fingerprint, x.Fingerprint)
			if run.FingerprintError != "" || run.EndFingerprint == "" {
				check.Current, check.Reason = "unknown", "未完整记录运行前后的文件内容"
			} else if run.Fingerprint != run.EndFingerprint {
				check.Current, check.Reason = "stale", "运行期间文件内容发生变化"
			} else if configErr != nil || run.ConfigVersion != config.Version {
				check.Current, check.Reason = "stale", "检查命令配置已变化或无法核对"
			}
			report.Checks = append(report.Checks, check)
		}
	}
	slices.SortFunc(report.Checks, func(a, b ReviewCheck) int { return strings.Compare(b.CreatedAt, a.CreatedAt) })
	if len(report.Checks) == 0 && (!configured || len(profile.Requirements) == 0) {
		issue("no_checks", "checks", "没有可核对的检查记录")
	} else if len(report.Checks) > 0 && (report.Checks[0].Status != "success" || report.Checks[0].Current != "current") && (!configured || len(profile.Requirements) == 0) {
		issue("check_attention", "checks", "最近检查未通过、内容已变化或结果无法核对")
	}
	report.Coverage = reviewCoverageStates(profile, report.Checks)
	if len(report.Checks) > 20 {
		report.Checks = report.Checks[:20]
	}
	for _, state := range report.Coverage {
		if state.Status != "passed" && state.Status != "recorded" {
			issue("coverage_"+state.Requirement.ID, "checks", state.Requirement.Name+"需要核验："+state.Reason)
		}
	}
	for _, area := range profile.Areas {
		if !slices.ContainsFunc(report.Coverage, func(c ReviewCoverageState) bool { return c.Requirement.Area == area }) {
			issue("coverage_unspecified_"+area, "settings", "已启用 "+area+"，但尚未声明必需检查项")
		}
	}
	evidence, err := readReviewRecords[ReviewEvidence](filepath.Join(s.reviewDirectory(path), "evidence"))
	if err != nil {
		return report, err
	}
	report.Delivery = reviewDelivery(x, report.Checks, evidence)
	for i := range report.Delivery {
		stage := &report.Delivery[i]
		if !slices.Contains([]string{"working", "commit"}, stage.Stage) {
			if configured && !slices.Contains(profile.Stages, stage.Stage) {
				stage.Status, stage.Detail = "not_applicable", "按项目适用项设置，不要求此阶段"
			} else if !configured {
				stage.Status, stage.Detail = "unknown", "尚未声明此阶段是否适用"
			}
		}
	}
	for _, stage := range report.Delivery {
		if configured && slices.Contains([]string{"deploy", "acceptance"}, stage.Stage) && stage.Status != "recorded" && stage.Status != "not_applicable" {
			issue("delivery_"+stage.Stage, "delivery", stage.Name+"尚未核验或记录已过期")
		}
	}
	resources, version, err := s.reviewResources(path)
	report.ConfigVersion = version
	if err != nil {
		issue("resources_unknown", "resources", err.Error())
	} else {
		if len(resources) == 0 {
			issue("resources_missing", "resources", "尚未声明本地数据、备份和环境检查对象")
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
				issue("resource_"+state.Definition.ID, "resources", state.Definition.Name+"需要核验")
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
	report.Tasks, err = readReviewRecords[ReviewTask](filepath.Join(s.reviewDirectory(path), "tasks"))
	if err != nil {
		return report, err
	}
	for _, task := range report.Tasks {
		if task.EndedAt == "" {
			issue("task_"+task.ID, "handover", "任务尚未交接："+task.Title)
		}
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
	}{x, version, report.Checks, evidence, resources, report.Tasks, report.Coverage, acknowledgements})
	return report, nil
}

func reviewCheckCurrent(record, current string) (string, string) {
	if record == "" || current == "" {
		return "unknown", "未记录内容或当前内容无法核对"
	}
	if record != current {
		return "stale", "检查之后文件内容发生变化"
	}
	return "current", "与当前文件内容一致；不覆盖忽略的依赖、外部服务或环境变更"
}

func reviewDelivery(x ReviewSnapshot, checks []ReviewCheck, evidence []ReviewEvidence) []ReviewStage {
	stages := []ReviewStage{{Stage: "working", Name: "本地改动", Status: "observed", Detail: "已读取当前文件状态"}, {Stage: "commit", Name: "提交", Status: "pending", Detail: "尚未找到提交"}, {Stage: "push", Name: "推送", Status: "pending", Detail: "仅凭本地信息无法证明远程现状"}, {Stage: "ci", Name: "CI / 检查", Status: "pending", Detail: "未核验完整检查范围"}, {Stage: "deploy", Name: "部署", Status: "pending", Detail: "打包或 CD 命令完成不等同线上部署"}, {Stage: "acceptance", Name: "线上验收", Status: "pending", Detail: "需要独立核验业务行为"}}
	if x.Error != "" {
		stages[0].Status, stages[0].Detail = "unknown", x.Error
	}
	if x.Head != "" {
		stages[1].Status, stages[1].Detail = "observed", x.Head
		if len(x.Files) > 0 {
			stages[1].Detail += "；仍有未提交文件"
		}
	}
	if x.UpstreamKnown {
		stages[2].Detail = fmt.Sprintf("相对本地上游引用：领先 %d / 落后 %d；未联网核验", x.Ahead, x.Behind)
	}
	if len(checks) > 0 {
		stages[3].Detail = "最近记录：" + checks[0].Command + "；" + checks[0].Reason
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
			stages[i].Detail = "手工核验记录：" + entry.Note
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
		return report, errors.New("项目状态或记录已变化，请重新检查后再保存")
	}
	return report, nil
}

// RecordReviewCheck records an external assertion, clearly separated from executed pipelines.
func (s *Service) RecordReviewCheck(path, version string, input ReviewCheckInput) (ReviewCheck, error) {
	s.reviewMu.Lock()
	defer s.reviewMu.Unlock()
	check := ReviewCheck{}
	if strings.TrimSpace(input.Command) == "" || len(input.Command) > 2000 || strings.TrimSpace(input.Scope) == "" || len(input.Scope) > 2000 || strings.TrimSpace(input.Evidence) == "" || len(input.Evidence) > 8192 || !slices.Contains([]string{"success", "failure"}, input.Status) {
		return check, errors.New("请填写检查命令、范围、结果及说明，说明上限 8 KiB")
	}
	if err := validateReviewCoverage(input.Coverage, input.Environment, input.Target); err != nil {
		return check, err
	}
	report, err := s.checkedReview(path, version)
	if err != nil {
		return check, err
	}
	check = ReviewCheck{ID: reviewRecordID(), Command: input.Command, Status: input.Status, Scope: input.Scope, Evidence: input.Evidence, Source: "external", CreatedAt: time.Now().UTC().Format(time.RFC3339), Commit: report.Snapshot.Head, Fingerprint: report.Snapshot.Fingerprint}
	check.Coverage, check.Environment, check.Target = input.Coverage, input.Environment, input.Target
	err = saveReviewRecord(filepath.Join(s.reviewDirectory(path), "checks", check.ID+".json"), check)
	return check, err
}

func (s *Service) RecordReviewEvidence(path, version string, input ReviewEvidenceInput) (ReviewEvidence, error) {
	s.reviewMu.Lock()
	defer s.reviewMu.Unlock()
	record := ReviewEvidence{}
	if !slices.Contains([]string{"push", "ci", "deploy", "acceptance", "resource"}, input.Stage) || !slices.Contains([]string{"success", "failure", "not_applicable"}, input.Status) || strings.TrimSpace(input.Note) == "" || len(input.Note) > 8192 {
		return record, errors.New("请选择核验阶段、结果并填写说明，说明上限 8 KiB")
	}
	report, err := s.checkedReview(path, version)
	if err != nil {
		return record, err
	}
	record = ReviewEvidence{ID: reviewRecordID(), Stage: input.Stage, Target: input.Target, Status: input.Status, Note: input.Note, CreatedAt: time.Now().UTC().Format(time.RFC3339), Commit: report.Snapshot.Head, Fingerprint: report.Snapshot.Fingerprint}
	if input.Stage == "resource" {
		index := slices.IndexFunc(report.Resources, func(r ReviewResourceResult) bool { return r.State.Definition.ID == input.Target })
		if index < 0 {
			return record, errors.New("未找到这项数据或环境检查")
		}
		state := report.Resources[index].State
		if input.Status == "success" && (state.Observed == "missing" || state.Error != "") {
			return record, errors.New("检查对象缺失或读取失败，请先处理，或明确记录为不适用")
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
	s.reviewMu.Lock()
	defer s.reviewMu.Unlock()
	task := ReviewTask{}
	if strings.TrimSpace(title) == "" || len(title) > 500 {
		return task, errors.New("请填写任务名称，最多 500 字节")
	}
	report, err := s.checkedReview(path, version)
	if err != nil {
		return task, err
	}
	task = ReviewTask{ID: reviewRecordID(), Title: title, StartedAt: time.Now().UTC().Format(time.RFC3339), Start: reviewTaskPoint(report.Snapshot), NewStashes: []GitStash{}, NewUntracked: []string{}}
	err = saveReviewRecord(filepath.Join(s.reviewDirectory(path), "tasks", task.ID+".json"), task)
	return task, err
}

func (s *Service) FinishReviewTask(path, id, version string, input ReviewFinishInput) (ReviewTask, error) {
	s.reviewMu.Lock()
	defer s.reviewMu.Unlock()
	task := ReviewTask{}
	if !pipelineRunID.MatchString(id) || strings.TrimSpace(input.Summary) == "" || len(input.Summary) > 8192 || len(input.Checks) > 8192 || len(input.Blockers) > 8192 {
		return task, errors.New("请填写完成内容，交接各段最多 8 KiB")
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
		return task, errors.New("任务已交接或编号不匹配")
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
	fmt.Fprintf(&text, "# %s\n\n项目：%s\n工作树：%s\n任务：%s\n开始：%s\n交接：%s\n\n## 完成内容\n\n%s\n\n## Git 状态\n\n分支：%s\n起始提交：%s\n当前提交：%s\n已暂存 %d / 未暂存 %d / 未跟踪 %d / stash %d\n", task.Title, report.Project.Name, report.Project.Path, task.ID, task.StartedAt, task.EndedAt, task.Summary, task.End.Branch, task.Start.Head, task.End.Head, task.End.Staged, task.End.Unstaged, len(task.End.Untracked), len(task.End.Stashes))
	if !task.Start.Known || !task.End.Known {
		text.WriteString("\n开始或结束状态不完整，无法可靠区分期间新增内容。\n")
	} else {
		text.WriteString("\n期间新增 stash（不推定创建者）：\n")
		for _, stash := range task.NewStashes {
			fmt.Fprintf(&text, "- %s %s %s\n", stash.Selector, stash.OID, stash.Subject)
		}
		text.WriteString("\n期间新增未跟踪文件：\n")
		for _, name := range task.NewUntracked {
			fmt.Fprintf(&text, "- %s\n", name)
		}
	}
	fmt.Fprintf(&text, "\n## 检查说明（填报）\n\n%s\n\n## 交付状态\n\n", task.Checks)
	for _, stage := range report.Delivery {
		fmt.Fprintf(&text, "- %s [%s]：%s\n", stage.Name, stage.Status, stage.Detail)
	}
	fmt.Fprintf(&text, "\n## 遗留问题\n\n%s\n\n说明：此记录不代表提交、推送、部署或线上验收已经完成；手工核验与应用执行结果分开记录。\n", task.Blockers)
	return text.String()
}

func (s *Service) ReviewHandover(path, id string) (string, error) {
	if _, err := s.registeredProject(path); err != nil {
		return "", err
	}
	if !pipelineRunID.MatchString(id) {
		return "", errors.New("交接记录编号无效")
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
		return "", errors.New("任务尚未交接")
	}
	return task.Handover, nil
}
