package workspace

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"

	"go.yaml.in/yaml/v3"
)

const opaqueStartupWarning = "Some startup commands could not be expanded; the original command is retained."

type runtimePackage struct {
	Name           string            `json:"name"`
	PackageManager string            `json:"packageManager"`
	Scripts        map[string]string `json:"scripts"`
	Workspaces     json.RawMessage   `json:"workspaces"`
}

// Runtime discovery is a bounded static reader, never a shell/JS evaluator.
type packageRuntime struct {
	path, manager string
	root          *os.Root
	input         []byte
	files         map[string][]byte
	err           error
	packages      map[string]runtimePackage
	manifestNames []string
	env           map[string]string
}

func (r *packageRuntime) read(name string) []byte {
	name = filepath.ToSlash(filepath.Clean(name))
	if body, ok := r.files[name]; ok {
		return body
	}
	if len(r.files) >= 128 {
		r.err = errors.New("runtime discovery exceeds 128 input files")
		return nil
	}
	body, err := readProjectConfig(r.root, name)
	if err != nil && !errors.Is(err, os.ErrNotExist) && r.err == nil {
		r.err = fmt.Errorf("read runtime input %s: %w", name, err)
	}
	r.files[name] = body
	// Only fingerprints enter the discovery input. Environment secrets are never
	// copied into service definitions, diagnostics, or configuration records.
	r.input = append(r.input, []byte(name+"\x00"+digest(string(body))+"\n")...)
	return body
}

func (r *packageRuntime) pkg(dir string) runtimePackage {
	if pkg, ok := r.packages[dir]; ok {
		return pkg
	}
	var pkg runtimePackage
	if body := r.read(filepath.Join(dir, "package.json")); body != nil {
		if err := json.Unmarshal(body, &pkg); err != nil {
			r.err = fmt.Errorf("read %s: invalid package JSON", filepath.Join(dir, "package.json"))
		}
	}
	r.packages[dir] = pkg
	return pkg
}

// shellWords accepts literal argv, quotes and &&. Expansion, pipes, redirection,
// background jobs and shell control flow stay opaque instead of being rewritten.
func shellWords(text string) ([]string, bool) {
	var result []string
	var word strings.Builder
	quote := rune(0)
	started, escaped := false, false
	flush := func() {
		if started {
			result = append(result, word.String())
			word.Reset()
			started = false
		}
	}
	chars := []rune(text)
	for i, c := range chars {
		if escaped {
			word.WriteRune(c)
			escaped = false
			continue
		}
		if c == '\\' && quote != '\'' {
			escaped, started = true, true
			continue
		}
		if quote != 0 {
			if c == quote {
				quote = 0
			} else {
				if quote == '"' && (c == '$' || c == '`') {
					return nil, false
				}
				word.WriteRune(c)
			}
			continue
		}
		switch c {
		case '\'', '"':
			quote, started = c, true
		case ' ', '\t':
			flush()
		case '&':
			if (i+1 < len(chars) && chars[i+1] == '&') || (i > 0 && chars[i-1] == '&') {
				flush()
				if i > 0 && chars[i-1] == '&' {
					result = append(result, "&&")
				}
			} else {
				return nil, false
			}
		case '$', '`', ';', '|', '<', '>', '\n', '\r', '(', ')', '#':
			return nil, false
		default:
			word.WriteRune(c)
			started = true
		}
	}
	flush()
	return result, quote == 0 && !escaped
}

// packageRun resolves an existing script reference, including npm workspaces and
// prefix/working-directory selectors. It does not discover unrelated dev tasks.
func (r *packageRuntime) packageRun(args []string, dir string) (string, string, bool) {
	if len(args) < 2 || !slices.Contains([]string{"npm", "pnpm", "yarn", "bun"}, filepath.Base(args[0])) {
		return "", "", false
	}
	name, workspace := "", ""
	for i := 1; i < len(args); i++ {
		arg := args[i]
		if arg == "run" {
			continue
		}
		if arg == "--" {
			break
		}
		key, value, equal := strings.Cut(arg, "=")
		if slices.Contains([]string{"--workspace", "-w", "--filter", "--prefix", "--dir", "-C"}, key) {
			if !equal {
				i++
				if i >= len(args) {
					return "", "", false
				}
				value = args[i]
			}
			if slices.Contains([]string{"--workspace", "-w", "--filter"}, key) {
				workspace = value
			} else {
				dir = filepath.Join(dir, value)
			}
		} else if strings.HasPrefix(arg, "-") || name != "" {
			return "", "", false
		} else {
			name = arg
		}
	}
	if !filepath.IsLocal(filepath.Join(dir, "package.json")) {
		return "", "", false
	}
	if workspace != "" {
		if r.manifestNames == nil {
			names, err := projectCheckFiles(r.path, r.root)
			if err != nil {
				r.err = err
				return "", "", false
			}
			r.manifestNames = names
		}
		found := ""
		for _, file := range r.manifestNames {
			if filepath.Base(file) != "package.json" {
				continue
			}
			candidate := filepath.Dir(file)
			if candidate == "." {
				candidate = ""
			}
			var patterns []string
			workspaceConfig := r.pkg("").Workspaces
			if len(workspaceConfig) > 0 && json.Unmarshal(workspaceConfig, &patterns) != nil {
				var object struct {
					Packages []string `json:"packages"`
				}
				_ = json.Unmarshal(workspaceConfig, &object)
				patterns = object.Packages
			}
			if len(workspaceConfig) == 0 {
				var config struct {
					Packages []string `yaml:"packages"`
				}
				if body := r.read("pnpm-workspace.yaml"); body != nil {
					if err := yaml.Unmarshal(body, &config); err != nil {
						r.err = errors.New("invalid pnpm workspace YAML")
					}
					patterns = config.Packages
				}
			}
			if !slices.ContainsFunc(patterns, func(pattern string) bool { match, _ := filepath.Match(pattern, candidate); return match }) {
				continue
			}
			pkg := r.pkg(candidate)
			if pkg.Name == workspace || candidate == filepath.Clean(workspace) {
				if found != "" {
					return "", "", false
				}
				found = candidate
			}
		}
		if found == "" {
			return "", "", false
		}
		dir = found
	}
	_, ok := r.pkg(dir).Scripts[name]
	return dir, name, ok
}

var argvForward = regexp.MustCompile(`(?:const|let)\s*\[\s*([A-Za-z_$][\w$]*)\s*,\s*\.\.\.([A-Za-z_$][\w$]*)\s*\]\s*=\s*process\.argv\.slice\(\s*2\s*\)`)
var mutatingWrapper = regexp.MustCompile(`\b(?:writeFile|appendFile|mkdir|unlink|rm|exec|execSync|spawnSync)\s*\(`)

func (r *packageRuntime) wrapper(prefix []string) bool {
	if len(prefix) == 0 {
		return true
	}
	if prefix[0] != "node" {
		return false
	}
	for i := 1; i < len(prefix); i++ {
		if strings.HasPrefix(prefix[i], "--env-file=") || strings.HasPrefix(prefix[i], "--env-file-if-exists=") {
			_, file, _ := strings.Cut(prefix[i], "=")
			r.loadEnv(file)
			continue
		}
		if i != len(prefix)-1 || !slices.Contains([]string{".js", ".mjs", ".cjs"}, filepath.Ext(prefix[i])) {
			return false
		}
		body := string(r.read(prefix[i]))
		match := argvForward.FindStringSubmatch(body)
		if len(match) == 0 || mutatingWrapper.MatchString(body) {
			return false
		}
		forward := regexp.MustCompile(`\bspawn\(\s*` + regexp.QuoteMeta(match[1]) + `\s*,\s*` + regexp.QuoteMeta(match[2]) + `\s*[,)]`)
		return forward.MatchString(body) && strings.Count(body, "spawn(") == 1
	}
	return false
}

func (r *packageRuntime) concurrentChildren(script string) ([]string, [][]string, bool) {
	args, ok := shellWords(script)
	if !ok {
		return nil, nil, false
	}
	parallel := slices.Index(args, "run-p")
	if parallel < 0 {
		parallel = slices.Index(args, "npm-run-all")
	}
	if parallel >= 0 {
		prefix := slices.Clone(args[:parallel])
		if len(prefix) > 0 && prefix[len(prefix)-1] == "npx" {
			prefix = prefix[:len(prefix)-1]
		}
		if !r.wrapper(prefix) {
			return nil, nil, false
		}
		isParallel := args[parallel] == "run-p"
		children := [][]string{}
		for _, pattern := range args[parallel+1:] {
			if pattern == "--parallel" || pattern == "-p" {
				isParallel = true
				continue
			}
			if strings.HasPrefix(pattern, "-") {
				return nil, nil, false
			}
			names := []string{}
			for name := range r.pkg("").Scripts {
				if match, _ := filepath.Match(pattern, name); match {
					names = append(names, name)
				}
			}
			slices.Sort(names)
			if len(names) == 0 {
				return nil, nil, false
			}
			for _, name := range names {
				children = append(children, []string{r.manager, "run", name})
			}
		}
		return prefix, children, isParallel && len(children) > 1 && len(children) <= 8
	}
	index := slices.Index(args, "concurrently")
	if index < 0 {
		return nil, nil, false
	}
	prefix := slices.Clone(args[:index])
	if len(prefix) > 0 && prefix[len(prefix)-1] == "npx" {
		prefix = prefix[:len(prefix)-1]
	}
	if !r.wrapper(prefix) {
		return nil, nil, false
	}
	children := [][]string{}
	for i := index + 1; i < len(args); i++ {
		arg := args[i]
		if slices.Contains([]string{"--kill-others", "--kill-others-on-fail", "--raw", "--success=all", "--success=first"}, arg) {
			continue
		}
		if slices.Contains([]string{"--names", "-n", "--prefix-colors", "-c", "--prefix", "-p"}, arg) {
			i++
			if i >= len(args) {
				return nil, nil, false
			}
			continue
		}
		if strings.HasPrefix(arg, "-") {
			return nil, nil, false
		}
		child, ok := shellWords(arg)
		if !ok || slices.Contains(child, "&&") {
			return nil, nil, false
		}
		if _, _, ok := r.packageRun(child, ""); !ok {
			return nil, nil, false
		}
		children = append(children, child)
	}
	return prefix, children, len(children) > 1 && len(children) <= 8
}

func discoverPackageServices(path string, root *os.Root) ([]LocalServiceDefinition, []byte, []string, error) {
	r := &packageRuntime{path: path, root: root, files: map[string][]byte{}, packages: map[string]runtimePackage{}, env: map[string]string{}}
	pkg := r.pkg("")
	manager, err := projectPackageManager(root, pkg.PackageManager)
	if err != nil {
		return nil, nil, nil, err
	}
	r.manager = manager
	name := ""
	for _, candidate := range []string{"dev", "start"} {
		if _, ok := pkg.Scripts[candidate]; ok {
			name = candidate
			break
		}
	}
	if name == "" {
		return nil, r.input, nil, r.err
	}
	services := []LocalServiceDefinition{}
	warnings := []string{}
	script, alias := pkg.Scripts[name], name
	for range 12 {
		words, ok := shellWords(script)
		if !ok || pkg.Scripts["pre"+alias] != "" || pkg.Scripts["post"+alias] != "" {
			break
		}
		dir, next, ok := r.packageRun(words, "")
		if !ok || dir != "" {
			break
		}
		alias, script = next, pkg.Scripts[next]
	}
	prefix, children, expanded := r.concurrentChildren(script)
	if pkg.Scripts["pre"+name] != "" || pkg.Scripts["post"+name] != "" || pkg.Scripts["pre"+alias] != "" || pkg.Scripts["post"+alias] != "" {
		expanded = false // Keep aggregate lifecycle hooks under their original owner.
	}
	if !expanded {
		children = [][]string{{manager, "run", name}}
		if strings.Contains(script, "concurrently") || strings.Contains(script, "run-p") || strings.Contains(script, "npm-run-all") {
			warnings = append(warnings, opaqueStartupWarning)
		}
	}
	ids := map[string]bool{}
	for _, child := range children {
		_, id, _ := r.packageRun(child, "")
		if ids[id] {
			return nil, nil, nil, errors.New("duplicate startup script references require an explicit services override")
		}
		ids[id] = true
		command := slices.Concat(prefix, child)
		if !expanded {
			command = child
		}
		d := LocalServiceDefinition{ID: id, Name: id, Command: command}
		if expanded {
			d.Port = r.scriptPort(child, "", nil, 0)
			d.FixedPort = d.Port > 0
			if d.Port > 0 {
				d.URL = fmt.Sprintf("http://127.0.0.1:%d", d.Port)
			}
		} else {
			d.Port = r.scriptPort(child, "", nil, 0)
		}
		services = append(services, d)
	}
	setup, observed := r.preparations(name)
	dependencies := []string{}
	for _, d := range setup {
		dependencies = append(dependencies, d.ID)
	}
	for i := range observed {
		observed[i].DependsOn = slices.Clone(dependencies)
	}
	for _, d := range observed {
		dependencies = append(dependencies, d.ID)
	}
	for i := range services {
		services[i].DependsOn = slices.Clone(dependencies)
	}
	services = slices.Concat(setup, observed, services)
	for i := range services {
		if !services[i].ObserveOnly {
			services[i].scriptVersion = digest(string(r.input))
		}
	}
	return services, r.input, warnings, r.err
}
