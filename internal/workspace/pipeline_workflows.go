package workspace

import (
	"fmt"
	"os"
	"slices"
	"strings"

	"go.yaml.in/yaml/v3"
)

type WorkflowStep struct {
	Name             string `json:"name"`
	Run              string `json:"run"`
	Uses             string `json:"uses"`
	If               string `json:"if"`
	WorkingDirectory string `json:"workingDirectory"`
}

type WorkflowJob struct {
	ID       string         `json:"id"`
	Name     string         `json:"name"`
	Needs    []string       `json:"needs"`
	If       string         `json:"if"`
	Uses     string         `json:"uses"`
	RunsOn   string         `json:"runsOn"`
	Strategy string         `json:"strategy"`
	Steps    []WorkflowStep `json:"steps"`
}

type WorkflowDefinition struct {
	Content  string        `json:"content"`
	Path     string        `json:"path"`
	Name     string        `json:"name"`
	Triggers string        `json:"triggers"`
	Jobs     []WorkflowJob `json:"jobs"`
}

type WorkflowOverview struct {
	Workflows []WorkflowDefinition `json:"workflows"`
	Warnings  []string             `json:"warnings"`
	Version   string               `json:"version"`
}

// PipelineWorkflows reads repository definitions independently of GitHub auth or
// run history. Expressions, matrices, reusable jobs, and actions stay declarations.
func (s *Service) PipelineWorkflows(path string) (WorkflowOverview, error) {
	result := WorkflowOverview{Workflows: []WorkflowDefinition{}, Warnings: []string{}}
	if _, err := s.registeredProject(path); err != nil {
		return result, err
	}
	root, err := os.OpenRoot(path)
	if err != nil {
		return result, err
	}
	defer root.Close()
	dir, err := root.Open(".github/workflows")
	if os.IsNotExist(err) {
		result.Version = digest("no workflows")
		return result, nil
	}
	if err != nil {
		return result, err
	}
	defer dir.Close()
	entries, err := dir.ReadDir(-1)
	if err != nil {
		return result, err
	}
	// ReadDir on an open directory is not sorted, so sort to keep versions stable.
	slices.SortFunc(entries, func(a, b os.DirEntry) int { return strings.Compare(a.Name(), b.Name()) })
	input := ""
	files := 0
	for _, entry := range entries {
		if entry.IsDir() || (!strings.HasSuffix(entry.Name(), ".yml") && !strings.HasSuffix(entry.Name(), ".yaml")) {
			continue
		}
		if files >= 64 {
			result.Warnings = append(result.Warnings, "Workflow discovery reached 64 files; results are partial.")
			break
		}
		files++
		name := ".github/workflows/" + entry.Name()
		body, err := readProjectConfig(root, name)
		if err != nil {
			result.Warnings = append(result.Warnings, name+": "+err.Error())
			continue
		}
		if len(input)+len(body) > 1024*1024 {
			result.Warnings = append(result.Warnings, "Workflow definitions exceed 1 MiB; results are partial.")
			break
		}
		input += name + "\x00" + string(body)
		workflow, err := parseWorkflow(name, body)
		if err != nil {
			result.Warnings = append(result.Warnings, name+": "+err.Error())
			continue
		}
		result.Workflows = append(result.Workflows, workflow)
	}
	result.Version = digest(input + strings.Join(result.Warnings, "\n"))
	return result, nil
}

func yamlDeclaration(node yaml.Node) string {
	if node.Kind == 0 {
		return ""
	}
	if node.Kind == yaml.ScalarNode {
		return node.Value
	}
	data, err := yaml.Marshal(&node)
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(data))
}

func parseWorkflow(path string, body []byte) (WorkflowDefinition, error) {
	var source struct {
		Name     string    `yaml:"name"`
		On       yaml.Node `yaml:"on"`
		Jobs     yaml.Node `yaml:"jobs"`
		Defaults struct {
			Run struct {
				WorkingDirectory string `yaml:"working-directory"`
			} `yaml:"run"`
		} `yaml:"defaults"`
	}
	if err := yaml.Unmarshal(body, &source); err != nil {
		return WorkflowDefinition{}, fmt.Errorf("invalid workflow YAML: %w", err)
	}
	if source.Jobs.Kind != yaml.MappingNode {
		return WorkflowDefinition{}, fmt.Errorf("workflow jobs must be a mapping")
	}
	result := WorkflowDefinition{Content: string(body), Path: path, Name: source.Name, Triggers: yamlDeclaration(source.On), Jobs: []WorkflowJob{}}
	if result.Name == "" {
		result.Name = path
	}
	if len(source.Jobs.Content) > 128 {
		return result, fmt.Errorf("workflow exceeds 64 jobs")
	}
	seen := map[string]bool{}
	for i := 0; i < len(source.Jobs.Content); i += 2 {
		id := source.Jobs.Content[i].Value
		if seen[id] {
			return result, fmt.Errorf("duplicate workflow job %s", id)
		}
		seen[id] = true
		var job struct {
			Name     string    `yaml:"name"`
			Needs    yaml.Node `yaml:"needs"`
			If       string    `yaml:"if"`
			Uses     string    `yaml:"uses"`
			RunsOn   yaml.Node `yaml:"runs-on"`
			Strategy yaml.Node `yaml:"strategy"`
			Defaults struct {
				Run struct {
					WorkingDirectory string `yaml:"working-directory"`
				} `yaml:"run"`
			} `yaml:"defaults"`
			Steps []struct {
				Name             string `yaml:"name"`
				Run              string `yaml:"run"`
				Uses             string `yaml:"uses"`
				If               string `yaml:"if"`
				WorkingDirectory string `yaml:"working-directory"`
			} `yaml:"steps"`
		}
		if err := source.Jobs.Content[i+1].Decode(&job); err != nil {
			return result, fmt.Errorf("invalid job: %w", err)
		}
		if len(job.Steps) > 256 {
			return result, fmt.Errorf("job exceeds 256 steps")
		}
		item := WorkflowJob{ID: source.Jobs.Content[i].Value, Name: job.Name, If: job.If, Uses: job.Uses, RunsOn: yamlDeclaration(job.RunsOn), Strategy: yamlDeclaration(job.Strategy), Needs: []string{}, Steps: []WorkflowStep{}}
		switch job.Needs.Kind {
		case yaml.ScalarNode:
			item.Needs = append(item.Needs, job.Needs.Value)
		case yaml.SequenceNode:
			for _, node := range job.Needs.Content {
				if node.Kind != yaml.ScalarNode {
					return result, fmt.Errorf("job needs must be strings")
				}
				item.Needs = append(item.Needs, node.Value)
			}
		case 0:
		default:
			return result, fmt.Errorf("job needs must be a string or sequence")
		}
		for _, step := range job.Steps {
			cwd := step.WorkingDirectory
			if cwd == "" {
				cwd = job.Defaults.Run.WorkingDirectory
			}
			if cwd == "" {
				cwd = source.Defaults.Run.WorkingDirectory
			}
			item.Steps = append(item.Steps, WorkflowStep{Name: step.Name, Run: step.Run, Uses: step.Uses, If: step.If, WorkingDirectory: cwd})
		}
		result.Jobs = append(result.Jobs, item)
	}
	return result, nil
}
