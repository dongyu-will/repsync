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
	"unicode/utf8"
)

const maxProjectMemories = 256

type ProjectMemoryInput struct {
	Statement   string   `json:"statement"`
	Source      string   `json:"source"`
	Environment string   `json:"environment"`
	Keywords    []string `json:"keywords"`
}

type ProjectMemory struct {
	ProjectMemoryInput
	ID           string               `json:"id"`
	Origin       string               `json:"origin"`
	Status       string               `json:"status"`
	CreatedAt    string               `json:"createdAt"`
	UpdatedAt    string               `json:"updatedAt"`
	ConfirmedAt  string               `json:"confirmedAt"`
	ArchivedFrom string               `json:"archivedFrom"`
	Version      string               `json:"version"`
	Session      *CodexMemoryCitation `json:"session,omitempty"`
}

func (s *Service) projectMemoryPath(path string) string {
	return filepath.Join(filepath.Dir(s.path), "memory", gitDigest(path)+".json")
}

func (s *Service) readProjectMemory(path string) ([]ProjectMemory, error) {
	items := []ProjectMemory{}
	data, err := readReviewData(s.projectMemoryPath(path))
	if errors.Is(err, os.ErrNotExist) {
		return items, nil
	}
	if err != nil {
		return nil, fmt.Errorf("读取项目记忆: %w", err)
	}
	if err := json.Unmarshal(data, &items); err != nil || items == nil || len(items) > maxProjectMemories {
		return nil, errors.New("项目记忆文件无法读取")
	}
	ids := map[string]bool{}
	for i := range items {
		item := &items[i]
		if item.ID == "" || ids[item.ID] || item.Source == "" || item.Statement == "" ||
			!slices.Contains([]string{"pending", "confirmed", "archived"}, item.Status) ||
			(item.Status == "archived" && !slices.Contains([]string{"pending", "confirmed"}, item.ArchivedFrom)) ||
			(item.Status == "confirmed" && item.ConfirmedAt == "") {
			return nil, errors.New("项目记忆中有无效记录")
		}
		ids[item.ID] = true
		item.Version = ""
		item.Version = gitDigest(*item)
	}
	slices.SortFunc(items, func(a, b ProjectMemory) int { return strings.Compare(b.UpdatedAt, a.UpdatedAt) })
	return items, nil
}

// ProjectMemories lists the registered project's records, including pending and archived ones.
func (s *Service) ProjectMemories(path string) ([]ProjectMemory, error) {
	if _, err := s.registeredProject(path); err != nil {
		return nil, err
	}
	return s.readProjectMemory(path)
}

// ConfirmedProjectMemories returns the current records suitable for use as context.
func (s *Service) ConfirmedProjectMemories(path string) ([]ProjectMemory, error) {
	items, err := s.ProjectMemories(path)
	return confirmedProjectMemories(items), err
}

func confirmedProjectMemories(items []ProjectMemory) []ProjectMemory {
	confirmed := []ProjectMemory{}
	for _, item := range items {
		if item.Status == "confirmed" {
			confirmed = append(confirmed, item)
		}
	}
	return confirmed
}

// RecordProjectMemory saves a user-entered record as confirmed.
func (s *Service) RecordProjectMemory(path string, input ProjectMemoryInput) (ProjectMemory, error) {
	return s.addProjectMemory(path, input, "user", "confirmed")
}

// ProposeProjectMemory records an AI inference for user review.
func (s *Service) ProposeProjectMemory(path string, input ProjectMemoryInput) (ProjectMemory, error) {
	return s.addProjectMemory(path, input, "ai", "pending")
}

func (s *Service) addProjectMemory(path string, input ProjectMemoryInput, origin, status string) (ProjectMemory, error) {
	var item ProjectMemory
	if _, err := s.registeredProject(path); err != nil {
		return item, err
	}
	input, err := normalizeMemoryInput(input)
	if err != nil {
		return item, err
	}
	s.memoryMu.Lock()
	defer s.memoryMu.Unlock()
	items, err := s.readProjectMemory(path)
	if err != nil {
		return item, err
	}
	for _, existing := range items {
		if existing.Origin == origin && existing.Statement == input.Statement && existing.Source == input.Source &&
			existing.Environment == input.Environment && slices.Equal(existing.Keywords, input.Keywords) {
			return existing, nil
		}
	}
	if len(items) >= maxProjectMemories {
		return item, errors.New("项目记忆达到 256 条上限")
	}
	item = newProjectMemory(input, origin, status, nil)
	return item, s.saveProjectMemories(path, append(items, item))
}

func normalizeMemoryInput(input ProjectMemoryInput) (ProjectMemoryInput, error) {
	input.Statement = strings.TrimSpace(input.Statement)
	input.Source = strings.TrimSpace(input.Source)
	input.Environment = strings.TrimSpace(input.Environment)
	if input.Statement == "" || input.Source == "" || len(input.Statement) > 2048 || len(input.Source) > 1024 || len(input.Environment) > 256 {
		return input, errors.New("请填写记忆内容和来源，并检查字段长度")
	}
	if !utf8.ValidString(input.Statement) || !utf8.ValidString(input.Source) || !utf8.ValidString(input.Environment) {
		return input, errors.New("项目记忆需要使用 UTF-8 文本")
	}
	if len(input.Keywords) > 12 {
		return input, errors.New("最多填写 12 个文档关键词")
	}
	keywords := []string{}
	for _, keyword := range input.Keywords {
		keyword = strings.TrimSpace(keyword)
		if keyword == "" {
			continue
		}
		if len(keyword) > 100 || !utf8.ValidString(keyword) || strings.ContainsAny(keyword, "\x00\r\n") {
			return input, errors.New("文档关键词需要是 100 字节以内的单行文本")
		}
		if !slices.ContainsFunc(keywords, func(existing string) bool { return strings.EqualFold(existing, keyword) }) {
			keywords = append(keywords, keyword)
		}
	}
	input.Keywords = keywords
	return input, nil
}

func newProjectMemory(input ProjectMemoryInput, origin, status string, session *CodexMemoryCitation) ProjectMemory {
	now := time.Now().UTC().Format(time.RFC3339Nano)
	item := ProjectMemory{
		ProjectMemoryInput: input, ID: rand.Text(), Origin: origin,
		Status: status, CreatedAt: now, UpdatedAt: now, Session: session,
	}
	if status == "confirmed" {
		item.ConfirmedAt = now
	}
	item.Version = gitDigest(item)
	return item
}

// SetProjectMemoryState confirms, archives or restores a reviewed version.
func (s *Service) SetProjectMemoryState(path, id, version, action string) (ProjectMemory, error) {
	if _, err := s.registeredProject(path); err != nil {
		return ProjectMemory{}, err
	}
	s.memoryMu.Lock()
	defer s.memoryMu.Unlock()
	items, err := s.readProjectMemory(path)
	if err != nil {
		return ProjectMemory{}, err
	}
	index := slices.IndexFunc(items, func(item ProjectMemory) bool { return item.ID == id })
	if index < 0 || items[index].Version != version {
		return ProjectMemory{}, errors.New("记忆已变化，请重新载入后再操作")
	}
	item := &items[index]
	now := time.Now().UTC().Format(time.RFC3339Nano)
	switch {
	case action == "confirm" && item.Status == "pending":
		item.Status, item.ConfirmedAt = "confirmed", now
	case action == "archive" && item.Status != "archived":
		item.ArchivedFrom, item.Status = item.Status, "archived"
	case action == "restore" && item.Status == "archived":
		item.Status, item.ArchivedFrom = item.ArchivedFrom, ""
	default:
		return ProjectMemory{}, errors.New("此记忆状态不支持该操作")
	}
	item.UpdatedAt = now
	item.Version = ""
	item.Version = gitDigest(*item)
	return *item, s.saveProjectMemories(path, items)
}

func (s *Service) saveProjectMemories(path string, items []ProjectMemory) error {
	data, err := json.MarshalIndent(items, "", "  ")
	if err != nil {
		return err
	}
	if len(data) > 1024*1024 {
		return errors.New("项目记忆达到 1 MiB 上限")
	}
	return saveJSON(s.projectMemoryPath(path), items)
}
