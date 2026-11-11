package workspace

import (
	"crypto/rand"
	"errors"
	"os"
	"path/filepath"
	"unicode/utf8"
)

// SaveDocument updates an existing root Markdown file only if its loaded content is unchanged.
func (s *Service) SaveDocument(path, name, expected, content string) error {
	if _, err := s.registeredProject(path); err != nil {
		return err
	}
	if filepath.Base(name) != name || !markdownFile(name) {
		return errors.New("select a Markdown file in the project root")
	}
	return s.saveDocument(path, name, expected, content)
}

func (s *Service) saveDocument(path, name, expected, content string) error {
	if !utf8.ValidString(content) || len(content) > 256*1024 {
		return errors.New("Markdown must be UTF-8 text no larger than 256 KiB")
	}
	// Serialize with shared-rule updates, which can also modify AGENTS.md.
	s.syncMu.Lock()
	defer s.syncMu.Unlock()
	root, err := os.OpenRoot(path)
	if err != nil {
		return err
	}
	defer root.Close()
	info, err := root.Lstat(name)
	if err != nil {
		return err
	}
	if !info.Mode().IsRegular() {
		return errors.New("only regular Markdown files can be edited; symbolic links cannot be modified")
	}
	if info.Mode().Perm()&0222 == 0 {
		return errors.New("file is read-only; check its permissions")
	}
	readCurrent := func() (string, error) {
		f, err := root.Open(name)
		if err != nil {
			return "", err
		}
		defer f.Close()
		return readLimitedText(f)
	}
	before, err := readCurrent()
	if err != nil {
		return err
	}
	if before != expected {
		return errors.New("file changed externally; draft preserved; copy your edits before reloading")
	}
	tempName := ".repsync-document-" + rand.Text()
	temp, err := root.OpenFile(tempName, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
	if err != nil {
		return err
	}
	defer root.Remove(tempName)
	if _, err = temp.WriteString(content); err == nil {
		err = temp.Chmod(info.Mode().Perm())
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
	current, err := root.Lstat(name)
	if err != nil {
		return err
	}
	if !current.Mode().IsRegular() || !os.SameFile(info, current) || current.Mode().Perm() != info.Mode().Perm() {
		return errors.New("file or permissions changed while saving; reload")
	}
	latest, err := readCurrent()
	if err != nil {
		return err
	}
	if latest != expected {
		return errors.New("file changed while saving; draft preserved; reload")
	}
	return root.Rename(tempName, name)
}
