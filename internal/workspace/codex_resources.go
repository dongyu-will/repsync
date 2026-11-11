package workspace

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"reflect"
	"regexp"
	"slices"
	"strings"
	"syscall"

	"github.com/pelletier/go-toml/v2"
)

const codexProjectConfig = ".codex/config.toml"

var codexResourceName = regexp.MustCompile(`^[a-zA-Z0-9_-]{1,64}$`)
var codexEnvName = regexp.MustCompile(`^[a-zA-Z_][a-zA-Z0-9_]*$`)

type CodexResourceChange struct {
	Kind     string   `json:"kind"`
	Name     string   `json:"name"`
	Action   string   `json:"action"`
	Source   string   `json:"source"`
	URL      string   `json:"url"`
	TokenEnv string   `json:"tokenEnv"`
	Command  string   `json:"command"`
	Args     []string `json:"args"`
}

type CodexResourcePlacement struct {
	Kind   string `json:"kind"`
	Name   string `json:"name"`
	Target string `json:"target"`
	Source string `json:"source"`
	State  string `json:"state"`
}

type CodexResourcePreview struct {
	Version string `json:"version"`
	Target  string `json:"target"`
	Before  string `json:"before"`
	After   string `json:"after"`
}

type CodexResourcePlacements struct {
	Items []CodexResourcePlacement `json:"items"`
	Issue string                   `json:"issue"`
}

type codexSkillRecord struct {
	Source string `json:"source"`
	Device int32  `json:"device"`
	Inode  uint64 `json:"inode"`
	Mtime  int64  `json:"mtime"`
}

type codexPlacementPlan struct {
	preview       CodexResourcePreview
	before, after fileSnapshot
	link          codexSkillRecord
	userDirectory string
}

func (s *Service) codexSkillRecords(path string) string {
	return filepath.Join(s.syncDir(), "codex-skills", digest(path))
}

// ProjectCodexResources lists placements owned by RepSync and their current state.
func (s *Service) ProjectCodexResources(path string) (CodexResourcePlacements, error) {
	s.syncMu.Lock()
	defer s.syncMu.Unlock()
	if _, err := s.registeredProject(path); err != nil {
		return CodexResourcePlacements{}, err
	}
	root, err := os.OpenRoot(path)
	if err != nil {
		return CodexResourcePlacements{}, err
	}
	defer root.Close()
	items := []CodexResourcePlacement{}
	directory := s.codexSkillRecords(path)
	records, err := os.ReadDir(directory)
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return CodexResourcePlacements{}, err
	}
	for _, entry := range records {
		name := strings.TrimSuffix(entry.Name(), ".json")
		if name == entry.Name() || !codexResourceName.MatchString(name) {
			continue
		}
		var record codexSkillRecord
		target := codexSkillTarget(name)
		state := "changed"
		if err := readCodexSkillRecord(directory, name, &record); err == nil {
			if current, err := readCodexSkillLink(root, target); err == nil && current == record {
				state = "applied"
				if _, err := os.Stat(filepath.Join(record.Source, "SKILL.md")); err != nil {
					state = "source_missing"
				}
			} else if errors.Is(err, os.ErrNotExist) {
				state = "missing"
			}
		}
		items = append(items, CodexResourcePlacement{"skill", name, target, record.Source, state})
	}
	snapshot, err := readCodexProjectConfig(path, root)
	if err != nil {
		return CodexResourcePlacements{Items: items, Issue: err.Error()}, nil
	}
	tables, err := codexMCPEntries(snapshot.Content)
	if err != nil {
		return CodexResourcePlacements{Items: items, Issue: err.Error()}, nil
	}
	for name, value := range tables {
		block, owned, err := managedCodexMCP(path, name, snapshot.Content)
		if owned {
			state := "applied"
			if err != nil || block == "" {
				state = "changed"
			} else if entry, ok := value.(map[string]any); ok && entry["enabled"] == false {
				state = "disabled"
			}
			source := ""
			if codexMCPOverride(value) {
				source = "user"
			}
			items = append(items, CodexResourcePlacement{"mcp", name, codexProjectConfig, source, state})
		}
	}
	slices.SortFunc(items, func(a, b CodexResourcePlacement) int {
		if a.Kind == b.Kind {
			return strings.Compare(a.Name, b.Name)
		}
		return strings.Compare(a.Kind, b.Kind)
	})
	return CodexResourcePlacements{Items: items}, nil
}

// PreviewCodexResource plans one placement or removal without writing the project.
func (s *Service) PreviewCodexResource(path string, change CodexResourceChange) (CodexResourcePreview, error) {
	s.syncMu.Lock()
	defer s.syncMu.Unlock()
	plan, err := s.planCodexResource(path, change)
	return plan.preview, err
}

// ApplyCodexResource rechecks the preview before writing only its target.
func (s *Service) ApplyCodexResource(path string, change CodexResourceChange, version string) error {
	s.syncMu.Lock()
	defer s.syncMu.Unlock()
	plan, err := s.planCodexResource(path, change)
	if err != nil {
		return err
	}
	if plan.preview.Version != version {
		return errors.New("来源或目标已变化，请重新预览")
	}
	if plan.userDirectory != "" {
		return replaceUpdateTarget(plan.userDirectory, "config.toml", plan.before, plan.after)
	}
	root, err := os.OpenRoot(path)
	if err != nil {
		return err
	}
	defer root.Close()
	if err := codexResourceParents(root, plan.preview.Target, change.Action != "remove"); err != nil {
		return err
	}
	if change.Kind == "mcp" {
		return replaceUpdateTarget(path, plan.preview.Target, plan.before, plan.after)
	}
	recordPath := filepath.Join(s.codexSkillRecords(path), change.Name+".json")
	if change.Action == "remove" {
		current, err := readCodexSkillLink(root, plan.preview.Target)
		switch {
		case err == nil && current == plan.link:
			if err := root.Remove(plan.preview.Target); err != nil {
				return err
			}
		case errors.Is(err, os.ErrNotExist):
			// A manually removed link only needs its placement record cleared.
		default:
			return errors.New("项目 Skill 已变化，请核对原位置")
		}
		return os.Remove(recordPath)
	}
	if err := root.Symlink(plan.link.Source, plan.preview.Target); err != nil {
		return fmt.Errorf("建立项目 Skill 链接: %w", err)
	}
	current, err := readCodexSkillLink(root, plan.preview.Target)
	if err != nil || current.Source != plan.link.Source {
		return errors.New("项目 Skill 链接无法核验，请检查目标")
	}
	if err := saveJSON(recordPath, current); err != nil {
		if link, check := readCodexSkillLink(root, plan.preview.Target); check == nil && link == current {
			err = errors.Join(err, root.Remove(plan.preview.Target))
		}
		return fmt.Errorf("保存 Skill 接入记录: %w", err)
	}
	return nil
}

func (s *Service) planCodexResource(path string, change CodexResourceChange) (codexPlacementPlan, error) {
	var plan codexPlacementPlan
	if !codexResourceName.MatchString(change.Name) {
		return plan, errors.New("请输入 1–64 位字母、数字、下划线或连字符作为名称")
	}
	if !slices.Contains([]string{"add", "remove", "enable", "disable"}, change.Action) ||
		(change.Kind != "mcp" && (change.Action == "enable" || change.Action == "disable")) {
		return plan, errors.New("请选择有效的接入操作")
	}
	if path == "" {
		if change.Kind != "mcp" || (change.Action != "enable" && change.Action != "disable") {
			return plan, errors.New("用户默认值仅支持启停已有 MCP")
		}
		paths, err := currentAIPaths()
		if err != nil {
			return plan, err
		}
		return planCodexUserMCP(paths.codex, change)
	}
	if _, err := s.registeredProject(path); err != nil {
		return plan, err
	}
	root, err := os.OpenRoot(path)
	if err != nil {
		return plan, err
	}
	defer root.Close()
	switch change.Kind {
	case "skill":
		target := codexSkillTarget(change.Name)
		plan.preview.Target = target
		if err := codexResourceParents(root, target, false); err != nil {
			return plan, err
		}
		directory := s.codexSkillRecords(path)
		var record codexSkillRecord
		recordErr := readCodexSkillRecord(directory, change.Name, &record)
		current, linkErr := readCodexSkillLink(root, target)
		if change.Action == "remove" {
			if recordErr != nil {
				return plan, errors.New("没有此 Skill 的接入记录")
			}
			if !errors.Is(linkErr, os.ErrNotExist) && (linkErr != nil || current != record) {
				return plan, errors.New("项目 Skill 不是原先创建的链接，请核对目标")
			}
			plan.link = record
			plan.preview.Before = record.Source
			plan.preview.Version = gitDigest([]any{path, change, record, current})
			return plan, nil
		}
		if !errors.Is(recordErr, os.ErrNotExist) || !errors.Is(linkErr, os.ErrNotExist) {
			return plan, errors.New("已有同名项目 Skill 或接入记录，请先核对")
		}
		source, version, err := codexSkillSource(change.Source, change.Name)
		if err != nil {
			return plan, err
		}
		if filepath.Clean(filepath.Join(path, target)) == source {
			return plan, errors.New("Skill 已位于项目目标位置")
		}
		plan.link.Source = source
		plan.preview.After = source
		plan.preview.Version = gitDigest([]any{path, change, source, version})
		return plan, nil
	case "mcp":
		var userDirectory string
		if change.Action == "enable" || change.Action == "disable" {
			paths, err := currentAIPaths()
			if err != nil {
				return plan, err
			}
			userDirectory = paths.codex
		}
		return planCodexMCP(path, root, userDirectory, change)
	default:
		return plan, errors.New("仅支持项目 Skill 和 MCP")
	}
}

func codexSkillTarget(name string) string {
	return filepath.Join(".agents", "skills", name)
}

func codexSkillSource(path, name string) (string, string, error) {
	if !filepath.IsAbs(path) || filepath.Base(path) != "SKILL.md" {
		return "", "", errors.New("请选择已盘点的 SKILL.md")
	}
	path, err := filepath.EvalSymlinks(path)
	if err != nil {
		return "", "", err
	}
	info, err := os.Stat(path)
	if err != nil {
		return "", "", err
	}
	if !info.Mode().IsRegular() || filepath.Base(path) != "SKILL.md" {
		return "", "", errors.New("Skill 来源需要是普通 SKILL.md 文件")
	}
	content, err := readText(path)
	if err != nil {
		return "", "", err
	}
	if skillField(content, "name") != name {
		return "", "", errors.New("Skill 名称已变化，请重新盘点")
	}
	return filepath.Dir(path), digest(content), nil
}

func readCodexSkillRecord(directory, name string, record *codexSkillRecord) error {
	content, err := readText(filepath.Join(directory, name+".json"))
	if err != nil {
		return err
	}
	return json.Unmarshal([]byte(content), record)
}

func readCodexSkillLink(root *os.Root, target string) (codexSkillRecord, error) {
	info, err := root.Lstat(target)
	if err != nil {
		return codexSkillRecord{}, err
	}
	stat, ok := info.Sys().(*syscall.Stat_t)
	if !ok || info.Mode()&os.ModeSymlink == 0 {
		return codexSkillRecord{}, errors.New("同名项目 Skill 已存在")
	}
	link, err := root.Readlink(target)
	return codexSkillRecord{Source: link, Device: int32(stat.Dev), Inode: stat.Ino, Mtime: info.ModTime().UnixNano()}, err
}

func codexResourceParents(root *os.Root, target string, create bool) error {
	current := ""
	for _, part := range strings.Split(filepath.Dir(target), string(filepath.Separator)) {
		current = filepath.Join(current, part)
		info, err := root.Lstat(current)
		if errors.Is(err, os.ErrNotExist) {
			if !create {
				return nil
			}
			if err := root.Mkdir(current, 0755); err != nil && !errors.Is(err, os.ErrExist) {
				return err
			}
			info, err = root.Lstat(current)
		}
		if err != nil {
			return err
		}
		if !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
			return fmt.Errorf("%s 需要是普通目录", current)
		}
	}
	return nil
}

func readCodexProjectConfig(path string, root *os.Root) (fileSnapshot, error) {
	if err := codexResourceParents(root, codexProjectConfig, false); err != nil {
		return fileSnapshot{}, err
	}
	if _, err := root.Lstat(codexProjectConfig); errors.Is(err, os.ErrNotExist) {
		return fileSnapshot{Mode: 0600}, nil
	}
	return readUpdateTarget(path, codexProjectConfig)
}

func codexMCPEntries(text string) (map[string]any, error) {
	var config struct {
		MCP map[string]any `toml:"mcp_servers"`
	}
	if err := toml.Unmarshal([]byte(text), &config); err != nil {
		return nil, errors.New("项目 Codex 配置无法解析，请先修复 TOML")
	}
	return config.MCP, nil
}

func codexMCPMarkers(path, name string) (string, string) {
	id := digest(path + "\x00" + name)[:16]
	return "# repsync:codex-mcp " + id + " ", "# repsync:codex-mcp end " + id
}

// managedCodexMCP finds the intact block written for this project and server.
func managedCodexMCP(path, name, text string) (string, bool, error) {
	prefix, end := codexMCPMarkers(path, name)
	offset := strings.Index(text, prefix)
	if offset < 0 {
		return "", false, nil
	}
	startLine, rest, ok := strings.Cut(text[offset:], "\n")
	body, tail, found := strings.Cut(rest, end+"\n")
	if !ok || !found || (offset > 0 && text[offset-1] != '\n') || strings.Contains(tail, prefix) ||
		strings.TrimPrefix(startLine, prefix) != digest(body) {
		return "", true, errors.New("MCP 接入区块已被修改，请核对项目配置")
	}
	return startLine + "\n" + body + end + "\n", true, nil
}

func planCodexMCP(path string, root *os.Root, userDirectory string, change CodexResourceChange) (codexPlacementPlan, error) {
	var plan codexPlacementPlan
	before, err := readCodexProjectConfig(path, root)
	if err != nil {
		return plan, err
	}
	if before.Exists && before.Mode&0222 == 0 {
		return plan, errors.New("项目 Codex 配置为只读")
	}
	tables, err := codexMCPEntries(before.Content)
	if err != nil {
		return plan, err
	}
	block, owned, err := managedCodexMCP(path, change.Name, before.Content)
	if err != nil {
		return plan, err
	}
	plan.before, plan.after = before, before
	plan.preview.Target = codexProjectConfig
	var sourceVersion string
	if change.Action == "remove" {
		if !owned {
			return plan, errors.New("只撤回由 RepSync 接入的项目 MCP")
		}
		plan.preview.Before = block
		plan.after.Content = strings.Replace(before.Content, block, "", 1)
	} else {
		value, exists := tables[change.Name]
		switching := owned && codexMCPOverride(value) && (change.Action == "enable" || change.Action == "disable")
		if (exists || owned) && !switching {
			return plan, errors.New("项目已有同名 MCP，请先核对现有配置")
		}
		settings, version, err := codexMCPSettings(userDirectory, change)
		if err != nil {
			return plan, err
		}
		sourceVersion = version
		data, err := toml.Marshal(settings)
		if err != nil {
			return plan, err
		}
		prefix, end := codexMCPMarkers(path, change.Name)
		body := "[mcp_servers." + change.Name + "]\n" + string(data)
		next := prefix + digest(body) + "\n" + body + end + "\n"
		separator := ""
		if before.Content != "" && !strings.HasSuffix(before.Content, "\n") {
			separator = "\n"
		}
		if switching {
			plan.preview.Before = block
			plan.after.Content = strings.Replace(before.Content, block, next, 1)
		} else {
			plan.after.Content = before.Content + separator + next
		}
		plan.after.Exists = true
		plan.preview.After = next
	}
	entries, err := codexMCPEntries(plan.after.Content)
	if err != nil {
		return plan, err
	}
	if _, exists := entries[change.Name]; exists != (change.Action != "remove") {
		return plan, errors.New("无法确认指定的 MCP 表已更新")
	}
	if !reflect.DeepEqual(codexConfigWithoutServer(before.Content, change.Name), codexConfigWithoutServer(plan.after.Content, change.Name)) {
		return plan, errors.New("修改会影响其他 Codex 配置，请先核对区块附近的内容")
	}
	if len(plan.after.Content) > 256*1024 {
		return plan, errors.New("项目 Codex 配置超过 256 KiB")
	}
	if sameSnapshot(plan.before, plan.after) {
		return plan, errors.New("项目已使用此设置")
	}
	plan.preview.Version = gitDigest([]any{path, change, sourceVersion, plan.before, plan.after})
	return plan, nil
}

func codexMCPOverride(value any) bool {
	entry, ok := value.(map[string]any)
	_, enabled := entry["enabled"].(bool)
	return ok && len(entry) == 1 && enabled
}

// Both versions have already passed TOML validation.
func codexConfigWithoutServer(text, name string) map[string]any {
	config := map[string]any{}
	_ = toml.Unmarshal([]byte(text), &config)
	if servers, ok := config["mcp_servers"].(map[string]any); ok {
		delete(servers, name)
		if len(servers) == 0 {
			delete(config, "mcp_servers")
		}
	}
	return config
}

func codexMCPSettings(userDirectory string, change CodexResourceChange) (map[string]any, string, error) {
	if change.Action == "enable" || change.Action == "disable" {
		content, err := readText(filepath.Join(userDirectory, "config.toml"))
		if err != nil {
			return nil, "", errors.New("无法读取 Codex 用户配置")
		}
		entries, err := codexMCPEntries(content)
		if err != nil {
			return nil, "", errors.New("Codex 用户配置无法解析，请先修复 TOML")
		}
		server, _ := entries[change.Name].(map[string]any)
		if server["command"] == nil && server["url"] == nil {
			return nil, "", errors.New("用户配置中没有此 MCP 的连接定义")
		}
		return map[string]any{"enabled": change.Action == "enable"}, digest(content), nil
	}
	if change.URL != "" && change.Command != "" {
		return nil, "", errors.New("请选择 HTTP 或标准输入输出其中一种连接方式")
	}
	if change.URL != "" {
		endpoint, err := url.Parse(change.URL)
		if err != nil || len(change.URL) > 2048 || endpoint.Hostname() == "" ||
			(endpoint.Scheme != "https" && endpoint.Scheme != "http") ||
			endpoint.User != nil || endpoint.RawQuery != "" || endpoint.Fragment != "" || endpoint.ForceQuery {
			return nil, "", errors.New("请输入无账号和查询参数的 HTTP(S) 地址，令牌使用环境变量")
		}
		settings := map[string]any{"url": change.URL}
		if change.TokenEnv != "" {
			if !codexEnvName.MatchString(change.TokenEnv) {
				return nil, "", errors.New("请输入令牌的环境变量名称")
			}
			settings["bearer_token_env_var"] = change.TokenEnv
		}
		return settings, "", nil
	}
	if strings.TrimSpace(change.Command) == "" || len(change.Command) > 1024 || len(change.Args) > 64 || change.TokenEnv != "" {
		return nil, "", errors.New("请输入启动程序，参数最多 64 项")
	}
	for _, arg := range change.Args {
		if len(arg) > 2048 {
			return nil, "", errors.New("单个 MCP 参数超过长度限制")
		}
	}
	return map[string]any{"command": change.Command, "args": change.Args}, "", nil
}
