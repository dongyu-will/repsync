package workspace

import (
	"errors"
	"path/filepath"
	"reflect"
	"slices"
	"strconv"
	"strings"

	"github.com/pelletier/go-toml/v2"
	"github.com/pelletier/go-toml/v2/unstable"
)

func planCodexUserMCP(directory string, change CodexResourceChange) (codexPlacementPlan, error) {
	var plan codexPlacementPlan
	before, err := readUpdateTarget(directory, "config.toml")
	if err != nil {
		return plan, err
	}
	if !before.Exists || before.Mode&0222 == 0 {
		return plan, errors.New("Codex 用户配置不存在或为只读")
	}
	var config map[string]any
	if err := toml.Unmarshal([]byte(before.Content), &config); err != nil {
		return plan, errors.New("Codex 用户配置无法解析，请先修复 TOML")
	}
	servers, _ := config["mcp_servers"].(map[string]any)
	server, _ := servers[change.Name].(map[string]any)
	if server["command"] == nil && server["url"] == nil {
		return plan, errors.New("Codex 用户配置中没有此 MCP 的连接定义")
	}
	enabled := change.Action == "enable"
	old, explicit := server["enabled"]
	previous, valid := old.(bool)
	if explicit && !valid {
		return plan, errors.New("Codex 用户配置的 enabled 值需要是布尔值")
	}
	if (!explicit && enabled) || (explicit && previous == enabled) {
		return plan, errors.New("用户默认值已是此设置")
	}
	next, err := setCodexMCPEnabled(before.Content, change.Name, enabled)
	if err != nil {
		return plan, err
	}
	if len(next) > 256*1024 {
		return plan, errors.New("Codex 用户配置超过 256 KiB")
	}
	server["enabled"] = enabled
	var actual map[string]any
	if err := toml.Unmarshal([]byte(next), &actual); err != nil || !reflect.DeepEqual(config, actual) {
		return plan, errors.New("无法单独修改此 MCP 的启停设置")
	}
	after := before
	after.Content = next
	plan.userDirectory = directory
	plan.before, plan.after = before, after
	plan.preview = CodexResourcePreview{
		Version: gitDigest([]any{directory, change, before, after}),
		Target:  filepath.Join(directory, "config.toml"),
		After:   "enabled = " + strconv.FormatBool(enabled),
	}
	if explicit {
		plan.preview.Before = "enabled = " + strconv.FormatBool(previous)
	} else {
		plan.preview.Before = "默认启用"
	}
	return plan, nil
}

// setCodexMCPEnabled edits the value's byte range, keeping comments and other settings intact.
func setCodexMCPEnabled(text, name string, enabled bool) (string, error) {
	var parser unstable.Parser
	parser.Reset([]byte(text))
	serverKey := []string{"mcp_servers", name}
	enabledKey := []string{"mcp_servers", name, "enabled"}
	value := strconv.FormatBool(enabled)
	var table []string
	tableEnd := -1
	firstTable := len(text)
	hasDottedEntry := false
	for parser.NextExpression() {
		expr := parser.Expression()
		switch expr.Kind {
		case unstable.Table, unstable.ArrayTable:
			keys := expr.Key()
			table = nil
			for keys.Next() {
				key := keys.Node()
				table = append(table, string(key.Data))
				offset := int(key.Raw.Offset)
				firstTable = min(firstTable, strings.LastIndex(text[:offset], "\n")+1)
				if expr.Kind == unstable.Table && slices.Equal(table, serverKey) && keys.IsLast() {
					tableEnd = offset + int(key.Raw.Length)
				}
			}
		case unstable.KeyValue:
			key := append(slices.Clone(table), codexTOMLKey(expr)...)
			node := expr.Value()
			if slices.Equal(key, enabledKey) && node.Kind == unstable.Bool {
				return replaceCodexBool(text, node, value), nil
			}
			if slices.Equal(key, serverKey) && node.Kind == unstable.InlineTable {
				return editCodexMCPInline(text, node, nil, value)
			}
			if slices.Equal(key, serverKey[:1]) && node.Kind == unstable.InlineTable {
				return editCodexMCPInline(text, node, []string{name}, value)
			}
			if len(table) == 0 && len(key) >= len(serverKey) && slices.Equal(key[:len(serverKey)], serverKey) {
				hasDottedEntry = true
			}
		}
	}
	if err := parser.Error(); err != nil {
		return "", errors.New("无法定位 Codex 用户配置中的 MCP")
	}
	if tableEnd >= 0 {
		if offset := strings.IndexByte(text[tableEnd:], '\n'); offset >= 0 {
			at := tableEnd + offset + 1
			eol := "\n"
			if at > 1 && text[at-2] == '\r' {
				eol = "\r\n"
			}
			return text[:at] + "enabled = " + value + eol + text[at:], nil
		}
		return text + "\nenabled = " + value + "\n", nil
	}
	if hasDottedEntry {
		eol := "\n"
		if strings.Contains(text, "\r\n") {
			eol = "\r\n"
		}
		prefix := ""
		if firstTable == len(text) && !strings.HasSuffix(text, "\n") {
			prefix = eol
		}
		return text[:firstTable] + prefix + "mcp_servers." + name + ".enabled = " + value + eol + text[firstTable:], nil
	}
	return "", errors.New("无法定位此 MCP 的启停设置")
}

func codexTOMLKey(node *unstable.Node) []string {
	var key []string
	for parts := node.Key(); parts.Next(); {
		key = append(key, string(parts.Node().Data))
	}
	return key
}

func replaceCodexBool(text string, node *unstable.Node, value string) string {
	at := int(node.Raw.Offset)
	return text[:at] + value + text[at+int(node.Raw.Length):]
}

func editCodexMCPInline(text string, node *unstable.Node, prefix []string, value string) (string, error) {
	wanted := append(slices.Clone(prefix), "enabled")
	for children := node.Children(); children.Next(); {
		child := children.Node()
		if child.Kind != unstable.KeyValue {
			continue
		}
		key := codexTOMLKey(child)
		if slices.Equal(key, wanted) && child.Value().Kind == unstable.Bool {
			return replaceCodexBool(text, child.Value(), value), nil
		}
		if len(prefix) > 0 && slices.Equal(key, prefix) && child.Value().Kind == unstable.InlineTable {
			return editCodexMCPInline(text, child.Value(), nil, value)
		}
	}
	at := int(node.Raw.Offset) + 1
	return text[:at] + strings.Join(wanted, ".") + " = " + value + ", " + text[at:], nil
}
