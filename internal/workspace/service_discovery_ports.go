package workspace

import (
	"go/ast"
	"go/parser"
	"go/token"
	"maps"
	"net"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
)

func parseRuntimePort(value string) (int, bool) {
	n, err := strconv.Atoi(value)
	return n, err == nil && n > 0 && n <= 65535
}

func runtimeAddressPort(value string) int {
	host, text, err := net.SplitHostPort(value)
	if err != nil || !runtimeLocalHost(host) {
		return 0
	}
	n, ok := parseRuntimePort(text)
	if !ok {
		return 0
	}
	return n
}

func (r *packageRuntime) loadEnv(name string) {
	for _, line := range strings.Split(string(r.read(name)), "\n") {
		key, value, ok := strings.Cut(strings.TrimPrefix(strings.TrimSpace(line), "export "), "=")
		key, value = strings.TrimSpace(key), strings.TrimSpace(value)
		if !ok || key == "" || strings.HasPrefix(key, "#") || strings.ContainsAny(value, "$`\n\r") {
			continue
		}
		r.env[key] = strings.Trim(value, "\"'")
	}
}

func (r *packageRuntime) envValue(key string, inline map[string]string) string {
	if value, ok := inline[key]; ok {
		return value
	}
	if value, ok := os.LookupEnv(key); ok {
		return value
	}
	return r.env[key]
}

func (r *packageRuntime) scriptPort(args []string, dir string, env map[string]string, depth int) int {
	if depth >= 12 || len(args) == 0 {
		return 0
	}
	env = maps.Clone(env)
	if env == nil {
		env = map[string]string{}
	}
	if args[0] == "env" || args[0] == "cross-env" {
		args = args[1:]
	}
	for len(args) > 0 {
		key, value, ok := strings.Cut(args[0], "=")
		if !ok || strings.HasPrefix(key, "-") {
			break
		}
		env[key], args = value, args[1:]
	}
	if len(args) >= 4 && args[0] == "cd" && args[2] == "&&" {
		dir, args = filepath.Join(dir, args[1]), args[3:]
	}
	if len(args) == 0 || slices.Contains(args, "&&") || !filepath.IsLocal(filepath.Join(dir, "package.json")) {
		return 0
	}
	if childDir, name, ok := r.packageRun(args, dir); ok {
		words, ok := shellWords(r.pkg(childDir).Scripts[name])
		if !ok {
			return 0
		}
		return r.scriptPort(words, childDir, env, depth+1)
	}
	for i, arg := range args {
		value, ok := strings.CutPrefix(arg, "--port=")
		if arg == "--port" && i+1 < len(args) {
			value, ok = args[i+1], true
		}
		if ok {
			if n, valid := parseRuntimePort(value); valid {
				return n
			}
			return 0
		}
	}
	if args[0] == "vite" && (len(args) == 1 || strings.HasPrefix(args[1], "--")) {
		return r.vitePort(dir)
	}
	if len(args) == 3 && args[0] == "go" && args[1] == "run" && strings.HasPrefix(args[2], ".") {
		return r.goPort(filepath.Join(dir, args[2]), env)
	}
	return 0
}

// literalJSObjectNumber reads only a direct numeric property of an object. It
// skips strings/comments and nested objects; it never executes a Vite config.
func literalJSObjectNumber(text, object, property string) (int, bool) {
	quote, escaped, depth := byte(0), false, 0
	active, found := false, false
	port, hasPort := 0, false
	for i := 0; i < len(text); i++ {
		c := text[i]
		if escaped {
			escaped = false
			continue
		}
		if quote != 0 {
			if c == '\\' {
				escaped = true
			} else if c == quote {
				quote = 0
			}
			continue
		}
		if c == '/' && i+1 < len(text) {
			if text[i+1] == '/' {
				if end := strings.IndexByte(text[i+2:], '\n'); end >= 0 {
					i += end + 2
					continue
				}
				break
			}
			if text[i+1] == '*' {
				if end := strings.Index(text[i+2:], "*/"); end >= 0 {
					i += end + 3
					continue
				}
				break
			}
		}
		if c == '\'' || c == '"' || c == '`' {
			name := object
			if active {
				name = property
			}
			if (!active || depth == 1) && strings.HasPrefix(text[i+1:], name+string(c)) {
				return 0, true // Quoted keys are outside this literal-property reader.
			}
			quote = c
			continue
		}
		word := object
		if active {
			word = property
		}
		if (!active || depth == 1) && strings.HasPrefix(text[i:], word) && (i == 0 || !identifierByte(text[i-1])) {
			rest := strings.TrimSpace(text[i+len(word):])
			if strings.HasPrefix(rest, ":") {
				rest = strings.TrimSpace(rest[1:])
				if !active {
					if !strings.HasPrefix(rest, "{") {
						return 0, true
					}
					active, found = true, true
					i += len(word) + strings.IndexByte(text[i+len(word):], '{') - 1
					continue
				}
				end := 0
				for end < len(rest) && rest[end] >= '0' && rest[end] <= '9' {
					end++
				}
				if end == 0 || (end < len(rest) && !strings.ContainsRune(",}\n\r \t", rune(rest[end]))) {
					return 0, true
				}
				if hasPort {
					return 0, true
				}
				port, _ = parseRuntimePort(rest[:end])
				hasPort = true
			} else if active && (strings.HasPrefix(rest, ",") || strings.HasPrefix(rest, "}")) {
				return 0, true // A shorthand property is dynamic.
			}
		}
		if active {
			if c == '{' {
				depth++
			} else if c == '}' {
				depth--
				if depth == 0 {
					if hasPort {
						return port, true
					}
					return 5173, true // Vite's documented default, with no port override.
				}
			} else if depth == 1 && strings.HasPrefix(text[i:], "...") {
				return 0, true
			}
		}
	}
	return 0, found
}

func identifierByte(c byte) bool {
	return c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' || c >= '0' && c <= '9' || c == '_' || c == '$'
}

func (r *packageRuntime) vitePort(dir string) int {
	for _, ext := range []string{"ts", "js", "mts", "mjs", "cts", "cjs"} {
		body := r.read(filepath.Join(dir, "vite.config."+ext))
		if body != nil {
			if port, found := literalJSObjectNumber(string(body), "server", "port"); found {
				return port
			}
			if !strings.Contains(string(body), "defineConfig({") {
				return 0
			}
		}
	}
	return 5173
}

func goString(expr ast.Expr) string {
	if literal, ok := expr.(*ast.BasicLit); ok && literal.Kind == token.STRING {
		value, _ := strconv.Unquote(literal.Value)
		return value
	}
	return ""
}

func (r *packageRuntime) goPort(dir string, env map[string]string) int {
	folder, err := r.root.Open(dir)
	if err != nil {
		return 0
	}
	defer folder.Close()
	entries, err := folder.ReadDir(65)
	if err != nil || len(entries) > 64 {
		return 0
	}
	slices.SortFunc(entries, func(a, b os.DirEntry) int { return strings.Compare(a.Name(), b.Name()) })
	ports := map[int]bool{}
	for _, entry := range entries {
		if !strings.HasSuffix(entry.Name(), ".go") || strings.HasSuffix(entry.Name(), "_test.go") {
			continue
		}
		file, err := parser.ParseFile(token.NewFileSet(), entry.Name(), r.read(filepath.Join(dir, entry.Name())), 0)
		if err != nil {
			continue
		}
		httpPackage := ""
		for _, imported := range file.Imports {
			if goString(imported.Path) == "net/http" {
				httpPackage = "http"
				if imported.Name != nil {
					httpPackage = imported.Name.Name
				}
			}
		}
		for _, decl := range file.Decls {
			fn, ok := decl.(*ast.FuncDecl)
			if !ok || fn.Name.Name != "main" || fn.Body == nil {
				continue
			}
			values, keys := map[string]string{}, map[string]string{}
			for _, stmt := range fn.Body.List {
				if assign, ok := stmt.(*ast.AssignStmt); ok && len(assign.Lhs) == 1 && len(assign.Rhs) == 1 {
					id, ok := assign.Lhs[0].(*ast.Ident)
					if !ok {
						continue
					}
					values[id.Name] = goString(assign.Rhs[0])
					if call, ok := assign.Rhs[0].(*ast.CallExpr); ok && len(call.Args) == 1 {
						if sel, ok := call.Fun.(*ast.SelectorExpr); ok && sel.Sel.Name == "Getenv" {
							if pkg, ok := sel.X.(*ast.Ident); ok && pkg.Name == "os" {
								keys[id.Name] = goString(call.Args[0])
								values[id.Name] = r.envValue(keys[id.Name], env)
							}
						}
					}
				}
				if branch, ok := stmt.(*ast.IfStmt); ok && len(branch.Body.List) == 1 {
					cond, ok := branch.Cond.(*ast.BinaryExpr)
					literal, empty := condString(cond)
					if !ok || cond.Op != token.EQL || !empty || literal != "" || branch.Else != nil {
						continue
					}
					id, ok := cond.X.(*ast.Ident)
					assign, assigned := branch.Body.List[0].(*ast.AssignStmt)
					if !ok || !assigned || keys[id.Name] == "" || values[id.Name] != "" || len(assign.Lhs) != 1 || len(assign.Rhs) != 1 {
						continue
					}
					if target, ok := assign.Lhs[0].(*ast.Ident); ok && target.Name == id.Name {
						values[id.Name] = goString(assign.Rhs[0])
					}
				}
			}
			ast.Inspect(fn.Body, func(node ast.Node) bool {
				object, ok := node.(*ast.CompositeLit)
				if !ok {
					return true
				}
				typ, ok := object.Type.(*ast.SelectorExpr)
				if !ok || typ.Sel.Name != "Server" || httpPackage == "" {
					return true
				}
				pkg, ok := typ.X.(*ast.Ident)
				if !ok || pkg.Name != httpPackage {
					return true
				}
				for _, element := range object.Elts {
					field, ok := element.(*ast.KeyValueExpr)
					if !ok {
						continue
					}
					key, ok := field.Key.(*ast.Ident)
					if !ok || key.Name != "Addr" {
						continue
					}
					value := goString(field.Value)
					if id, ok := field.Value.(*ast.Ident); ok {
						value = values[id.Name]
					}
					if port := runtimeAddressPort(value); port > 0 {
						ports[port] = true
					}
				}
				return true
			})
		}
	}
	if len(ports) == 1 {
		for port := range ports {
			return port
		}
	}
	return 0
}

func condString(condition *ast.BinaryExpr) (string, bool) {
	if condition == nil {
		return "", false
	}
	literal, ok := condition.Y.(*ast.BasicLit)
	if !ok || literal.Kind != token.STRING {
		return "", false
	}
	return goString(literal), true
}
