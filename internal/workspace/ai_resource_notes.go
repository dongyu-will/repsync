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
	"unicode/utf8"
)

// AIResourceNoteData is an AI-authored report, not independent proof of an update.
type AIResourceNoteData struct {
	ResourceID       string   `json:"resourceId"`
	ResourceRevision string   `json:"resourceRevision"`
	InstalledVersion string   `json:"installedVersion"`
	LatestVersion    string   `json:"latestVersion"`
	UpdateStatus     string   `json:"updateStatus"`
	CheckedAt        string   `json:"checkedAt"`
	Summary          string   `json:"summary"`
	Changes          string   `json:"changes"`
	Manual           string   `json:"manual"`
	Sources          []string `json:"sources"`
}

type AIResourceNoteInput struct {
	AIResourceNoteData
	ExpectedVersion string `json:"expectedVersion"`
}

type AIResourceNote struct {
	AIResourceNoteData
	RecordedAt string `json:"recordedAt"`
	Version    string `json:"version"`
	Stale      bool   `json:"stale"`
}

func (s *Service) aiResourceNotePath(scope, id string) string {
	return filepath.Join(filepath.Dir(s.path), "ai-resource-notes", gitDigest(scope), gitDigest(id)+".json")
}

func (s *Service) readAIResourceNote(scope, id string) (*AIResourceNote, error) {
	data, err := readReviewData(s.aiResourceNotePath(scope, id))
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var note AIResourceNote
	if err := json.Unmarshal(data, &note); err != nil {
		return nil, errors.New("cannot read AI resource update and manual record")
	}
	if note.ResourceID != id {
		return nil, errors.New("AI resource record identity does not match")
	}
	if err := validateAIResourceNote(note.AIResourceNoteData); err != nil {
		return nil, err
	}
	if note.Sources == nil {
		note.Sources = []string{}
	}
	note.Version, note.Stale = "", false
	note.Version = gitDigest(note)
	return &note, nil
}

func (s *Service) attachAIResourceNotes(report *AIResourceReport) {
	for i := range report.Resources {
		resource := &report.Resources[i]
		resource.Note = nil
		if resource.Revision == "" {
			resource.Revision = gitDigest([]string{
				resource.ID, resource.Location, resource.Version, resource.Availability,
			})
		}
		note, err := s.readAIResourceNote(report.Scope, resource.ID)
		if err == nil && note == nil && report.Scope != "" {
			note, err = s.readAIResourceNote("", resource.ID)
			// A global MCP with the same name can point at a different project server.
			if note != nil && resource.Kind == "mcp" && note.ResourceRevision != resource.Revision {
				note = nil
			}
		}
		if err != nil {
			report.Issues = append(report.Issues, "AI resource notes: "+resource.Name+": "+err.Error())
			continue
		}
		if note != nil {
			note.Stale = note.ResourceRevision != resource.Revision
			resource.Note = note
		}
	}
}

func validateAIResourceNote(data AIResourceNoteData) error {
	if data.ResourceID == "" || data.ResourceRevision == "" || strings.TrimSpace(data.Summary) == "" {
		return errors.New("provide resourceId, resourceRevision and a summary")
	}
	if !slices.Contains([]string{"current", "available", "unknown", "failed"}, data.UpdateStatus) {
		return errors.New("updateStatus must be current, available, unknown or failed")
	}
	checked, err := time.Parse(time.RFC3339, data.CheckedAt)
	if err != nil || checked.After(time.Now().Add(5*time.Minute)) {
		return errors.New("checkedAt must be an RFC3339 time, not in the future")
	}
	known := data.UpdateStatus == "current" || data.UpdateStatus == "available"
	if known && (data.InstalledVersion == "" || data.LatestVersion == "" || len(data.Sources) == 0) {
		return errors.New("known update status requires installedVersion, latestVersion and sources")
	}
	if len(data.Sources) > 16 {
		return errors.New("at most 16 sources allowed")
	}
	for _, source := range data.Sources {
		if strings.TrimSpace(source) == "" || len(source) > 2048 {
			return errors.New("sources must be nonempty and at most 2048 bytes each")
		}
	}
	fields := []string{data.ResourceID, data.ResourceRevision, data.InstalledVersion, data.LatestVersion}
	for _, field := range fields {
		if len(field) > 512 || strings.ContainsAny(field, "\x00\r\n") {
			return errors.New("resource identifiers and versions must be single-line text up to 512 bytes")
		}
	}
	encoded, err := json.Marshal(data)
	if err != nil || len(encoded) > 64*1024 {
		return errors.New("AI resource report exceeds 64 KiB; shorten the manual")
	}
	textFields := append(fields, data.Summary, data.Changes, data.Manual)
	textFields = append(textFields, data.Sources...)
	for _, field := range textFields {
		if !utf8.ValidString(field) || strings.ContainsRune(field, '\x00') {
			return errors.New("AI resource report must use UTF-8 text without NUL")
		}
	}
	return nil
}

// RecordAIResourceNote saves an AI report against the inventoried resource snapshot.
// Installation, upstream checks and verification remain with the existing AI tools.
func (s *Service) RecordAIResourceNote(path string, input AIResourceNoteInput) (AIResourceNote, error) {
	if input.Sources == nil {
		input.Sources = []string{}
	}
	if err := validateAIResourceNote(input.AIResourceNoteData); err != nil {
		return AIResourceNote{}, err
	}
	report, err := s.AIResources(path)
	if err != nil {
		return AIResourceNote{}, err
	}
	index := slices.IndexFunc(report.Resources, func(resource AIResource) bool {
		return resource.ID == input.ResourceID
	})
	if index < 0 {
		return AIResourceNote{}, errors.New("resource not inventoried; rescan before recording")
	}
	resource := report.Resources[index]
	readable := !slices.Contains([]string{"error", "missing", "candidate"}, resource.Availability)
	if resource.Revision != input.ResourceRevision || !readable {
		return AIResourceNote{}, errors.New("resource changed or is unreadable; rescan and regenerate the report")
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	existing, err := s.readAIResourceNote(path, input.ResourceID)
	if err != nil {
		return AIResourceNote{}, err
	}
	// Global notes displayed in a project are references, not project-owned records.
	expected := ""
	if existing != nil {
		expected = existing.Version
	} else if path != "" {
		global, err := s.readAIResourceNote("", input.ResourceID)
		if err != nil {
			return AIResourceNote{}, err
		}
		if global != nil && (resource.Kind != "mcp" || global.ResourceRevision == resource.Revision) {
			expected = global.Version
		}
	}
	if input.ExpectedVersion != expected {
		return AIResourceNote{}, errors.New("AI resource report changed; reread before replacing it")
	}
	note := AIResourceNote{AIResourceNoteData: input.AIResourceNoteData, RecordedAt: time.Now().UTC().Format(time.RFC3339)}
	if err := saveJSON(s.aiResourceNotePath(path, input.ResourceID), note); err != nil {
		return AIResourceNote{}, fmt.Errorf("save AI resource report: %w", err)
	}
	note.Version = gitDigest(note)
	return note, nil
}
