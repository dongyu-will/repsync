package workspace

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"
)

type ReviewResource struct {
	ID     string `json:"id"`
	Name   string `json:"name"`
	Kind   string `json:"kind"`
	Target string `json:"target"`
	Guide  string `json:"guide"`
}

type ReviewResourceState struct {
	Definition ReviewResource `json:"definition"`
	Observed   string         `json:"observed"`
	Version    string         `json:"version"`
	Size       int64          `json:"size"`
	ModifiedAt string         `json:"modifiedAt"`
	Tracking   string         `json:"tracking"`
	Error      string         `json:"error"`
}

func (s *Service) reviewResources(path string) ([]ReviewResourceState, string, error) {
	states := []ReviewResourceState{}
	root, err := os.OpenRoot(path)
	if err != nil {
		return states, "", err
	}
	defer root.Close()
	file, err := loadProjectConfigFile(root)
	if err != nil {
		return states, "", err
	}
	definitions := []ReviewResource{}
	if data := file.fields["reviewResources"]; len(data) > 0 {
		if err := json.Unmarshal(data, &definitions); err != nil {
			return states, file.version, errors.New("invalid reviewResources configuration")
		}
	}
	if err := validateReviewResources(definitions); err != nil {
		return states, file.version, err
	}
	for _, d := range definitions {
		state := ReviewResourceState{Definition: d, Observed: "unknown", Tracking: "not_applicable"}
		switch d.Kind {
		case "environment":
			if _, ok := os.LookupEnv(d.Target); ok {
				state.Observed = "set"
			} else {
				state.Observed = "missing"
			}
		case "manual":
			state.Observed = "needs_confirmation"
		default:
			info, e := root.Stat(d.Target)
			if errors.Is(e, os.ErrNotExist) {
				state.Observed = "missing"
			} else if e != nil {
				state.Error = e.Error()
			} else {
				if d.Kind == "directory" && !info.IsDir() || d.Kind == "file" && !info.Mode().IsRegular() {
					state.Error = "Resource type does not match configuration"
				} else {
					state.Observed = "exists"
				}
				state.Size = info.Size()
				state.ModifiedAt = info.ModTime().UTC().Format(time.RFC3339Nano)
			}
			if _, e := git(path, "ls-files", "--error-unmatch", "--", d.Target); e == nil {
				state.Tracking = "tracked"
			} else if _, e := git(path, "check-ignore", "-q", "--", d.Target); e == nil {
				state.Tracking = "ignored"
			} else {
				state.Tracking = "untracked"
			}
		}
		state.Version = gitDigest(state)
		states = append(states, state)
	}
	return states, file.version, nil
}

func validateReviewResources(definitions []ReviewResource) error {
	if len(definitions) > 32 {
		return errors.New("up to 32 data and environment resources can be declared")
	}
	ids := map[string]bool{}
	for _, d := range definitions {
		if d.ID == "" || len(d.ID) > 80 || ids[d.ID] || strings.TrimSpace(d.Name) == "" || len(d.Name) > 200 || len(d.Guide) > 4096 {
			return errors.New("verification resources require unique IDs, names, and valid guidance")
		}
		ids[d.ID] = true
		if !slices.Contains([]string{"file", "directory", "environment", "manual"}, d.Kind) {
			return errors.New("invalid verification resource type")
		}
		if d.Kind == "environment" && !environmentName.MatchString(d.Target) {
			return errors.New("invalid environment variable name")
		}
		if (d.Kind == "file" || d.Kind == "directory") && (!filepath.IsLocal(d.Target) || d.Target == ".") {
			return fmt.Errorf("%s requires a project-relative path", d.Name)
		}
	}
	return nil
}

// SaveReviewResources preserves the project's services, pipelines and extension fields.
func (s *Service) SaveReviewResources(path, version string, definitions []ReviewResource) error {
	if err := validateReviewResources(definitions); err != nil {
		return err
	}
	return s.saveProjectSection(path, version, "reviewResources", false, definitions)
}
