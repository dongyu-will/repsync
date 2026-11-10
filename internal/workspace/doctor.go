package workspace

import (
	"context"
	"fmt"
	"maps"
	"slices"
)

type DiagnosisIssue struct {
	Level     string `json:"level"`
	Kind      string `json:"kind"`
	ServiceID string `json:"serviceId,omitempty"`
	Message   string `json:"message"`
}

type ServiceCheck struct {
	ID                  string   `json:"id"`
	Name                string   `json:"name"`
	Command             []string `json:"command"`
	Cwd                 string   `json:"cwd"`
	Port                int      `json:"port"`
	AutoPort            bool     `json:"autoPort"`
	Environment         []string `json:"environment"`
	EndpointEnvironment []string `json:"endpointEnvironment"`
	ExecutableAvailable bool     `json:"executableAvailable"`
}

type ProjectDiagnosis struct {
	Project       Project            `json:"project"`
	Ready         bool               `json:"ready"`
	ConfigVersion string             `json:"configVersion"`
	Services      []ServiceCheck     `json:"services"`
	Pipelines     []Pipeline         `json:"pipelines"`
	Resources     []ResourceAdoption `json:"resources"`
	Issues        []DiagnosisIssue   `json:"issues"`
}

// DiagnoseProject inspects configuration and executable availability without executing project commands.
func (s *Service) DiagnoseProject(path string) (ProjectDiagnosis, error) {
	project, err := s.registeredProject(path)
	if err != nil {
		return ProjectDiagnosis{}, err
	}
	report := ProjectDiagnosis{Project: project, Ready: true, Services: []ServiceCheck{}, Pipelines: []Pipeline{}, Resources: []ResourceAdoption{}, Issues: []DiagnosisIssue{}}
	issue := func(level, kind, id, message string) {
		report.Issues = append(report.Issues, DiagnosisIssue{Level: level, Kind: kind, ServiceID: id, Message: message})
		if level == "error" {
			report.Ready = false
		}
	}
	config, err := s.ProjectConfiguration(path)
	if err != nil {
		issue("error", "configuration", "", err.Error())
	} else {
		report.ConfigVersion = config.Version
	}
	services, err := s.localServiceConfig(path)
	if err != nil {
		issue("error", "services", "", err.Error())
	} else {
		for _, d := range services.services {
			check := ServiceCheck{ID: d.ID, Name: d.Name, Command: d.Command, Cwd: d.Cwd, Port: d.Port, AutoPort: d.AutoPort, Environment: []string{}, EndpointEnvironment: []string{}}
			for name := range d.Env {
				check.Environment = append(check.Environment, name)
			}
			for name := range d.EnvFrom {
				check.EndpointEnvironment = append(check.EndpointEnvironment, name)
			}
			slices.Sort(check.Environment)
			slices.Sort(check.EndpointEnvironment)
			_, err := projectCommand(context.Background(), path, d.Cwd, d.Env, d.Command, false)
			check.ExecutableAvailable = err == nil
			if err != nil {
				issue("error", "executable", d.ID, err.Error())
			}
			if d.Port == 0 && !d.AutoPort {
				issue("warning", "port_unchecked", d.ID, "未声明监听端口；若为网络服务，请由 AI 补齐端口输入")
			}
			report.Services = append(report.Services, check)
		}
		if len(services.services) > 0 {
			plans, err := s.servicePlans([]string{path}, nil)
			if err != nil {
				issue("error", "dependencies", "", err.Error())
			} else {
				live := make([]ServiceStartResult, len(plans))
				s.taskMu.Lock()
				for i, plan := range plans {
					if active := s.activeLocalServices[plan.path][plan.configured.ID]; active != nil {
						active.mu.Lock()
						live[i] = ServiceStartResult{State: active.state}
						active.mu.Unlock()
					}
				}
				s.taskMu.Unlock()
				for i, plan := range plans {
					if len(plan.dependencies) > 0 && localServiceRunning(live[i].State.Status) {
						unavailable := false
						for _, index := range plan.dependencies {
							if !localServiceRunning(live[index].State.Status) {
								unavailable = true
							}
						}
						if unavailable {
							issue("error", "dependency_not_running", plan.configured.ID, "引用的服务尚未运行或已停止")
						} else {
							effective, resolveErr := resolveServiceLinks(resolveServicePort(plan.configured, live[i].State.Definition.Port), plan, live)
							if resolveErr != nil {
								issue("error", "dependency_endpoint", plan.configured.ID, resolveErr.Error())
							} else if !maps.Equal(effective.Env, live[i].State.Definition.Env) {
								issue("error", "dependency_address_changed", plan.configured.ID, "依赖地址已变化，请由 AI 重启此服务以重新注入地址")
							}
						}
					}
					for _, index := range plan.dependencies {
						source := plans[index].configured
						if source.Port == 0 && !source.AutoPort {
							issue("error", "dependency_endpoint", plan.configured.ID, "引用的服务未声明监听端口")
						}
					}
					for name, index := range plan.dependencies {
						endpoint := plan.configured.EnvFrom[name]
						if endpoint.Field != "port" && plans[index].configured.URL == "" {
							issue("error", "dependency_endpoint", plan.configured.ID, "引用的服务未声明页面或 API URL")
						}
					}
				}
			}
		}
	}
	pipelines, err := s.Pipelines(path)
	if err != nil {
		issue("error", "pipelines", "", err.Error())
	} else {
		report.Pipelines = pipelines.Pipelines
		for _, pipeline := range pipelines.Pipelines {
			for _, step := range pipeline.Steps {
				if _, err := projectCommand(context.Background(), path, pipeline.Cwd, pipeline.Env, step, true); err != nil {
					issue("error", "pipeline_executable", pipeline.ID, err.Error())
				}
			}
		}
	}
	overview, err := s.LocalServices(path)
	if err == nil {
		for _, state := range overview.Services {
			if !localServiceRunning(state.Status) && state.PortState == "occupied" && !state.Definition.AutoPort {
				issue("error", "port_occupied", state.Definition.ID, fmt.Sprintf("固定端口 %d 已占用", state.Definition.Port))
			}
			if state.ConfigChanged {
				issue("warning", "restart_required", state.Definition.ID, "配置已变化，运行实例尚未更新")
			}
		}
	}
	resources, err := s.ResourceStatus(path)
	if err != nil {
		issue("error", "resources", "", err.Error())
	} else {
		report.Resources = resources
		for _, resource := range resources {
			switch resource.Status {
			case "error":
				issue("error", "resource", resource.ID, resource.Error)
			case "modified":
				issue("warning", "resource_modified", resource.ID, "共享文件有项目差异，更新将被阻止")
			case "missing", "outdated":
				issue("warning", "resource_update", resource.ID, "共享文件尚未同步到当前来源版本")
			}
		}
	}
	if len(report.Services) == 0 && len(report.Pipelines) == 0 {
		issue("error", "commands_missing", "", "尚未接入启动或检查入口，请由 AI 读取项目指南补齐")
	}
	return report, nil
}
