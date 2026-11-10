package workspace

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
)

type SharedAdoption struct {
	Project           Project `json:"project"`
	Status            string  `json:"status"`
	SourcePath        string  `json:"sourcePath"`
	SourceVersion     string  `json:"sourceVersion"`
	AdoptedVersion    string  `json:"adoptedVersion"`
	HasProjectContent bool    `json:"hasProjectContent"`
	Error             string  `json:"error"`
}

func sharedParts(content string) (block, project string, err error) {
	starts, ends := strings.Count(content, sharedStart), strings.Count(content, sharedEnd)
	if starts == 0 && ends == 0 {
		return "", content, nil
	}
	start, end := strings.Index(content, sharedStart), strings.Index(content, sharedEnd)
	if starts != 1 || ends != 1 || end < start {
		return "", "", errors.New("公共区块标记不完整或重复，请先修复 AGENTS.md")
	}
	return content[start : end+len(sharedEnd)], content[:start] + content[end+len(sharedEnd):], nil
}

// SharedAdoptions reads actual managed blocks and backup evidence, without creating update previews.
func (s *Service) SharedAdoptions() ([]SharedAdoption, error) {
	s.syncMu.Lock()
	defer s.syncMu.Unlock()
	projects, err := s.List()
	if err != nil {
		return nil, err
	}
	source, err := s.sharedSource()
	if err != nil {
		return nil, err
	}
	entries, err := os.ReadDir(filepath.Join(s.syncDir(), "history"))
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return nil, err
	}
	records := []updateRecord{}
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".json") {
			continue
		}
		record, err := s.readRecord(strings.TrimSuffix(entry.Name(), ".json"))
		if err != nil {
			return nil, err
		}
		if record.PolicyVersion == "" && record.ResourceID == "" {
			records = append(records, record)
		}
	}
	expected, _ := sharedDocument("", source.Content)
	expectedBlock, _, _ := sharedParts(expected)
	results := []SharedAdoption{}
	for _, project := range projects {
		item := SharedAdoption{Project: project, SourcePath: source.Path, SourceVersion: source.Version, Status: "not_adopted"}
		target, err := readTarget(project.Path)
		if err != nil {
			item.Status, item.Error = "error", err.Error()
			results = append(results, item)
			continue
		}
		block, own, err := sharedParts(target.Content)
		item.HasProjectContent = strings.TrimSpace(own) != ""
		if err != nil {
			item.Status, item.Error = "error", err.Error()
		} else if source.Path == "" {
			item.Status = "no_source"
		} else if block != "" {
			item.Status = "untracked"
			latest := ""
			known := false
			for _, record := range records {
				if record.Project.Path != project.Path {
					continue
				}
				known = true
				after, _, parseErr := sharedParts(record.After.Content)
				if parseErr == nil && after == block && record.CreatedAt > latest {
					latest, item.AdoptedVersion = record.CreatedAt, record.Version
					item.Status = "outdated"
				}
			}
			if item.AdoptedVersion == "" && known {
				item.Status = "modified"
			}
			if block == expectedBlock {
				item.Status, item.AdoptedVersion = "current", source.Version
			}
		}
		results = append(results, item)
	}
	return results, nil
}
