package workspace

import (
	"bufio"
	"encoding/json"
	"errors"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"
	"unicode/utf8"
)

const (
	codexMemoryFileLimit = 32 * 1024 * 1024
	codexMemoryTextLimit = 24 * 1024
)

type CodexMemorySession struct {
	ID       string `json:"id"`
	File     string `json:"file"`
	Title    string `json:"title"`
	Started  string `json:"started"`
	Modified string `json:"modified"`
}

type CodexMemorySessions struct {
	Sessions []CodexMemorySession `json:"sessions"`
	Partial  bool                 `json:"partial"`
}

type CodexMemoryCitation struct {
	SessionID string `json:"sessionId"`
	File      string `json:"file"`
	Line      int    `json:"line"`
	Timestamp string `json:"timestamp"`
	Quote     string `json:"quote"`
}

type codexMemoryLogEntry struct {
	Type      string          `json:"type"`
	Timestamp string          `json:"timestamp"`
	Payload   json.RawMessage `json:"payload"`
}

type codexMemoryMessage struct {
	Line      int    `json:"line"`
	Text      string `json:"text"`
	Timestamp string `json:"-"`
}

func codexMemorySessionsDir() (string, error) {
	paths, err := currentAIPaths()
	if err != nil {
		return "", err
	}
	return filepath.Join(paths.codex, "sessions"), nil
}

// ListCodexMemorySessions reads metadata for recent sessions started within the project.
func (s *Service) ListCodexMemorySessions(path string) (CodexMemorySessions, error) {
	result := CodexMemorySessions{Sessions: []CodexMemorySession{}}
	if _, err := s.registeredProject(path); err != nil {
		return result, err
	}
	directory, err := codexMemorySessionsDir()
	if err != nil {
		return result, err
	}
	root, err := os.OpenRoot(directory)
	if errors.Is(err, os.ErrNotExist) {
		return result, nil
	}
	if err != nil {
		return result, err
	}
	defer root.Close()
	type file struct {
		name     string
		modified time.Time
	}
	var files []file
	now := time.Now()
	since, deadline := now.AddDate(0, 0, -30), now.Add(5*time.Second)
	nodes := 0
	err = fs.WalkDir(root.FS(), ".", func(name string, entry fs.DirEntry, err error) error {
		if err != nil {
			result.Partial = true
			return nil
		}
		nodes++
		if nodes > 8000 || time.Now().After(deadline) {
			result.Partial = true
			return fs.SkipAll
		}
		if entry.Type()&fs.ModeSymlink != 0 || entry.IsDir() || filepath.Ext(name) != ".jsonl" {
			return nil
		}
		info, err := entry.Info()
		if err != nil {
			result.Partial = true
		} else if info.Mode().IsRegular() && !info.ModTime().Before(since) {
			files = append(files, file{name, info.ModTime()})
		}
		return nil
	})
	if err != nil {
		return result, err
	}
	slices.SortFunc(files, func(a, b file) int {
		if order := b.modified.Compare(a.modified); order != 0 {
			return order
		}
		return strings.Compare(a.name, b.name)
	})
	if len(files) > 256 {
		files = files[:256]
		result.Partial = true
	}
	budget := int64(16 * 1024 * 1024)
	seen := map[string]bool{}
	for _, file := range files {
		if len(result.Sessions) == 20 || budget <= 0 || time.Now().After(deadline) {
			result.Partial = true
			break
		}
		session, cwd, n, err := codexMemorySessionMeta(root, file.name, min(budget, int64(2*1024*1024)))
		budget -= n
		if err != nil {
			result.Partial = true
			continue
		}
		if session.ID == "" || cwd == "" || !withinAIScope(cwd, path) || seen[session.ID] {
			continue
		}
		seen[session.ID] = true
		session.Modified = file.modified.UTC().Format(time.RFC3339)
		result.Sessions = append(result.Sessions, session)
	}
	titles := codexMemoryTitles(filepath.Join(filepath.Dir(directory), "session_index.jsonl"))
	for i := range result.Sessions {
		result.Sessions[i].Title = titles[result.Sessions[i].ID]
	}
	return result, nil
}

func codexMemoryTitles(path string) map[string]string {
	titles := map[string]string{}
	info, err := os.Lstat(path)
	if err != nil || !info.Mode().IsRegular() {
		return titles
	}
	file, err := os.Open(path)
	if err != nil {
		return titles
	}
	defer file.Close()
	scanner := bufio.NewScanner(io.LimitReader(file, 2*1024*1024))
	for scanner.Scan() {
		var item struct {
			ID   string
			Name string `json:"thread_name"`
		}
		if json.Unmarshal(scanner.Bytes(), &item) == nil && item.ID != "" && strings.TrimSpace(item.Name) != "" {
			titles[item.ID] = truncateAIText(strings.TrimSpace(item.Name), 120)
		}
	}
	return titles
}

func openCodexMemoryLog(root *os.Root, name string) (*os.File, error) {
	if filepath.Ext(name) != ".jsonl" {
		return nil, errors.New("请选择 Codex 会话")
	}
	if err := validateUpdatePath(root, name); err != nil {
		return nil, errors.New("会话路径不在 Codex 会话目录中")
	}
	info, err := root.Lstat(name)
	if err != nil || !info.Mode().IsRegular() {
		return nil, errors.New("会话文件不可读取")
	}
	file, err := root.Open(name)
	if err != nil {
		return nil, err
	}
	opened, err := file.Stat()
	if err != nil || !os.SameFile(info, opened) {
		file.Close()
		return nil, errors.New("会话文件已变化，请重新选择")
	}
	return file, nil
}

func codexMemorySessionMeta(root *os.Root, name string, limit int64) (CodexMemorySession, string, int64, error) {
	file, err := openCodexMemoryLog(root, name)
	if err != nil {
		return CodexMemorySession{}, "", 0, err
	}
	defer file.Close()
	reader := &io.LimitedReader{R: file, N: limit}
	scanner := codexMemoryScanner(reader)
	session, cwd, err := codexMemoryMetadata(scanner, name)
	return session, cwd, limit - reader.N, err
}

func codexMemoryScanner(reader io.Reader) *bufio.Scanner {
	scanner := bufio.NewScanner(reader)
	scanner.Buffer(make([]byte, 32*1024), 2*1024*1024)
	return scanner
}

func codexMemoryMetadata(scanner *bufio.Scanner, name string) (CodexMemorySession, string, error) {
	var entry codexMemoryLogEntry
	var meta struct {
		ID        string          `json:"id"`
		SessionID string          `json:"session_id"`
		Cwd       string          `json:"cwd"`
		Timestamp string          `json:"timestamp"`
		Source    json.RawMessage `json:"source"`
	}
	if !scanner.Scan() || json.Unmarshal(scanner.Bytes(), &entry) != nil || entry.Type != "session_meta" ||
		json.Unmarshal(entry.Payload, &meta) != nil {
		return CodexMemorySession{}, "", errors.New("无法读取会话元数据")
	}
	if len(meta.Source) > 0 && meta.Source[0] == '{' {
		return CodexMemorySession{}, "", nil // agent-generated sessions
	}
	if meta.ID == "" {
		meta.ID = meta.SessionID
	}
	if len(meta.ID) > 200 || strings.ContainsAny(meta.ID, "\x00\r\n") {
		return CodexMemorySession{}, "", errors.New("会话编号无效")
	}
	return CodexMemorySession{ID: meta.ID, File: name, Started: meta.Timestamp}, evidencePath(meta.Cwd), nil
}

func readCodexMemoryMessages(directory, path, fileName, expectedID string) (CodexMemorySession, []codexMemoryMessage, bool, error) {
	root, err := os.OpenRoot(directory)
	if err != nil {
		return CodexMemorySession{}, nil, false, err
	}
	defer root.Close()
	file, err := openCodexMemoryLog(root, fileName)
	if err != nil {
		return CodexMemorySession{}, nil, false, err
	}
	defer file.Close()
	reader := &io.LimitedReader{R: file, N: codexMemoryFileLimit}
	scanner := codexMemoryScanner(reader)
	session, cwd, err := codexMemoryMetadata(scanner, fileName)
	if err != nil || session.ID == "" || session.ID != expectedID || cwd == "" || !withinAIScope(cwd, path) {
		return session, nil, false, errors.New("该会话不属于当前项目或已变化，请重新选择")
	}
	partial := false
	line := 1
	turn, turnPath := "", ""
	completed := map[string]bool{}
	messages := []codexMemoryMessage{}
	current := []codexMemoryMessage{}
	add := func(text, timestamp string) {
		if len(text) > codexMemoryTextLimit {
			partial = true
			return
		}
		if strings.TrimSpace(text) == "" || !utf8.ValidString(text) ||
			slices.ContainsFunc(current, func(item codexMemoryMessage) bool { return item.Text == text }) {
			return
		}
		current = append(current, codexMemoryMessage{Line: line, Text: text, Timestamp: timestamp})
	}
	deadline := time.Now().Add(5 * time.Second)
	for scanner.Scan() {
		line++
		if line%100 == 0 && time.Now().After(deadline) {
			partial = true
			break
		}
		var entry codexMemoryLogEntry
		if err := json.Unmarshal(scanner.Bytes(), &entry); err != nil {
			partial = true
			turn, turnPath, current = "", "", nil
			continue
		}
		switch entry.Type {
		case "turn_context":
			var context struct {
				Cwd    string
				TurnID string `json:"turn_id"`
			}
			if json.Unmarshal(entry.Payload, &context) == nil && context.TurnID == turn {
				turnPath = evidencePath(context.Cwd)
			}
		case "event_msg":
			var event struct {
				Type    string `json:"type"`
				TurnID  string `json:"turn_id"`
				Message string `json:"message"`
			}
			if json.Unmarshal(entry.Payload, &event) != nil {
				continue
			}
			switch event.Type {
			case "task_started", "turn_started":
				turn, turnPath, current = event.TurnID, "", nil
			case "task_complete", "turn_completed":
				if turn != "" && event.TurnID == turn && !completed[turn] && turnPath != "" && withinAIScope(turnPath, path) {
					messages = append(messages, current...)
					completed[turn] = true
				}
				turn, turnPath, current = "", "", nil
			case "turn_aborted":
				turn, turnPath, current = "", "", nil
			case "user_message":
				if turn != "" {
					add(event.Message, entry.Timestamp)
				}
			}
		case "response_item":
			if turn != "" {
				for _, text := range codexAuthoredText(entry.Payload, turn) {
					add(text, entry.Timestamp)
				}
			}
		}
	}
	partial = partial || scanner.Err() != nil || reader.N == 0
	bytes, start := 0, len(messages)
	for start > 0 && len(messages)-start < 48 {
		next := len(messages[start-1].Text)
		if bytes+next > codexMemoryTextLimit {
			break
		}
		bytes += next
		start--
	}
	return session, messages[start:], partial || start > 0, nil
}

func codexAuthoredText(raw json.RawMessage, turn string) []string {
	var item struct {
		Type    string `json:"type"`
		Role    string `json:"role"`
		Content []struct {
			Type string `json:"type"`
			Text string `json:"text"`
		} `json:"content"`
		Metadata struct {
			Turn  string   `json:"turn_id"`
			Kinds []string `json:"content_item_kinds"`
		} `json:"internal_chat_message_metadata_passthrough"`
	}
	if json.Unmarshal(raw, &item) != nil || item.Type != "message" || item.Role != "user" ||
		item.Metadata.Turn != turn || len(item.Metadata.Kinds) != len(item.Content) {
		return nil
	}
	var texts []string
	for i, content := range item.Content {
		if item.Metadata.Kinds[i] == "user.text" && content.Type == "input_text" {
			texts = append(texts, content.Text)
		}
	}
	return texts
}
