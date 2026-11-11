package workspace

import (
	"errors"
	"os"
	"slices"
)

type SharedResourceInfo struct {
	SharedResource
	Version string `json:"version"`
	Error   string `json:"error"`
}

type ResourceProject struct {
	Project       Project            `json:"project"`
	ConfigVersion string             `json:"configVersion"`
	Adoptions     []ResourceAdoption `json:"adoptions"`
	Error         string             `json:"error"`
}

type ResourceOverview struct {
	Version   string               `json:"version"`
	Resources []SharedResourceInfo `json:"resources"`
	Projects  []ResourceProject    `json:"projects"`
}

// SharedResources reads sources and adoption states without creating update plans.
func (s *Service) SharedResources(path string) (ResourceOverview, error) {
	s.syncMu.Lock()
	defer s.syncMu.Unlock()
	result := ResourceOverview{Resources: []SharedResourceInfo{}, Projects: []ResourceProject{}}
	projects, err := s.List()
	if err != nil {
		return result, err
	}
	if path != "" {
		project, err := s.registeredProject(path)
		if err != nil {
			return result, err
		}
		projects = []Project{project}
	}
	items, err := s.resourceCatalog()
	if err != nil {
		return result, err
	}
	result.Version = gitDigest(items)
	for _, item := range items {
		info := SharedResourceInfo{SharedResource: item}
		if source, err := readResourceSource(item); err != nil {
			info.Error = err.Error()
		} else {
			info.Version = source.Version
		}
		result.Resources = append(result.Resources, info)
	}
	for _, project := range projects {
		item := ResourceProject{Project: project, Adoptions: []ResourceAdoption{}}
		root, err := os.OpenRoot(project.Path)
		if err != nil {
			item.Error = err.Error()
		} else {
			file, err := loadProjectConfigFile(root)
			if err == nil {
				item.ConfigVersion = file.version
				var adoptions []ResourceAdoption
				adoptions, err = s.resourceStatus(project.Path)
				if err == nil {
					item.Adoptions = adoptions
				}
			}
			if err == nil {
				var current projectConfigFile
				current, err = loadProjectConfigFile(root)
				if err == nil && file.version != current.version {
					err = errors.New("项目配置在读取期间变化，请重新读取")
				}
			}
			if err != nil {
				item.Error = err.Error()
			}
			root.Close()
		}
		result.Projects = append(result.Projects, item)
	}
	for _, project := range result.Projects {
		for _, adoption := range project.Adoptions {
			if !slices.ContainsFunc(result.Resources, func(item SharedResourceInfo) bool { return item.ID == adoption.ID }) {
				result.Resources = append(result.Resources, SharedResourceInfo{
					SharedResource: SharedResource{ID: adoption.ID}, Error: "公共来源未登记",
				})
			}
		}
	}
	return result, nil
}

func (s *Service) checkResourceCatalog(version string) error {
	items, err := s.resourceCatalog()
	if err != nil {
		return err
	}
	if version == "" || gitDigest(items) != version {
		return errors.New("共享资源登记已变化，请重新读取后再操作")
	}
	return nil
}

// SaveResource edits only the catalog; consuming files require a separate preview.
func (s *Service) SaveResource(version, id, path, target string) (SharedResource, error) {
	s.syncMu.Lock()
	defer s.syncMu.Unlock()
	if err := s.checkResourceCatalog(version); err != nil {
		return SharedResource{}, err
	}
	return s.connectResource(id, path, target)
}

func (s *Service) ForgetResource(version, id string) error {
	s.syncMu.Lock()
	defer s.syncMu.Unlock()
	if err := s.checkResourceCatalog(version); err != nil {
		return err
	}
	return s.removeResource(id)
}

// SetResourceAdoption changes the declaration, retaining project files and history.
func (s *Service) SetResourceAdoption(path, configVersion, catalogVersion, id string, adopt bool) error {
	s.syncMu.Lock()
	defer s.syncMu.Unlock()
	if _, err := s.registeredProject(path); err != nil {
		return err
	}
	if err := s.checkResourceCatalog(catalogVersion); err != nil {
		return err
	}
	ids, err := projectResourceIDs(path)
	if err != nil {
		return err
	}
	if adopt {
		items, err := s.resourceCatalog()
		if err != nil {
			return err
		}
		index := slices.IndexFunc(items, func(item SharedResource) bool { return item.ID == id })
		if index < 0 {
			return errors.New("未登记此共享资源，请重新读取")
		}
		if _, err := readResourceSource(items[index]); err != nil {
			return err
		}
		if _, err := readUpdateTarget(path, items[index].Target); err != nil {
			return err
		}
		for _, item := range items {
			if item.ID != id && slices.Contains(ids, item.ID) && item.Target == items[index].Target {
				return errors.New("项目已采用写入同一目标的其他资源")
			}
		}
		if !slices.Contains(ids, id) {
			ids = append(ids, id)
		}
		if len(ids) > 64 {
			return errors.New("每项目最多采用 64 个共享资源")
		}
	} else {
		ids = slices.DeleteFunc(ids, func(value string) bool { return value == id })
	}
	return s.saveProjectSection(path, configVersion, "resources", false, ids)
}
