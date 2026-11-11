package workspace

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
)

// ProjectConfig contains editable definitions, with omitted sections discovered independently.
type ProjectConfig struct {
	Version            string                   `json:"version"`
	ServicesAutomatic  bool                     `json:"servicesAutomatic"`
	PipelinesAutomatic bool                     `json:"pipelinesAutomatic"`
	Services           []LocalServiceDefinition `json:"services"`
	Pipelines          []Pipeline               `json:"pipelines"`
	Warnings           []string                 `json:"warnings"`
}

type projectConfigFile struct {
	fields  map[string]json.RawMessage
	version string
	mode    os.FileMode
}

func loadProjectConfigFile(root *os.Root) (projectConfigFile, error) {
	file := projectConfigFile{fields: map[string]json.RawMessage{}, mode: 0600}
	info, err := root.Lstat(".repsync.json")
	if errors.Is(err, os.ErrNotExist) {
		file.version = fmt.Sprintf("%x", sha256.Sum256([]byte("missing")))
		return file, nil
	}
	if err != nil {
		return file, err
	}
	if !info.Mode().IsRegular() {
		return file, errors.New(".repsync.json 必须是普通文件，不能是符号链接")
	}
	data, err := readProjectConfig(root, ".repsync.json")
	if err != nil {
		return file, err
	}
	if err := json.Unmarshal(data, &file.fields); err != nil || file.fields == nil {
		return file, errors.New(".repsync.json 需要是有效的 JSON 对象")
	}
	file.mode = info.Mode().Perm()
	file.version = fmt.Sprintf("%x", sha256.Sum256(append([]byte(fmt.Sprint(file.mode)+":"), data...)))
	return file, nil
}

// ProjectConfiguration reads effective commands without executing them.
func (s *Service) ProjectConfiguration(path string) (ProjectConfig, error) {
	if _, err := s.registeredProject(path); err != nil {
		return ProjectConfig{}, err
	}
	root, err := os.OpenRoot(path)
	if err != nil {
		return ProjectConfig{}, err
	}
	defer root.Close()
	file, err := loadProjectConfigFile(root)
	if err != nil {
		return ProjectConfig{}, err
	}
	config := ProjectConfig{
		Version:            file.version,
		ServicesAutomatic:  len(file.fields["services"]) == 0 || string(file.fields["services"]) == "null",
		PipelinesAutomatic: len(file.fields["pipelines"]) == 0 || string(file.fields["pipelines"]) == "null",
		Services:           []LocalServiceDefinition{}, Pipelines: []Pipeline{}, Warnings: []string{},
	}
	// Editing must remain possible when an existing working directory disappeared.
	if config.ServicesAutomatic {
		services, err := s.localServiceConfig(path)
		if err != nil {
			config.Warnings = append(config.Warnings, "服务自动识别失败："+err.Error())
		} else {
			config.Services = services.services
		}
	} else if err := json.Unmarshal(file.fields["services"], &config.Services); err != nil {
		return ProjectConfig{}, fmt.Errorf("services 格式无效: %w", err)
	}
	if config.PipelinesAutomatic {
		pipelines, err := s.Pipelines(path)
		if err != nil {
			config.Warnings = append(config.Warnings, "流水线自动识别失败："+err.Error())
		} else {
			config.Pipelines = pipelines.Pipelines
		}
	} else if err := json.Unmarshal(file.fields["pipelines"], &config.Pipelines); err != nil {
		return ProjectConfig{}, fmt.Errorf("pipelines 格式无效: %w", err)
	}
	for i := range config.Pipelines {
		if config.Pipelines[i].TimeoutSeconds == 0 {
			config.Pipelines[i].TimeoutSeconds = 1800
		}
	}
	return config, nil

}

func (s *Service) SaveServices(path, version string, automatic bool, definitions []LocalServiceDefinition) error {
	if !automatic {
		if definitions == nil {
			definitions = []LocalServiceDefinition{}
		}
		if err := validateLocalServices(definitions); err != nil {
			return err
		}
		for _, d := range definitions {
			if _, err := commandDirectory(path, d.Cwd); err != nil {
				return err
			}
		}
	}
	return s.saveProjectSection(path, version, "services", automatic, definitions)
}

func (s *Service) SavePipelines(path, version string, automatic bool, definitions []Pipeline) error {
	if !automatic {
		if definitions == nil {
			definitions = []Pipeline{}
		}
		if err := validatePipelines(definitions); err != nil {
			return err
		}
		for _, d := range definitions {
			if _, err := commandDirectory(path, d.Cwd); err != nil {
				return err
			}
		}
	}
	return s.saveProjectSection(path, version, "pipelines", automatic, definitions)
}

// Only the selected section changes. A stale editor must reload before saving.
func (s *Service) saveProjectSection(path, version, section string, automatic bool, definitions any) error {
	if _, err := s.registeredProject(path); err != nil {
		return err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	root, err := os.OpenRoot(path)
	if err != nil {
		return err
	}
	defer root.Close()
	file, err := loadProjectConfigFile(root)
	if err != nil {
		return err
	}
	if file.version != version {
		return errors.New("配置已被其他操作修改，请重新载入后再保存")
	}
	if automatic {
		delete(file.fields, section)
	} else {
		data, err := json.Marshal(definitions)
		if err != nil {
			return err
		}
		if section == "resources" {
			file.fields[section] = data
		} else if section == "reviewProfile" {
			var previous, next map[string]json.RawMessage
			_ = json.Unmarshal(file.fields[section], &previous)
			if err := json.Unmarshal(data, &next); err != nil {
				return err
			}
			for key, value := range previous {
				if _, ok := next[key]; !ok {
					next[key] = value
				}
			}
			file.fields[section], err = json.Marshal(next)
			if err != nil {
				return err
			}
		} else {
			// Retain extension fields on existing entries with the same ID.
			var previous, next []map[string]json.RawMessage
			_ = json.Unmarshal(file.fields[section], &previous)
			if err := json.Unmarshal(data, &next); err != nil {
				return err
			}
			for _, entry := range next {
				for _, old := range previous {
					if string(entry["id"]) == string(old["id"]) {
						for key, value := range old {
							if _, ok := entry[key]; !ok {
								entry[key] = value
							}
						}
						break
					}
				}
			}
			file.fields[section], err = json.Marshal(next)
			if err != nil {
				return err
			}
		}
	}
	data, err := json.MarshalIndent(file.fields, "", "  ")
	if err != nil {
		return err
	}
	data = append(data, '\n')
	if len(data) > 256*1024 {
		return errors.New("配置超过 256 KiB")
	}
	name := ".repsync-config-" + rand.Text()
	temp, err := root.OpenFile(name, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
	if err != nil {
		return err
	}
	defer root.Remove(name)
	if _, err = temp.Write(data); err == nil {
		err = temp.Chmod(file.mode)
	}
	if err == nil {
		err = temp.Sync()
	}
	closeErr := temp.Close()
	if err != nil {
		return err
	}
	if closeErr != nil {
		return closeErr
	}
	current, err := loadProjectConfigFile(root)
	if err != nil {
		return err
	}
	if current.version != version {
		return errors.New("配置在保存期间发生变化，请重新载入")
	}
	return root.Rename(name, ".repsync.json")
}

var environmentName = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]*$`)

func validateCommandOptions(cwd string, env map[string]string) error {
	if strings.ContainsRune(cwd, 0) || filepath.IsAbs(cwd) || !filepath.IsLocal(filepath.Clean(cwd)) {
		return errors.New("工作目录需要是项目内的相对目录，留空使用项目根目录")
	}
	for name, value := range env {
		if !environmentName.MatchString(name) || strings.ContainsRune(value, 0) {
			return errors.New("环境变量需要有效名称，值不能包含空字符")
		}
		switch name {
		case "PWD", "CI", "NO_COLOR", "GIT_DIR", "GIT_WORK_TREE", "GIT_INDEX_FILE", "GIT_COMMON_DIR", "GIT_OBJECT_DIRECTORY", "GIT_ALTERNATE_OBJECT_DIRECTORIES":
			return fmt.Errorf("环境变量 %s 由运行器管理", name)
		}
	}
	return nil
}

func commandDirectory(path, cwd string) (string, error) {
	if err := validateCommandOptions(cwd, nil); err != nil {
		return "", err
	}
	base, err := filepath.EvalSymlinks(path)
	if err != nil {
		return "", err
	}
	directory, err := filepath.EvalSymlinks(filepath.Join(base, cwd))
	if err != nil {
		return "", fmt.Errorf("工作目录不可访问: %w", err)
	}
	relative, err := filepath.Rel(base, directory)
	if err != nil || !filepath.IsLocal(relative) {
		return "", errors.New("工作目录不能指向项目目录之外")
	}
	info, err := os.Stat(directory)
	if err != nil {
		return "", err
	}
	if !info.IsDir() {
		return "", errors.New("工作目录必须是目录")
	}
	return directory, nil
}

func projectCommand(ctx context.Context, path, cwd string, env map[string]string, args []string, ci bool) (*exec.Cmd, error) {
	directory, err := commandDirectory(path, cwd)
	if err != nil {
		return nil, err
	}
	if err := validateCommandOptions(cwd, env); err != nil {
		return nil, err
	}
	environment := commandEnvironment(ci)
	values := make(map[string]string, len(env)+1)
	for name, value := range env {
		values[name] = value
	}
	values["PWD"] = directory
	for name, value := range values {
		prefix := name + "="
		filtered := environment[:0]
		for _, entry := range environment {
			if !strings.HasPrefix(entry, prefix) {
				filtered = append(filtered, entry)
			}
		}
		environment = append(filtered, prefix+value)
	}
	binary := args[0]
	if !strings.ContainsRune(binary, os.PathSeparator) {
		searchPath := ""
		for _, entry := range environment {
			if strings.HasPrefix(entry, "PATH=") {
				searchPath = strings.TrimPrefix(entry, "PATH=")
			}
		}
		binary = ""
		for _, entry := range filepath.SplitList(searchPath) {
			candidate := filepath.Join(entry, args[0])
			if !filepath.IsAbs(candidate) {
				candidate = filepath.Join(directory, candidate)
			}
			if info, err := os.Stat(candidate); err == nil && info.Mode().IsRegular() && info.Mode().Perm()&0111 != 0 {
				binary = candidate
				break
			}
		}
		if binary == "" {
			return nil, fmt.Errorf("找不到可执行文件 %s，请检查 PATH 或填写绝对路径", args[0])
		}
	}
	if strings.ContainsRune(binary, os.PathSeparator) {
		if !filepath.IsAbs(binary) {
			binary = filepath.Join(directory, binary)
		}
		info, err := os.Stat(binary)
		if err != nil {
			return nil, fmt.Errorf("可执行文件不可访问: %w", err)
		}
		if !info.Mode().IsRegular() || info.Mode().Perm()&0111 == 0 {
			return nil, fmt.Errorf("%s 不是可执行文件", binary)
		}
	}
	cmd := exec.CommandContext(ctx, binary, args[1:]...)
	cmd.Dir, cmd.Env = directory, environment
	return cmd, nil
}
