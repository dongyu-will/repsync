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
		return errors.New("请选择项目根目录中的 Markdown 文件")
	}
	return s.saveDocument(path, name, expected, content)
}

func (s *Service) saveDocument(path, name, expected, content string) error {
	if !utf8.ValidString(content) || len(content) > 256*1024 {
		return errors.New("Markdown 需要是 UTF-8 文本，且不能超过 256 KiB")
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
		return errors.New("只能编辑普通 Markdown 文件，不能修改符号链接")
	}
	if info.Mode().Perm()&0222 == 0 {
		return errors.New("文件为只读，请先检查文件权限")
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
		return errors.New("文件已被其他操作修改，草稿已保留；请复制需要的内容，再重新载入文件")
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
		return errors.New("文件或权限在保存期间发生变化，请重新载入")
	}
	latest, err := readCurrent()
	if err != nil {
		return err
	}
	if latest != expected {
		return errors.New("文件在保存期间发生变化，草稿已保留；请重新载入")
	}
	return root.Rename(tempName, name)
}
