package workspace

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"unicode/utf8"
)

const projectMapFile = ".repsync-map.json"

// ProjectMapReference locates a declared implementation or entry in the current files.
// Line and Issue are recomputed on every read, never trusted from the document.
type ProjectMapReference struct {
	Path  string `json:"path"`
	Find  string `json:"find,omitempty"`
	Line  int    `json:"line,omitempty"`
	Issue string `json:"issue,omitempty"`
}

type ProjectMapImplementation struct {
	Name   string               `json:"name"`
	Source ProjectMapReference  `json:"source"`
	Entry  *ProjectMapReference `json:"entry,omitempty"`
}

type ProjectMapModule struct {
	ID              string                     `json:"id"`
	Name            string                     `json:"name"`
	Summary         string                     `json:"summary,omitempty"`
	Overlap         string                     `json:"overlap,omitempty"`
	Implementations []ProjectMapImplementation `json:"implementations"`
}

type ProjectMap struct {
	File    string             `json:"file"`
	Exists  bool               `json:"exists"`
	Modules []ProjectMapModule `json:"modules"`
}

// ProjectMap reads a feature-first map maintained by the project's existing AI tools.
// It checks cited files and unique entry text; it does not infer semantic duplicates
// or claim that a located entry is reachable at runtime.
func (s *Service) ProjectMap(path string) (ProjectMap, error) {
	view := ProjectMap{File: projectMapFile, Modules: []ProjectMapModule{}}
	if _, err := s.registeredProject(path); err != nil {
		return view, err
	}
	root, err := os.OpenRoot(path)
	if err != nil {
		return view, err
	}
	defer root.Close()
	data, err := readProjectConfig(root, projectMapFile)
	if errors.Is(err, os.ErrNotExist) {
		return view, nil
	}
	if err != nil {
		return view, fmt.Errorf("read project map: %w", err)
	}
	var document struct {
		Version int                `json:"version"`
		Modules []ProjectMapModule `json:"modules"`
	}
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&document); err != nil {
		return view, fmt.Errorf("invalid project map: %w", err)
	}
	if err := decoder.Decode(new(any)); !errors.Is(err, io.EOF) {
		return view, errors.New("project map must contain exactly one JSON object")
	}
	if document.Version != 1 {
		return view, errors.New("project map version must be 1")
	}
	if err := validateProjectMap(document.Modules); err != nil {
		return view, err
	}
	view.Exists, view.Modules = true, document.Modules
	if view.Modules == nil {
		view.Modules = []ProjectMapModule{}
	}
	// Several modules often cite the same navigation file. Read each file once
	// and bound the total; an unreadable reference does not hide the other modules.
	type sourceFile struct {
		lines []string
		issue string
	}
	files := map[string]sourceFile{}
	total := 0
	locate := func(ref *ProjectMapReference) {
		ref.Line, ref.Issue = 0, ""
		file, loaded := files[ref.Path]
		if !loaded {
			if total >= 8*1024*1024 {
				file.issue = "Map reference read limit reached"
			} else {
				content, err := readProjectConfig(root, ref.Path)
				if err != nil {
					file.issue = "Referenced file missing or unreadable"
				} else {
					total += len(content)
					file.lines = strings.Split(string(content), "\n")
				}
			}
			files[ref.Path] = file
		}
		ref.Issue = file.issue
		if ref.Issue != "" {
			return
		}
		if ref.Find == "" {
			ref.Line = 1
			return
		}
		for index, line := range file.lines {
			if !strings.Contains(line, ref.Find) {
				continue
			}
			if ref.Line != 0 {
				ref.Line, ref.Issue = 0, "Reference matches more than one line"
				return
			}
			ref.Line = index + 1
		}
		if ref.Line == 0 {
			ref.Issue = "Reference text no longer found"
		}
	}
	for module := range view.Modules {
		for implementation := range view.Modules[module].Implementations {
			item := &view.Modules[module].Implementations[implementation]
			locate(&item.Source)
			if item.Entry != nil {
				locate(item.Entry)
			}
		}
	}
	return view, nil
}

func validateProjectMap(modules []ProjectMapModule) error {
	if len(modules) > 64 {
		return errors.New("project map supports up to 64 modules")
	}
	ids := map[string]bool{}
	references := 0
	for _, module := range modules {
		invalid := strings.TrimSpace(module.ID) == "" || strings.TrimSpace(module.Name) == ""
		tooLong := len(module.ID) > 128 || len(module.Name) > 256 || len(module.Summary) > 2048 || len(module.Overlap) > 2048
		if invalid || tooLong || ids[module.ID] {
			return errors.New("map modules require unique IDs and nonempty names within length limits")
		}
		ids[module.ID] = true
		if len(module.Implementations) == 0 || len(module.Implementations) > 8 {
			return errors.New("each map module requires 1–8 implementations")
		}
		sources := map[string]bool{}
		for _, implementation := range module.Implementations {
			identity := implementation.Source.Path + "\x00" + implementation.Source.Find
			if strings.TrimSpace(implementation.Name) == "" || len(implementation.Name) > 256 || sources[identity] {
				return errors.New("map implementations require names and distinct source references")
			}
			sources[identity] = true
			refs := []ProjectMapReference{implementation.Source}
			if implementation.Entry != nil {
				if strings.TrimSpace(implementation.Entry.Find) == "" {
					return errors.New("map entry references require specific text to locate")
				}
				refs = append(refs, *implementation.Entry)
			}
			for _, ref := range refs {
				if !filepath.IsLocal(ref.Path) || ref.Path == "." || filepath.Clean(ref.Path) == projectMapFile {
					return errors.New("map references must be project-relative files, excluding the map itself")
				}
				if len(ref.Path) > 2048 || len(ref.Find) > 256 || strings.ContainsAny(ref.Path+ref.Find, "\x00\r\n") {
					return errors.New("map reference paths and text must be bounded single-line values")
				}
			}
			references += len(refs)
		}
		fields := []string{module.ID, module.Name, module.Summary, module.Overlap}
		for _, field := range fields {
			if !utf8.ValidString(field) || strings.ContainsRune(field, '\x00') {
				return errors.New("project map must use UTF-8 text without NUL")
			}
		}
	}
	if references > 256 {
		return errors.New("project map supports up to 256 references")
	}
	return nil
}
