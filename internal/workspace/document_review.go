package workspace

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"unicode/utf8"
)

const documentReviewMaxFile = 256 * 1024

type DocumentReviewEvidence struct {
	Source   string `json:"source"`
	Path     string `json:"path"`
	Line     int    `json:"line"`
	Detail   string `json:"detail"`
	MemoryID string `json:"memoryId,omitempty"`
}

type DocumentFinding struct {
	Path       string                   `json:"path"`
	StartLine  int                      `json:"startLine"`
	EndLine    int                      `json:"endLine"`
	Version    string                   `json:"version"`
	Original   string                   `json:"original"`
	Terms      []string                 `json:"terms"`
	Evidence   []DocumentReviewEvidence `json:"evidence"`
	Suggestion string                   `json:"suggestion"`
}

type DocumentReview struct {
	Basis    string            `json:"basis"`
	Files    int               `json:"files"`
	Graph    string            `json:"graph"`
	Warnings []string          `json:"warnings"`
	Findings []DocumentFinding `json:"findings"`
}

type DocumentReviewEdit struct {
	Scope       string `json:"scope"`
	Basis       string `json:"basis"`
	Path        string `json:"path"`
	StartLine   int    `json:"startLine"`
	EndLine     int    `json:"endLine"`
	Version     string `json:"version"`
	Replacement string `json:"replacement"`
}

type DocumentReviewPreview struct {
	Before string `json:"before"`
	After  string `json:"after"`
	Token  string `json:"token"`
}

// PreviewDocumentReview checks the Git and document snapshots before preparing a single edit.
func (s *Service) PreviewDocumentReview(path string, edit DocumentReviewEdit) (DocumentReviewPreview, error) {
	preview, _, _, err := s.prepareDocumentReviewEdit(path, edit)
	return preview, err
}

// ApplyDocumentReview writes exactly the previewed change, preserving concurrent edits.
func (s *Service) ApplyDocumentReview(path string, edit DocumentReviewEdit, token string) error {
	s.memoryMu.Lock()
	defer s.memoryMu.Unlock()
	preview, original, updated, err := s.prepareDocumentReviewEdit(path, edit)
	if err != nil {
		return err
	}
	if token != preview.Token {
		return errors.New("changes have changed; generate a new preview")
	}
	return s.saveDocument(path, edit.Path, original, updated)
}

func (s *Service) prepareDocumentReviewEdit(path string, edit DocumentReviewEdit) (DocumentReviewPreview, string, string, error) {
	var preview DocumentReviewPreview
	if _, err := s.registeredProject(path); err != nil {
		return preview, "", "", err
	}
	if !reviewDocumentPath(edit.Path) || !utf8.ValidString(edit.Replacement) || len(edit.Replacement) > documentReviewMaxFile {
		return preview, "", "", errors.New("select a reviewable Markdown file and check the edit length")
	}
	_, basis, _, _, err := s.documentReviewContext(path, edit.Scope)
	if err != nil {
		return preview, "", "", err
	}
	if basis != edit.Basis {
		return preview, "", "", errors.New("Git changes or confirmed memories changed; rescan documents")
	}
	root, err := os.OpenRoot(path)
	if err != nil {
		return preview, "", "", err
	}
	defer root.Close()
	original, err := readReviewDocument(root, edit.Path)
	if err != nil {
		return preview, "", "", err
	}
	if gitDigest(original) != edit.Version {
		return preview, "", "", errors.New("document changed; rescan before previewing")
	}
	lines := reviewDocumentLines(original)
	if edit.StartLine < 1 || edit.EndLine < edit.StartLine || edit.EndLine > len(lines) {
		return preview, "", "", errors.New("paragraph location is stale; rescan")
	}
	before := strings.Join(lines[edit.StartLine-1:edit.EndLine], "")
	after := strings.ReplaceAll(edit.Replacement, "\r\n", "\n")
	if after != "" && strings.HasSuffix(before, "\n") && !strings.HasSuffix(after, "\n") {
		after += "\n"
	}
	if strings.Contains(before, "\r\n") {
		after = strings.ReplaceAll(after, "\n", "\r\n")
	}
	updated := strings.Join(lines[:edit.StartLine-1], "") + after + strings.Join(lines[edit.EndLine:], "")
	if updated == original {
		return preview, "", "", errors.New("paragraph content is unchanged")
	}
	if len(updated) > documentReviewMaxFile {
		return preview, "", "", errors.New("edited file exceeds 256 KiB")
	}
	preview = DocumentReviewPreview{
		Before: before, After: after, Token: gitDigest([]any{path, edit, before, after}),
	}
	return preview, original, updated, nil
}

func reviewDocumentPath(name string) bool {
	if !filepath.IsLocal(name) || filepath.Clean(name) != name || !markdownFile(name) {
		return false
	}
	parts := strings.Split(filepath.ToSlash(name), "/")
	if len(parts) > 1 && parts[0] != "docs" {
		return false
	}
	for _, part := range parts {
		switch strings.ToLower(part) {
		case "archive", "archives", "history", "changelog", "migrations", "node_modules", "vendor":
			return false
		}
		if strings.HasPrefix(part, ".") {
			return false
		}
	}
	switch strings.ToLower(strings.TrimSuffix(filepath.Base(name), filepath.Ext(name))) {
	case "changelog", "changes", "history", "releases":
		return false
	}
	return true
}

func readReviewDocument(root *os.Root, name string) (string, error) {
	info, err := root.Lstat(name)
	if err != nil {
		return "", err
	}
	if !info.Mode().IsRegular() || info.Size() > documentReviewMaxFile {
		return "", fmt.Errorf("%s must be a regular file no larger than 256 KiB", name)
	}
	file, err := root.Open(name)
	if err != nil {
		return "", err
	}
	defer file.Close()
	opened, err := file.Stat()
	if err != nil {
		return "", err
	}
	if !os.SameFile(info, opened) {
		return "", errors.New("document changed while reading")
	}
	content, err := readLimitedText(file)
	if err == nil && (!utf8.ValidString(content) || strings.ContainsRune(content, 0)) {
		err = errors.New("document must be UTF-8 text")
	}
	return content, err
}

func reviewDocumentLines(content string) []string {
	lines := strings.SplitAfter(content, "\n")
	if lines[len(lines)-1] == "" {
		lines = lines[:len(lines)-1]
	}
	return lines
}
