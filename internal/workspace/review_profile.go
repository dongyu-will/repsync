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
	{ID: "frontend-static", Name: "前端类型与静态检查", Area: "frontend", Method: "static"},
	{ID: "frontend-behavior", Name: "浏览器与交互验收", Area: "frontend", Method: "behavior"},
	{ID: "backend-static", Name: "后端静态检查", Area: "backend", Method: "static"},
	{ID: "backend-api", Name: "真实 HTTP / API 集成", Area: "backend", Method: "integration"},
	{ID: "database-integration", Name: "真实数据库集成", Area: "database", Method: "integration"},
	{ID: "database-migrations", Name: "迁移状态与兼容性", Area: "database", Method: "migration"},
	{ID: "database-consistency", Name: "数据一致性与关键业务约束", Area: "database", Method: "consistency"},
	{ID: "database-recovery", Name: "备份与恢复演练", Area: "database", Method: "recovery"},
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
		return profile, false, errors.New("项目适用项配置无效")
	}
	return profile, true, validateReviewProfile(profile)
}

func validateReviewProfile(profile ReviewProfile) error {
	if !slices.Contains([]string{"web", "backend", "desktop", "library", "notes", "custom"}, profile.Kind) {
		return errors.New("项目类型无效")
	}
	seen := map[string]bool{}
	for _, area := range profile.Areas {
		if !slices.Contains([]string{"frontend", "backend", "database"}, area) || seen["area:"+area] {
			return errors.New("检查范围无效或重复")
		}
		seen["area:"+area] = true
	}
	for _, id := range profile.Requirements {
		index := slices.IndexFunc(reviewRequirements, func(r ReviewRequirement) bool { return r.ID == id })
		if index < 0 || seen["requirement:"+id] || !slices.Contains(profile.Areas, reviewRequirements[index].Area) {
			return errors.New("必需检查项无效、重复或不属于适用范围")
		}
		seen["requirement:"+id] = true
	}
	for _, stage := range profile.Stages {
		if !slices.Contains([]string{"push", "ci", "deploy", "acceptance"}, stage) || seen["stage:"+stage] {
			return errors.New("交付阶段无效或重复")
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
	if len(coverage) > 8 || len(environment) > 200 || len(target) > 200 {
		return errors.New("检查范围或环境说明超出限制")
	}
	seen := map[string]bool{}
	for _, c := range coverage {
		valid := slices.ContainsFunc(reviewRequirements, func(r ReviewRequirement) bool { return r.Area == c.Area && r.Method == c.Method })
		// SQL/static checks can be recorded, but do not satisfy real database requirements.
		if c.Area == "database" && c.Method == "static" {
			valid = true
		}
		if !valid || seen[c.Area+":"+c.Method] {
			return errors.New("检查范围或方法无效、重复")
		}
		seen[c.Area+":"+c.Method] = true
		if c.Area == "database" && (strings.TrimSpace(environment) == "" || strings.TrimSpace(target) == "") {
			return errors.New("数据库检查需要环境和目标别名，不能只填写命令通过")
		}
	}
	for _, value := range []string{environment, target} {
		lower := strings.ToLower(value)
		if strings.Contains(lower, "://") || strings.Contains(lower, "password=") || strings.Contains(lower, "pwd=") {
			return errors.New("环境与目标只填写别名，不填写连接串或密码")
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
		state := ReviewCoverageState{Requirement: requirement, Status: "pending", Reason: "尚无对应范围和方法的检查记录"}
		for _, check := range checks {
			if !slices.ContainsFunc(check.Coverage, func(c ReviewCoverage) bool { return c.Area == requirement.Area && c.Method == requirement.Method }) {
				continue
			}
			state.CheckID, state.Source, state.Evidence, state.Environment, state.Target, state.RecordedAt = check.ID, check.Source, check.Evidence, check.Environment, check.Target, check.CreatedAt
			state.Status, state.Reason = check.Current, check.Reason
			if check.Current == "current" {
				if check.Status == "success" {
					state.Status = "recorded"
					state.Reason = "有当前内容的记录；覆盖范围与环境为声明信息"
					if check.Source == "pipeline" {
						state.Status = "passed"
						state.Reason = "命令通过；需核对声明范围是否实际执行"
					}
				} else if check.Status == "failure" {
					state.Status = "failed"
					state.Reason = "此项检查未通过"
				} else {
					state.Status = "pending"
					state.Reason = "此项检查尚未完成"
				}
			}
			if requirement.Area == "database" && (check.Environment == "" || check.Target == "") {
				state.Status, state.Reason = "unknown", "数据库环境或目标未记录，无法核验"
			}
			break
		}
		states = append(states, state)
	}
	return states
}
