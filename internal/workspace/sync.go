package workspace

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"time"
	"unicode/utf8"
)

const sharedStart = "<!-- repsync:shared:start -->"
const sharedEnd = "<!-- repsync:shared:end -->"

type SharedSource struct {
	Path    string `json:"path"`
	Version string `json:"version"`
	Content string `json:"content"`
}

type UpdatePreview struct {
	Target         string  `json:"target,omitempty"`
	Conflict       bool    `json:"conflict,omitempty"`
	Token          string  `json:"token"`
	Project        Project `json:"project"`
	SourcePath     string  `json:"sourcePath"`
	Version        string  `json:"version"`
	Diff           string  `json:"diff"`
	Changed        bool    `json:"changed"`
	ProjectContent string  `json:"projectContent"`
}

type UpdateHistory struct {
	Target     string  `json:"target,omitempty"`
	ID         string  `json:"id"`
	Project    Project `json:"project"`
	SourcePath string  `json:"sourcePath"`
	Version    string  `json:"version"`
	CreatedAt  string  `json:"createdAt"`
	State      string  `json:"state"`
}

type fileSnapshot struct {
	Content string `json:"content"`
	Exists  bool   `json:"exists"`
	Mode    uint32 `json:"mode"`
}

type updateRecord struct {
	ResourceID    string `json:"resourceId,omitempty"`
	ResourceMode  uint32 `json:"resourceMode,omitempty"`
	Conflict      bool   `json:"conflict,omitempty"`
	PolicyVersion string `json:"policyVersion,omitempty"`
	UpdateHistory
	Before fileSnapshot `json:"before"`
	After  fileSnapshot `json:"after"`
}

type pendingUpdate struct {
	Record  updateRecord
	Expires time.Time
}

func digest(content string) string {
	sum := sha256.Sum256([]byte(content))
	return hex.EncodeToString(sum[:])
}

func (s *Service) syncDir() string { return filepath.Join(filepath.Dir(s.path), "sync") }

func readSharedSource(path string) (SharedSource, error) {
	if !filepath.IsAbs(path) || !markdownFile(path) {
		return SharedSource{}, errors.New("请选择公共规则 Markdown 文件的绝对路径")
	}
	canonical, err := filepath.EvalSymlinks(path)
	if err != nil {
		return SharedSource{}, fmt.Errorf("读取规范源：%w", err)
	}
	info, err := os.Stat(canonical)
	if err != nil {
		return SharedSource{}, err
	}
	if !info.Mode().IsRegular() {
		return SharedSource{}, errors.New("规范源必须是普通文件")
	}
	content, err := readText(canonical)
	if err != nil {
		return SharedSource{}, err
	}
	if !utf8.ValidString(content) || strings.TrimSpace(content) == "" {
		return SharedSource{}, errors.New("规范源必须是非空 UTF-8 文本")
	}
	if strings.Contains(content, sharedStart) || strings.Contains(content, sharedEnd) || strings.Contains(content, policyStart) || strings.Contains(content, policyEnd) {
		return SharedSource{}, errors.New("规范源不能包含 repsync 受管理区块标记，请使用独立的公共规则文件")
	}
	return SharedSource{Path: canonical, Version: digest(content), Content: content}, nil
}

// SharedSource returns the current source and its content fingerprint.
func (s *Service) SharedSource() (SharedSource, error) {
	s.syncMu.Lock()
	defer s.syncMu.Unlock()
	return s.sharedSource()
}

func (s *Service) sharedSource() (SharedSource, error) {
	data, err := os.ReadFile(filepath.Join(s.syncDir(), "source.json"))
	if errors.Is(err, os.ErrNotExist) {
		return SharedSource{}, nil
	}
	if err != nil {
		return SharedSource{}, err
	}
	var config struct {
		Path string `json:"path"`
	}
	if err := json.Unmarshal(data, &config); err != nil {
		return SharedSource{}, fmt.Errorf("规范源配置损坏：%w", err)
	}
	return readSharedSource(config.Path)
}

// ConnectSharedSource registers a source without writing any project files.
func (s *Service) ConnectSharedSource(path string) (SharedSource, error) {
	s.syncMu.Lock()
	defer s.syncMu.Unlock()
	source, err := readSharedSource(strings.TrimSpace(path))
	if err != nil {
		return SharedSource{}, err
	}
	if err := saveJSON(filepath.Join(s.syncDir(), "source.json"), struct {
		Path string `json:"path"`
	}{source.Path}); err != nil {
		return SharedSource{}, err
	}
	s.previews = nil
	return source, nil
}

func readTarget(path string) (fileSnapshot, error) { return readUpdateTarget(path, "AGENTS.md") }

func readUpdateTarget(path, target string) (fileSnapshot, error) {
	root, err := os.OpenRoot(path)
	if err != nil {
		return fileSnapshot{}, err
	}
	defer root.Close()
	if err := validateUpdatePath(root, target); err != nil {
		return fileSnapshot{}, err
	}
	info, err := root.Lstat(target)
	if errors.Is(err, os.ErrNotExist) {
		return fileSnapshot{Mode: 0644}, nil
	}
	if err != nil {
		return fileSnapshot{}, err
	}
	if !info.Mode().IsRegular() {
		return fileSnapshot{}, fmt.Errorf("%s 必须是普通文件，暂不更新符号链接", target)
	}
	file, err := root.Open(target)
	if err != nil {
		return fileSnapshot{}, err
	}
	defer file.Close()
	content, err := readLimitedText(file)
	if err != nil {
		return fileSnapshot{}, err
	}
	if !utf8.ValidString(content) {
		return fileSnapshot{}, fmt.Errorf("%s 不是 UTF-8 文本", target)
	}
	return fileSnapshot{Content: content, Exists: true, Mode: uint32(info.Mode().Perm())}, nil
}

func sharedDocument(before, source string) (string, error) {
	block := sharedStart + "\n" + source
	if !strings.HasSuffix(block, "\n") {
		block += "\n"
	}
	block += sharedEnd
	starts, ends := strings.Count(before, sharedStart), strings.Count(before, sharedEnd)
	var after string
	if starts == 0 && ends == 0 {
		after = block + "\n"
		if before != "" {
			after += "\n" + before
		}
	} else {
		start, end := strings.Index(before, sharedStart), strings.Index(before, sharedEnd)
		if starts != 1 || ends != 1 || end < start {
			return "", errors.New("公共区块标记不完整或重复，请先修复 AGENTS.md")
		}
		after = before[:start] + block + before[end+len(sharedEnd):]
	}
	if len(after) > 256*1024 {
		return "", errors.New("更新后的 AGENTS.md 超过 256 KiB")
	}
	return after, nil
}

// PreviewUpdate creates a short-lived server-owned plan, without changing a project.
func (s *Service) PreviewUpdate(path string) (UpdatePreview, error) {
	s.syncMu.Lock()
	defer s.syncMu.Unlock()
	project, err := s.registeredProject(path)
	if err != nil {
		return UpdatePreview{}, err
	}
	source, err := s.sharedSource()
	if err != nil {
		return UpdatePreview{}, err
	}
	if source.Path == "" {
		return UpdatePreview{}, errors.New("请先接入公共规范源")
	}
	if source.Path == filepath.Join(path, "AGENTS.md") {
		return UpdatePreview{}, errors.New("规范源与项目 AGENTS.md 不能是同一个文件")
	}
	before, err := readTarget(path)
	if err != nil {
		return UpdatePreview{}, err
	}
	after, err := sharedDocument(before.Content, source.Content)
	if err != nil {
		return UpdatePreview{}, err
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
		return UpdatePreview{}, errors.New("待处理预览过多，请稍后重试")
	}
	token := rand.Text()
	record := updateRecord{UpdateHistory: UpdateHistory{ID: token, Project: project, SourcePath: source.Path, Version: source.Version, CreatedAt: now.UTC().Format("2006-01-02T15:04:05.000000000Z"), State: "prepared"}, Before: before, After: fileSnapshot{Content: after, Exists: true, Mode: before.Mode}}
	s.previews[token] = pendingUpdate{Record: record, Expires: now.Add(15 * time.Minute)}
	_, projectContent, _ := sharedParts(before.Content)
	return UpdatePreview{Token: token, Project: project, SourcePath: source.Path, Version: source.Version, Diff: diff, Changed: !sameSnapshot(before, record.After), ProjectContent: projectContent}, nil
}

func sameSnapshot(a, b fileSnapshot) bool {
	return a.Exists == b.Exists && a.Content == b.Content && (!a.Exists || a.Mode == b.Mode)
}

// ApplyUpdate checks the source and target again, then saves a backup before writing.
func (s *Service) ApplyUpdate(token string) (UpdateHistory, error) {
	s.syncMu.Lock()
	defer s.syncMu.Unlock()
	plan, ok := s.previews[token]
	if !ok || time.Now().After(plan.Expires) {
		return UpdateHistory{}, errors.New("预览已过期，请重新预览")
	}
	record := plan.Record
	if _, err := s.registeredProject(record.Project.Path); err != nil {
		return UpdateHistory{}, err
	}
	if record.Conflict {
		return UpdateHistory{}, errors.New("共享文件有项目差异，未覆盖；请先处理差异或改用独立共享文件")
	}
	if record.ResourceID != "" {
		if err := s.validateResourceAdoption(record.Project.Path); err != nil {
			return UpdateHistory{}, err
		}
		source, err := s.projectResource(record.Project.Path, record.ResourceID)
		if err != nil {
			return UpdateHistory{}, err
		}
		if source.Path != record.SourcePath || source.Version != record.Version || source.Target != record.Target || source.Mode != record.ResourceMode {
			return UpdateHistory{}, errors.New("共享资源或采用配置已变化，请重新预览")
		}
	} else if record.PolicyVersion != "" {
		config, err := s.ProjectPolicies(record.Project.Path)
		if err != nil {
			return UpdateHistory{}, err
		}
		if config.Version != record.PolicyVersion {
			return UpdateHistory{}, errors.New("项目规则已变化，请重新预览")
		}
	} else {
		source, err := s.sharedSource()
		if err != nil {
			return UpdateHistory{}, err
		}
		if source.Path != record.SourcePath || source.Version != record.Version {
			return UpdateHistory{}, errors.New("公共规范已变化，请重新预览")
		}
	}
	current, err := readUpdateTarget(record.Project.Path, updateTarget(record))
	if err != nil {
		return UpdateHistory{}, err
	}
	if !sameSnapshot(current, record.Before) {
		return UpdateHistory{}, errors.New("项目文件已变化，请重新预览；已有改动未被覆盖")
	}
	if sameSnapshot(record.Before, record.After) {
		return UpdateHistory{}, errors.New("内容已是最新，无需应用")
	}
	recordPath := filepath.Join(s.syncDir(), "history", record.ID+".json")
	if err := saveJSON(recordPath, record); err != nil {
		return UpdateHistory{}, fmt.Errorf("保存更新备份：%w", err)
	}
	if err := replaceUpdateTarget(record.Project.Path, updateTarget(record), record.Before, record.After); err != nil {
		return UpdateHistory{}, err
	}
	delete(s.previews, token)
	record.State = "applied"
	if err := saveJSON(recordPath, record); err != nil {
		return UpdateHistory{}, fmt.Errorf("文件已更新，备份已保存；完成状态记录失败：%w", err)
	}
	return record.UpdateHistory, nil
}

// UpdateHistory derives each operation's recoverability from the current file.
func (s *Service) UpdateHistory(path string) ([]UpdateHistory, error) {
	return s.updateHistory(path, false)
}
func (s *Service) ResourceHistory(path string) ([]UpdateHistory, error) {
	return s.updateHistory(path, true)
}
func (s *Service) updateHistory(path string, resources bool) ([]UpdateHistory, error) {
	s.syncMu.Lock()
	defer s.syncMu.Unlock()
	if _, err := s.registeredProject(path); err != nil {
		return nil, err
	}
	entries, err := os.ReadDir(filepath.Join(s.syncDir(), "history"))
	if errors.Is(err, os.ErrNotExist) {
		return []UpdateHistory{}, nil
	}
	if err != nil {
		return nil, err
	}
	history := []UpdateHistory{}
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".json") {
			continue
		}
		record, err := s.readRecord(strings.TrimSuffix(entry.Name(), ".json"))
		if err != nil {
			return nil, err
		}
		if record.Project.Path != path || (record.ResourceID != "") != resources {
			continue
		}
		current, err := readUpdateTarget(path, updateTarget(record))
		if err != nil {
			record.State = "unavailable"
			history = append(history, record.UpdateHistory)
			continue
		}
		if sameSnapshot(current, record.After) {
			record.State = "applied"
		} else if sameSnapshot(current, record.Before) {
			if record.State == "prepared" {
				record.State = "not-applied"
			} else {
				record.State = "restored"
			}
		} else {
			record.State = "changed"
		}
		history = append(history, record.UpdateHistory)
	}
	slices.SortFunc(history, func(a, b UpdateHistory) int { return strings.Compare(b.CreatedAt, a.CreatedAt) })
	return history, nil
}

func (s *Service) readRecord(id string) (updateRecord, error) {
	if id == "" || filepath.Base(id) != id || strings.ContainsAny(id, "/\\") {
		return updateRecord{}, errors.New("无效的更新记录")
	}
	data, err := os.ReadFile(filepath.Join(s.syncDir(), "history", id+".json"))
	if err != nil {
		return updateRecord{}, err
	}
	var record updateRecord
	if err := json.Unmarshal(data, &record); err != nil {
		return updateRecord{}, fmt.Errorf("更新记录损坏：%w", err)
	}
	if record.ID != id {
		return updateRecord{}, errors.New("更新记录标识不一致")
	}
	return record, nil
}

// RestoreUpdate restores the exact backup only while the applied file is unchanged.
func (s *Service) RestoreUpdate(id string) (UpdateHistory, error) {
	s.syncMu.Lock()
	defer s.syncMu.Unlock()
	record, err := s.readRecord(id)
	if err != nil {
		return UpdateHistory{}, err
	}
	if _, err := s.registeredProject(record.Project.Path); err != nil {
		return UpdateHistory{}, err
	}
	if err := replaceUpdateTarget(record.Project.Path, updateTarget(record), record.After, record.Before); err != nil {
		return UpdateHistory{}, err
	}
	record.State = "restored"
	if err := saveJSON(filepath.Join(s.syncDir(), "history", id+".json"), record); err != nil {
		return UpdateHistory{}, fmt.Errorf("文件已恢复，状态记录失败：%w", err)
	}
	return record.UpdateHistory, nil
}

func replaceUpdateTarget(path, target string, expected, next fileSnapshot) error {
	root, err := os.OpenRoot(path)
	if err != nil {
		return err
	}
	defer root.Close()
	if err := validateUpdatePath(root, target); err != nil {
		return err
	}
	// The final recheck narrows the window between preview validation and atomic replacement.
	current, err := readUpdateTarget(path, target)
	if err != nil {
		return err
	}
	if !sameSnapshot(current, expected) {
		return errors.New("项目文件已有后续改动，操作已停止")
	}
	if !next.Exists {
		return root.Remove(target)
	}
	temp := ".repsync-" + rand.Text()
	file, err := root.OpenFile(temp, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
	if err != nil {
		return err
	}
	defer root.Remove(temp)
	if _, err := file.WriteString(next.Content); err != nil {
		file.Close()
		return err
	}
	if err := file.Chmod(os.FileMode(next.Mode)); err != nil {
		file.Close()
		return err
	}
	if err := file.Sync(); err != nil {
		file.Close()
		return err
	}
	if err := file.Close(); err != nil {
		return err
	}
	current, err = readUpdateTarget(path, target)
	if err != nil {
		return err
	}
	if !sameSnapshot(current, expected) {
		return errors.New("项目文件已有后续改动，操作已停止")
	}
	return root.Rename(temp, target)
}

func saveJSON(path string, value any) error {
	data, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		return err
	}
	file, err := os.CreateTemp(filepath.Dir(path), ".repsync-*")
	if err != nil {
		return err
	}
	defer os.Remove(file.Name())
	if _, err := file.Write(data); err != nil {
		file.Close()
		return err
	}
	if err := file.Sync(); err != nil {
		file.Close()
		return err
	}
	if err := file.Close(); err != nil {
		return err
	}
	if err := os.Rename(file.Name(), path); err != nil {
		return err
	}
	dir, err := os.Open(filepath.Dir(path))
	if err != nil {
		return err
	}
	defer dir.Close()
	return dir.Sync()
}

func documentDiff(before, after string) (string, error) {
	if before == after {
		return "", nil
	}
	dir, err := os.MkdirTemp("", "repsync-diff-*")
	if err != nil {
		return "", err
	}
	defer os.RemoveAll(dir)
	a, b := filepath.Join(dir, "before"), filepath.Join(dir, "after")
	if err := os.WriteFile(a, []byte(before), 0600); err != nil {
		return "", err
	}
	if err := os.WriteFile(b, []byte(after), 0600); err != nil {
		return "", err
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, "git", "--no-pager", "diff", "--no-index", "--no-ext-diff", "--no-textconv", "--no-color", "--", a, b)
	for _, entry := range os.Environ() {
		if !strings.HasPrefix(entry, "GIT_") {
			cmd.Env = append(cmd.Env, entry)
		}
	}
	var out, stderr limitedBuffer
	cmd.Stdout, cmd.Stderr = &out, &stderr
	err = cmd.Run()
	var exit *exec.ExitError
	if err != nil && !(errors.As(err, &exit) && exit.ExitCode() == 1) {
		return "", fmt.Errorf("生成差异：%w %s", err, stderr.String())
	}
	if out.overflow {
		return "", errors.New("差异超过显示限制")
	}
	diff := strings.ReplaceAll(out.String(), a, "/AGENTS.md")
	diff = strings.ReplaceAll(diff, b, "/AGENTS.md")
	return diff, nil
}
