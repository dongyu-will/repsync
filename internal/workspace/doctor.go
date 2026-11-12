package workspace

import (
	"context"
	"fmt"
	"maps"
	"os"
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
	ObserveOnly         bool     `json:"observeOnly"`
	OneShot             bool     `json:"oneShot,omitempty"`
	DependsOn           []string `json:"dependsOn,omitempty"`
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
	AIResources   []AIResource       `json:"aiResources"`
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
			check := ServiceCheck{ID: d.ID, Name: d.Name, Command: d.Command, Cwd: d.Cwd, Port: d.Port, AutoPort: d.AutoPort, OneShot: d.OneShot, DependsOn: slices.Clone(d.DependsOn), Environment: []string{}, EndpointEnvironment: []string{}}
			for name := range d.Env {
				check.Environment = append(check.Environment, name)
			}
			for name := range d.EnvFrom {
				check.EndpointEnvironment = append(check.EndpointEnvironment, name)
			}
			slices.Sort(check.Environment)
			slices.Sort(check.EndpointEnvironment)
			check.ObserveOnly = d.ObserveOnly
			if d.ObserveOnly {
				if !servicePortConnects(d) {
					prepared := slices.ContainsFunc(services.services, func(setup LocalServiceDefinition) bool {
						return setup.OneShot && slices.Contains(d.DependsOn, setup.ID)
					})
					if prepared {
						issue("warning", "dependency_pending_setup", d.ID, "Dependency is unreachable; project startup will run its declared preparation before checking connectivity")
					} else {
						issue("error", "dependency_unreachable", d.ID, "External dependency is unreachable; start it using its existing owner")
					}
				}
			} else {
				_, err := projectCommand(context.Background(), path, d.Cwd, d.Env, d.Command, false)
				check.ExecutableAvailable = err == nil
				if err != nil {
					issue("error", "executable", d.ID, err.Error())
				}
			}
			if d.Port == 0 && !d.AutoPort && !d.OneShot {
				issue("warning", "port_unchecked", d.ID, "No listening port declared; ask AI to configure it for network services")
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
					if plan.configured.ObserveOnly {
						live[i] = ServiceStartResult{State: observedServiceState(plan.configured)}
					}
				}
				for i, plan := range plans {
					if len(plan.dependencies) > 0 && localServiceRunning(live[i].State.Status) {
						unavailable := false
						for _, index := range plan.dependencies {
							if !localServiceRunning(live[index].State.Status) && live[index].State.Status != "reachable" {
								unavailable = true
							}
						}
						if unavailable {
							issue("error", "dependency_not_running", plan.configured.ID, "Referenced service is not running or has stopped")
						} else {
							effective, resolveErr := resolveServiceLinks(resolveServicePort(plan.configured, live[i].State.Definition.Port), plan, live)
							if resolveErr != nil {
								issue("error", "dependency_endpoint", plan.configured.ID, resolveErr.Error())
							} else if !maps.Equal(effective.Env, live[i].State.Definition.Env) {
								issue("error", "dependency_address_changed", plan.configured.ID, "Dependency endpoint changed; ask AI to restart this service to inject the new address")
							}
						}
					}
					for _, index := range plan.dependencies {
						source := plans[index].configured
						if source.Port == 0 && !source.AutoPort {
							issue("error", "dependency_endpoint", plan.configured.ID, "Referenced service has no declared listening port")
						}
					}
					for name, index := range plan.dependencies {
						endpoint := plan.configured.EnvFrom[name]
						if endpoint.Field != "port" && plans[index].configured.URL == "" {
							issue("error", "dependency_endpoint", plan.configured.ID, "Referenced service has no page or API URL")
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
		for _, warning := range overview.Warnings {
			issue("warning", "runtime_discovery", "", warning)
		}
		for _, state := range overview.Services {
			if !localServiceRunning(state.Status) && state.PortState == "occupied" && !state.Definition.AutoPort {
				issue("error", "port_occupied", state.Definition.ID, fmt.Sprintf("Fixed port %d is occupied", state.Definition.Port))
			}
			if state.ConfigChanged {
				issue("warning", "restart_required", state.Definition.ID, "Configuration changed; running instance not updated")
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
				issue("warning", "resource_modified", resource.ID, "Shared file has project differences; updates will be blocked")
			case "missing", "outdated":
				issue("warning", "resource_update", resource.ID, "Shared file not synced to current source version")
			}
		}
	}
	if len(report.Services) == 0 && len(report.Pipelines) == 0 {
		issue("error", "commands_missing", "", "No start or check entry configured; ask AI to read project guides and configure it")
	}
	report.AIResources = []AIResource{}
	declarationsIssues := []string{}
	for _, declaration := range readAIIntent(path, &declarationsIssues) {
		resource := AIResource{ID: aiID(declaration.Kind, declaration.Name+":"+declaration.Source), Kind: declaration.Kind, Name: declaration.Name, Source: ".repsync.json", Location: declaration.Source, Availability: "unknown", Applicability: "adopted", Reason: declaration.Reason, Tasks: declaration.Tasks, Projects: []string{path}, Detail: "Declared as adopted; session visibility and usage not verified"}
		if declaration.Kind == "cli" {
			command, err := projectCommand(context.Background(), path, "", nil, []string{declaration.Name}, false)
			if err != nil {
				resource.Availability, resource.Detail = "missing", "CLI not found in the repsync execution environment"
				issue("warning", "ai_resource", declaration.Name, resource.Detail)
			} else {
				resource.Availability, resource.Location = "available", command.Path
			}
		}
		if declaration.Kind == "skill" && declaration.Source != "" {
			if info, err := os.Stat(declaration.Source); err != nil || !info.Mode().IsRegular() {
				resource.Availability, resource.Detail = "missing", "Declared Skill source missing or not a regular file"
				issue("warning", "ai_resource", declaration.Name, resource.Detail)
			} else {
				resource.Availability = "discovered"
			}
		}
		report.AIResources = append(report.AIResources, resource)
	}
	for _, message := range declarationsIssues {
		issue("warning", "ai_configuration", "", message)
	}
	return report, nil
}
