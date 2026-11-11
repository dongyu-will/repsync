package workspace

import (
	"bufio"
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"
	"unicode/utf8"
)

type claudeMemoryEntry struct {
	Type             string `json:"type"`
	SessionID        string `json:"sessionId"`
	UUID             string `json:"uuid"`
	ParentUUID       string `json:"parentUuid"`
	Cwd              string `json:"cwd"`
	Timestamp        string `json:"timestamp"`
	IsSidechain      bool   `json:"isSidechain"`
	IsMeta           bool   `json:"isMeta"`
	IsCompactSummary bool   `json:"isCompactSummary"`
	IsAPIError       bool   `json:"isApiErrorMessage"`
	Message          struct {
		Role       string          `json:"role"`
		Content    json.RawMessage `json:"content"`
		StopReason string          `json:"stop_reason"`
	} `json:"message"`
}

func claudeMemoryText(entry claudeMemoryEntry) string {
	var text string
	if json.Unmarshal(entry.Message.Content, &text) != nil {
		var blocks []struct {
			Type string `json:"type"`
			Text string `json:"text"`
		}
		if json.Unmarshal(entry.Message.Content, &blocks) != nil {
			return ""
		}
		var parts []string
		for _, block := range blocks {
			if block.Type == "tool_result" {
				return ""
			}
			if block.Type == "text" {
				parts = append(parts, block.Text)
			}
		}
		text = strings.Join(parts, "\n")
	}
	return text
}

func claudeMemoryUserText(entry claudeMemoryEntry) string {
	if entry.Type != "user" || entry.Message.Role != "user" || entry.IsSidechain || entry.IsMeta || entry.IsCompactSummary {
		return ""
	}
	text := claudeMemoryText(entry)
	trimmed := strings.TrimSpace(text)
	for _, prefix := range []string{"[Request interrupted by user", "<command-", "<local-command-", "<task-notification", "<system-reminder", "<ide_opened_file", "<ide_selection"} {
		if strings.HasPrefix(trimmed, prefix) {
			return ""
		}
	}
	if !utf8.ValidString(text) || trimmed == "" {
		return ""
	}
	return text
}

func claudeMemoryMetadata(scanner *bufio.Scanner, name string) (MemorySession, string, error) {
	for scanner.Scan() {
		var entry claudeMemoryEntry
		if json.Unmarshal(scanner.Bytes(), &entry) != nil {
			return MemorySession{}, "", errors.New("无法读取 Claude Code 会话元数据")
		}
		if entry.IsSidechain {
			return MemorySession{}, "", nil
		}
		text := claudeMemoryUserText(entry)
		if text == "" || entry.SessionID == "" || entry.Cwd == "" {
			continue
		}
		if len(entry.SessionID) > 200 || strings.ContainsAny(entry.SessionID, "\x00\r\n") {
			return MemorySession{}, "", errors.New("会话编号无效")
		}
		return MemorySession{Provider: "claude", ID: entry.SessionID, File: name,
			Title: truncateAIText(strings.TrimSpace(text), 120), Started: entry.Timestamp}, evidencePath(entry.Cwd), nil
	}
	return MemorySession{}, "", errors.New("无法读取 Claude Code 用户会话")
}

func readClaudeMemoryMessages(directory, path, name, expectedID string) (MemorySession, []memoryMessage, bool, error) {
	if len(strings.Split(filepath.ToSlash(name), "/")) != 2 {
		return MemorySession{}, nil, false, errors.New("请选择 Claude Code 主会话")
	}
	root, err := os.OpenRoot(directory)
	if err != nil {
		return MemorySession{}, nil, false, err
	}
	defer root.Close()
	file, err := openMemoryLog(root, name)
	if err != nil {
		return MemorySession{}, nil, false, err
	}
	defer file.Close()
	metadata := memoryScanner(io.LimitReader(file, 2*1024*1024))
	session, cwd, err := claudeMemoryMetadata(metadata, name)
	if err != nil || session.ID == "" || session.ID != expectedID || cwd == "" || !withinAIScope(cwd, path) {
		return session, nil, false, errors.New("该会话不属于当前项目或已变化，请重新选择")
	}
	if _, err := file.Seek(0, io.SeekStart); err != nil {
		return session, nil, false, err
	}
	reader := &io.LimitedReader{R: file, N: memoryFileLimit}
	scanner := memoryScanner(reader)
	var current *memoryMessage
	var messages []memoryMessage
	seen := map[string]bool{}
	chain := map[string]bool{}
	partial, belongs := false, false
	deadline := time.Now().Add(5 * time.Second)
	for line := 1; scanner.Scan(); line++ {
		if line%100 == 0 && time.Now().After(deadline) {
			partial = true
			break
		}
		var entry claudeMemoryEntry
		if json.Unmarshal(scanner.Bytes(), &entry) != nil {
			partial, current = true, nil
			clear(chain)
			continue
		}
		if entry.IsSidechain {
			return session, nil, false, errors.New("不能从子 Agent 会话提炼用户记忆")
		}
		if entry.Type != "user" && entry.Type != "assistant" {
			continue
		}
		if entry.SessionID != expectedID || expectedID == "" {
			return session, nil, false, errors.New("会话编号已变化，请重新选择")
		}
		if !withinAIScope(evidencePath(entry.Cwd), path) {
			current = nil
			clear(chain)
			continue
		}
		belongs = true
		if session.Started == "" {
			session.Started = entry.Timestamp
		}
		linked := entry.ParentUUID != "" && chain[entry.ParentUUID]
		if linked && entry.UUID != "" {
			chain[entry.UUID] = true
		}
		if entry.Type == "user" {
			text := claudeMemoryUserText(entry)
			if text == "" {
				if entry.IsCompactSummary || (!entry.IsMeta && claudeMemoryText(entry) != "") {
					current = nil
					clear(chain)
				}
				continue // Tool results and injected context are not authored evidence.
			}
			current = nil
			clear(chain)
			if len(text) > memoryTextLimit {
				partial = true
				continue
			}
			if entry.UUID == "" || seen[entry.UUID] {
				continue
			}
			seen[entry.UUID] = true
			chain[entry.UUID] = true
			current = &memoryMessage{Line: line, Text: text, Timestamp: entry.Timestamp}
		} else if linked && entry.Message.Role == "assistant" && !entry.IsAPIError &&
			(entry.Message.StopReason == "end_turn" || entry.Message.StopReason == "stop_sequence") {
			if current != nil {
				messages = append(messages, *current)
			}
			current = nil
			clear(chain)
		}
	}
	if !belongs {
		return session, nil, false, errors.New("该会话不属于当前项目或已变化，请重新选择")
	}
	partial = partial || scanner.Err() != nil || reader.N == 0
	messages, truncated := recentMemoryMessages(messages)
	return session, messages, partial || truncated, nil
}
