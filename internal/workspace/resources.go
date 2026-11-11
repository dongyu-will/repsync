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

type SharedResource struct {
	ID     string `json:"id"`
	Path   string `json:"path"`
	Target string `json:"target"`
}

type ResourceSource struct {
	SharedResource
	Version string `json:"version"`
	Content string `json:"content"`
	Mode    uint32 `json:"mode"`
}

type ResourceAdoption struct {
	SharedResource
	Status         string `json:"status"`
	Version        string `json:"version"`
	AdoptedVersion string `json:"adoptedVersion"`
	Error          string `json:"error,omitempty"`
}

func updateTarget(record updateRecord) string {
	if record.Target != "" {
		return record.Target
	}
	return "AGENTS.md"
}

func validateUpdatePath(root *os.Root, target string) error {
	if !filepath.IsLocal(target) || target == "." || filepath.Clean(target) != target {
		return errors.New("destination must be a project-relative file path")
	}
	parent := filepath.Dir(target)
	for parent != "." {
		info, err := root.Lstat(parent)
		if err != nil {
			return fmt.Errorf("destination parent folder unavailable; create or repair %s first: %w", parent, err)
		}
		if !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
			return errors.New("destination parent must not be a symbolic link or regular file")
		}
		parent = filepath.Dir(parent)
	}
	return nil
}

func resourceTarget(target string) error {
	if !filepath.IsLocal(target) || filepath.Clean(target) != target || target == "." {
		return errors.New("resource destination must be a project-relative file path")
	}
	for _, part := range strings.Split(filepath.ToSlash(target), "/") {
		if strings.EqualFold(part, ".git") {
			return errors.New("resources cannot write to .git")
		}
	}
	if strings.EqualFold(target, "AGENTS.md") || strings.EqualFold(target, ".repsync.json") {
		return errors.New("AGENTS.md uses block sync; .repsync.json is managed by project configuration")
	}
	return nil
}

func (s *Service) resourceCatalog() ([]SharedResource, error) {
	items := []SharedResource{}
	data, err := os.ReadFile(filepath.Join(s.syncDir(), "resources.json"))
	if errors.Is(err, os.ErrNotExist) {
		return items, nil
	}
	if err != nil {
		return nil, err
	}
	if len(data) > 256*1024 {
		return nil, errors.New("shared resource registration exceeds 256 KiB")
	}
	if err := json.Unmarshal(data, &items); err != nil {
		return nil, err
	}
	if len(items) > 64 {
		return nil, errors.New("shared resource registration exceeds 64 entries")
	}
	seen := map[string]bool{}
	for _, item := range items {
		if item.ID == "" || seen[item.ID] || !filepath.IsAbs(item.Path) {
			return nil, errors.New("invalid shared resource registration or duplicate id")
		}
		if err := resourceTarget(item.Target); err != nil {
			return nil, err
		}
		seen[item.ID] = true
	}
	return items, nil
}

func (s *Service) Resources() ([]SharedResource, error) {
	s.syncMu.Lock()
	defer s.syncMu.Unlock()
	return s.resourceCatalog()
}

// ConnectResource registers one text file; it never writes to a consuming project.
func (s *Service) ConnectResource(id, path, target string) (SharedResource, error) {
	s.syncMu.Lock()
	defer s.syncMu.Unlock()
	return s.connectResource(id, path, target)
}

func (s *Service) connectResource(id, path, target string) (SharedResource, error) {
	if id == "" || len(id) > 80 || strings.ContainsAny(id, "/\\\x00") {
		return SharedResource{}, errors.New("resource requires a valid id")
	}
	if err := resourceTarget(target); err != nil {
		return SharedResource{}, err
	}
	if !filepath.IsAbs(path) {
		return SharedResource{}, errors.New("resource source requires an absolute path")
	}
	canonical, err := filepath.EvalSymlinks(path)
	if err != nil {
		return SharedResource{}, err
	}
	item := SharedResource{ID: id, Path: canonical, Target: target}
	if _, err := readResourceSource(item); err != nil {
		return SharedResource{}, err
	}
	items, err := s.resourceCatalog()
	if err != nil {
		return SharedResource{}, err
	}
	index := slices.IndexFunc(items, func(item SharedResource) bool { return item.ID == id })
	if index < 0 {
		items = append(items, item)
	} else {
		items[index] = item
	}
	if len(items) > 64 {
		return SharedResource{}, errors.New("up to 64 shared file resources can be registered")
	}
	if err := saveJSON(filepath.Join(s.syncDir(), "resources.json"), items); err != nil {
		return SharedResource{}, err
	}
	for token, plan := range s.previews {
		if plan.Record.ResourceID == id {
			delete(s.previews, token)
		}
	}
	return item, nil
}

func readResourceSource(item SharedResource) (ResourceSource, error) {
	if err := resourceTarget(item.Target); err != nil {
		return ResourceSource{}, err
	}
	info, err := os.Stat(item.Path)
	if err != nil {
		return ResourceSource{}, err
	}
	if !info.Mode().IsRegular() {
		return ResourceSource{}, errors.New("shared resource source must be a regular text file")
	}
	content, err := readText(item.Path)
	if err != nil {
		return ResourceSource{}, err
	}
	if !utf8.ValidString(content) {
		return ResourceSource{}, errors.New("shared resources must be UTF-8 text")
	}
	return ResourceSource{SharedResource: item, Version: digest(content), Content: content, Mode: uint32(info.Mode().Perm())}, nil
}

func projectResourceIDs(path string) ([]string, error) {
	root, err := os.OpenRoot(path)
	if err != nil {
		return nil, err
	}
	defer root.Close()
	file, err := loadProjectConfigFile(root)
	if err != nil {
		return nil, err
	}
	ids := []string{}
	if data := file.fields["resources"]; len(data) > 0 {
		if err := json.Unmarshal(data, &ids); err != nil {
			return nil, errors.New("resources must be an array of resource ids")
		}
	}
	if len(ids) > 64 {
		return nil, errors.New("each project can adopt up to 64 shared resources")
	}
	seen := map[string]bool{}
	for _, id := range ids {
		if id == "" || seen[id] {
			return nil, errors.New("resources contains empty or duplicate ids")
		}
		seen[id] = true
	}
	return ids, nil
}

func (s *Service) projectResource(path, id string) (ResourceSource, error) {
	ids, err := projectResourceIDs(path)
	if err != nil {
		return ResourceSource{}, err
	}
	if !slices.Contains(ids, id) {
		return ResourceSource{}, errors.New("project has not adopted this resource; ask AI to register its id in .repsync.json resources")
	}
	items, err := s.resourceCatalog()
	if err != nil {
		return ResourceSource{}, err
	}
	index := slices.IndexFunc(items, func(item SharedResource) bool { return item.ID == id })
	if index < 0 {
		return ResourceSource{}, fmt.Errorf("shared resource %s is not registered", id)
	}
	return readResourceSource(items[index])
}

// resourceAdoption preserves any target that does not match the last applied managed snapshot.
func (s *Service) resourceAdoption(path, id string) (ResourceAdoption, ResourceSource, fileSnapshot, error) {
	item := ResourceAdoption{SharedResource: SharedResource{ID: id}, Status: "error"}
	source, err := s.projectResource(path, id)
	if err != nil {
		return item, source, fileSnapshot{}, err
	}
	item.SharedResource, item.Version = source.SharedResource, source.Version
	target, err := readUpdateTarget(path, source.Target)
	if err != nil {
		return item, source, target, err
	}
	if source.Path == filepath.Join(path, source.Target) {
		return item, source, target, errors.New("resource source and project destination cannot be the same file")
	}
	if !target.Exists {
		item.Status = "missing"
		return item, source, target, nil
	}
	if target.Content == source.Content {
		item.Status, item.AdoptedVersion = "current", source.Version
		return item, source, target, nil
	}
	item.Status = "modified"
	entries, err := os.ReadDir(filepath.Join(s.syncDir(), "history"))
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return item, source, target, err
	}
	var latest updateRecord
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".json") {
			continue
		}
		record, err := s.readRecord(strings.TrimSuffix(entry.Name(), ".json"))
		if err != nil {
			return item, source, target, err
		}
		if record.Project.Path == path && record.ResourceID == id && record.Target == source.Target && record.CreatedAt > latest.CreatedAt {
			latest = record
		}
	}
	if latest.ID != "" && (sameSnapshot(target, latest.After) || (latest.State == "restored" && sameSnapshot(target, latest.Before))) {
		item.Status, item.AdoptedVersion = "outdated", digest(target.Content)
	}
	return item, source, target, nil
}

func (s *Service) validateResourceAdoption(path string) error {
	ids, err := projectResourceIDs(path)
	if err != nil {
		return err
	}
	items, err := s.resourceCatalog()
	if err != nil {
		return err
	}
	targets := map[string]bool{}
	for _, id := range ids {
		index := slices.IndexFunc(items, func(item SharedResource) bool { return item.ID == id })
		if index < 0 {
			return fmt.Errorf("shared resource %s is not registered", id)
		}
		if targets[items[index].Target] {
			return errors.New("multiple resources cannot write to the same destination")
		}
		targets[items[index].Target] = true
	}
	return nil
}

func (s *Service) ResourceStatus(path string) ([]ResourceAdoption, error) {
	s.syncMu.Lock()
	defer s.syncMu.Unlock()
	return s.resourceStatus(path)
}

func (s *Service) resourceStatus(path string) ([]ResourceAdoption, error) {
	if _, err := s.registeredProject(path); err != nil {
		return nil, err
	}
	ids, err := projectResourceIDs(path)
	if err != nil {
		return nil, err
	}
	result := []ResourceAdoption{}
	targets := map[string]bool{}
	for _, id := range ids {
		item, _, _, err := s.resourceAdoption(path, id)
		if err != nil {
			item.Status, item.Error = "error", err.Error()
		}
		if item.Target != "" && targets[item.Target] {
			item.Status, item.Error = "error", "multiple resources cannot write to the same destination"
		}
		targets[item.Target] = true
		result = append(result, item)
	}
	return result, nil
}

func (s *Service) PreviewResource(path, id string) (UpdatePreview, error) {
	s.syncMu.Lock()
	defer s.syncMu.Unlock()
	project, err := s.registeredProject(path)
	if err != nil {
		return UpdatePreview{}, err
	}
	if err := s.validateResourceAdoption(path); err != nil {
		return UpdatePreview{}, err
	}
	item, source, before, err := s.resourceAdoption(path, id)
	if err != nil {
		return UpdatePreview{}, err
	}
	mode := before.Mode
	if !before.Exists {
		mode = source.Mode
	}
	after := fileSnapshot{Exists: true, Content: source.Content, Mode: mode}
	diff, err := documentDiff(before.Content, after.Content)
	if err != nil {
		return UpdatePreview{}, err
	}
	diff = strings.ReplaceAll(diff, "/AGENTS.md", "/"+source.Target)
	now := time.Now()
	for token, plan := range s.previews {
		if now.After(plan.Expires) {
			delete(s.previews, token)
		}
	}
	if len(s.previews) >= 32 {
		return UpdatePreview{}, errors.New("too many pending previews; try again later")
	}
	if s.previews == nil {
		s.previews = map[string]pendingUpdate{}
	}
	conflict := item.Status == "modified"
	token := rand.Text()
	record := updateRecord{ResourceID: id, ResourceMode: source.Mode, Conflict: conflict, UpdateHistory: UpdateHistory{ID: token, Project: project, Target: source.Target, SourcePath: source.Path, Version: source.Version, CreatedAt: now.UTC().Format("2006-01-02T15:04:05.000000000Z"), State: "prepared"}, Before: before, After: after}
	s.previews[token] = pendingUpdate{Record: record, Expires: now.Add(15 * time.Minute)}
	return UpdatePreview{Token: token, Project: project, Target: source.Target, SourcePath: source.Path, Version: source.Version, Diff: diff, Changed: !sameSnapshot(before, after), Conflict: conflict}, nil
}

func (s *Service) RemoveResource(id string) error {
	s.syncMu.Lock()
	defer s.syncMu.Unlock()
	return s.removeResource(id)
}

func (s *Service) removeResource(id string) error {
	items, err := s.resourceCatalog()
	if err != nil {
		return err
	}
	index := slices.IndexFunc(items, func(item SharedResource) bool { return item.ID == id })
	if index < 0 {
		return errors.New("resource not registered")
	}
	items = slices.Delete(items, index, index+1)
	if err := saveJSON(filepath.Join(s.syncDir(), "resources.json"), items); err != nil {
		return err
	}
	for token, plan := range s.previews {
		if plan.Record.ResourceID == id {
			delete(s.previews, token)
		}
	}
	return nil
}
