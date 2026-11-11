package workspace

import (
	"errors"
	"path/filepath"
	"slices"
	"strings"
)

type ReviewTaskContext struct {
	Title       string   `json:"title"`
	Owner       string   `json:"owner"`
	Areas       []string `json:"areas"`
	Paths       []string `json:"paths"`
	Environment string   `json:"environment"`
	Target      string   `json:"target"`
}

func validateReviewTaskContext(input ReviewTaskContext) error {
	if strings.TrimSpace(input.Title) == "" || len(input.Title) > 500 || len(input.Owner) > 300 || len(input.Paths) > 128 || len(input.Areas) > 3 {
		return errors.New("task name, owner, or scope exceeds limits")
	}
	for _, area := range input.Areas {
		if !slices.Contains([]string{"frontend", "backend", "database"}, area) {
			return errors.New("invalid task scope")
		}
	}
	for _, path := range input.Paths {
		if !filepath.IsLocal(path) || len(path) > 1024 || strings.ContainsRune(path, 0) {
			return errors.New("task file scope must use project-relative paths")
		}
	}
	coverage := []ReviewCoverage{}
	if slices.Contains(input.Areas, "database") {
		coverage = append(coverage, ReviewCoverage{Area: "database", Method: "integration"})
	}
	return validateReviewCoverage(coverage, input.Environment, input.Target)
}

func (s *Service) reviewTaskHistory(path string) ([]ReviewTask, error) {
	entries, err := readReviewRecordsLimit[ReviewTask](filepath.Join(s.reviewDirectory(path), "tasks"), 1000)
	if err != nil {
		return entries, err
	}
	result := []ReviewTask{}
	closed := 0
	for _, task := range entries {
		if task.EndedAt == "" {
			result = append(result, task)
		} else if closed < 20 {
			result = append(result, task)
			closed++
		}
	}
	return result, nil
}

func (s *Service) reviewTaskWarnings(path string, current []ReviewTask) []string {
	warnings := []string{}
	projects, err := s.List()
	if err != nil {
		return []string{"Cannot review active tasks in other projects"}
	}
	type entry struct {
		task    ReviewTask
		path    string
		current bool
	}
	active := []entry{}
	for _, p := range projects {
		tasks := current
		if p.Path != path {
			tasks, err = s.reviewTaskHistory(p.Path)
			if err != nil {
				warnings = append(warnings, p.Name+" task history could not be fully read")
				continue
			}
		}
		for _, task := range tasks {
			if task.EndedAt == "" {
				active = append(active, entry{task: task, path: p.Path, current: p.Path == path})
				if p.Path != path && pathsOverlap(path, p.Path) {
					warnings = append(warnings, "Folder already has an active task: "+task.Title)
				}
			}
		}
	}
	if len(active) > 128 {
		return append(warnings, "More than 128 active tasks; scope conflict review incomplete")
	}
	seen := map[string]bool{}
	comparisons := 0
	for i, a := range active {
		for _, b := range active[i+1:] {
			if !a.current && !b.current {
				continue
			}
			messages := []string{}
			if slices.Contains(a.task.Areas, "database") && slices.Contains(b.task.Areas, "database") && a.task.Environment != "" && a.task.Environment == b.task.Environment && a.task.Target != "" && a.task.Target == b.task.Target {
				messages = append(messages, "Same database environment and target declared; coordinate before acting")
			}
			if pathsOverlap(a.path, b.path) {
				if len(a.task.Paths) == 0 || len(b.task.Paths) == 0 {
					messages = append(messages, "Scope not fully declared; concurrent edits to the same file cannot be ruled out")
				} else {
					overlap := false
					for _, ap := range a.task.Paths {
						for _, bp := range b.task.Paths {
							comparisons++
							if comparisons > 10000 {
								return append(warnings, "Task path comparisons exceeded limits; scope conflict review incomplete")
							}
							if pathsOverlap(filepath.Join(a.path, ap), filepath.Join(b.path, bp)) {
								overlap = true
								break
							}
						}
						if overlap {
							break
						}
					}
					if overlap {
						messages = append(messages, "Declared file scopes overlap; coordinate edits")
					}
				}
			}
			if len(messages) > 0 {
				text := a.task.Title + " / " + b.task.Title + ": " + strings.Join(messages, "; ")
				if !seen[text] {
					warnings = append(warnings, text)
					seen[text] = true
				}
			}
		}
	}
	return warnings
}
