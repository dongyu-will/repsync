//go:build integration

package workspace_test

import (
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"repsync/internal/workspace"
)

func TestPackageRuntimeDiscoveryAndStartup(t *testing.T) {
	if _, err := exec.LookPath("node"); err != nil {
		t.Skip("Node is required")
	}
	root := t.TempDir()
	write := func(name, body string) {
		t.Helper()
		if err := os.MkdirAll(filepath.Dir(filepath.Join(root, name)), 0700); err != nil {
			t.Fatal(err)
		}
		writeSyncFile(t, filepath.Join(root, name), body)
	}
	db, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	dbPort, apiPort, webPort := db.Addr().(*net.TCPAddr).Port, availableServicePort(t), availableServicePort(t)
	write("package.json", `{"private":true,"workspaces":["clients/*"],"scripts":{
"dev":"node --env-file-if-exists=.env.local scripts/context.mjs npx concurrently --kill-others --names gateway,queue,view \"npm run serve:gateway\" \"npm run serve:queue\" \"npm run serve:view\"",
"serve:gateway":"cd backend && go run ./cmd/server",
"serve:queue":"node queue.mjs",
"serve:view":"npm run dev --workspace @orchard/view",
"infra:ready":"node scripts/infra.mjs",
"unrelated:dev":"node must-not-start.mjs"}}`)
	write("README.md", "# Development\n```sh\nnpm ci\nnpm run infra:ready\nnpm run dev\n```\n\nOptional `npm run unrelated:dev`.\n")
	write(".env.local", "DISCOVERY_SECRET=never-copy-this-value\n")
	write("scripts/context.mjs", `import {readFileSync} from 'node:fs';
import {spawn} from 'node:child_process';
const token=readFileSync(new URL('../runtime.env',import.meta.url),'utf8').trim();
const [command,...args]=process.argv.slice(2);
const child=spawn(command,args,{stdio:'inherit',env:{...process.env,BOOT_TOKEN:token}});
for(const signal of ['SIGINT','SIGTERM'])process.on(signal,()=>child.kill(signal));
child.on('exit',code=>process.exit(code??1));`)
	write("scripts/infra.mjs", `import {writeFileSync} from 'node:fs';
import {spawnSync} from 'node:child_process';
writeFileSync('runtime.env','prepared');
const result=spawnSync('docker',['compose','up','-d','--wait','store'],{stdio:'inherit'});
process.exit(result.status??1);`)
	write("compose.yaml", fmt.Sprintf("services:\n  store:\n    image: postgres:17\n    ports: ['127.0.0.1:%d:5432']\n  optional:\n    profiles: [alternate]\n    ports: ['61234:1234']\n", dbPort))
	// Substitute only Docker's CLI boundary. The dependency is a real TCP listener;
	// preparation, environment forwarding, npm, Go, Vite and stop behavior are real.
	write("bin/docker", "#!/bin/sh\ntest \"$1 $2 $3\" = 'compose up -d'\n")
	if err := os.Chmod(filepath.Join(root, "bin/docker"), 0700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", filepath.Join(root, "bin")+string(os.PathListSeparator)+os.Getenv("PATH"))
	write("backend/go.mod", "module fixture\n\ngo 1.27.3\n")
	write("backend/cmd/server/main.go", fmt.Sprintf(`package main
import("net/http";"os")
func main(){
 if os.Getenv("BOOT_TOKEN")!="prepared" { panic("environment wrapper was lost") }
 addr:=os.Getenv("ORCHARD_BIND")
 if addr=="" {addr="127.0.0.1:%d"}
 server:=http.Server{Addr:addr,Handler:http.HandlerFunc(func(w http.ResponseWriter,r *http.Request){w.Write([]byte("gateway"))})}
 server.ListenAndServe()
}`, apiPort))
	write("queue.mjs", "if(process.env.BOOT_TOKEN!=='prepared')throw Error('lost wrapper');setInterval(()=>{},1000);\n")
	write("clients/view/package.json", `{"name":"@orchard/view","private":true,"scripts":{"dev":"vite"}}`)
	write("clients/view/vite.config.mjs", fmt.Sprintf("export default {server:{host:'127.0.0.1',port:%d,strictPort:true}};\n", webPort))
	write("clients/view/index.html", "<h1>workspace-view</h1>")
	vite, _ := filepath.Abs("../../frontend/node_modules/vite/bin/vite.js")
	if _, err := os.Stat(vite); err != nil {
		t.Skip("Vite is required")
	}
	if err := os.MkdirAll(filepath.Join(root, "node_modules/.bin"), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(vite, filepath.Join(root, "node_modules/.bin/vite")); err != nil {
		t.Fatal(err)
	}
	s := workspace.NewService(filepath.Join(t.TempDir(), "projects.json"))
	t.Cleanup(s.Shutdown)
	project, err := s.Add(root)
	if err != nil {
		t.Fatal(err)
	}
	root = project.Path
	before := readSyncFile(t, filepath.Join(root, "package.json"))
	overview, err := s.LocalServices(root)
	if err != nil || overview.Error != "" || len(overview.Services) != 5 {
		t.Fatalf("discovery: %+v %v", overview, err)
	}
	ports := map[string]int{"serve:gateway": apiPort, "serve:view": webPort, "serve:queue": 0}
	for _, state := range overview.Services {
		d := state.Definition
		if expected, ok := ports[d.ID]; ok {
			if d.Port != expected || !slices.Contains(d.DependsOn, "infra:ready") || !slices.Contains(d.DependsOn, fmt.Sprintf("dependency-%d", dbPort)) || d.Command[0] != "node" {
				t.Fatalf("lost metadata or wrapper: %+v", d)
			}
		}
	}
	encoded, _ := json.Marshal(overview)
	if strings.Contains(string(encoded), "never-copy-this-value") {
		t.Fatal("environment secret leaked")
	}
	if _, err := s.StartProjects([]string{root}, true); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(root, "runtime.env")); !os.IsNotExist(err) {
		t.Fatal("dry run executed preparation")
	}
	batch, err := s.StartProjects([]string{root}, false)
	if err != nil {
		t.Fatalf("startup: %+v %v", batch, err)
	}
	for _, port := range []int{apiPort, webPort} {
		deadline := time.Now().Add(15 * time.Second)
		for {
			response, err := http.Get(fmt.Sprintf("http://127.0.0.1:%d", port))
			if err == nil {
				response.Body.Close()
				break
			}
			if time.Now().After(deadline) {
				t.Fatalf("port %d did not become ready: %v", port, err)
			}
			time.Sleep(50 * time.Millisecond)
		}
	}
	// Metadata sources participate in stale-plan and running-instance protection.
	write("scripts/context.mjs", readSyncFile(t, filepath.Join(root, "scripts/context.mjs"))+"\n// changed wrapper\n")
	if _, err := s.StartProject(root, overview.Version); err == nil {
		t.Fatal("stale runtime plan was accepted")
	}
	changed, _ := s.LocalServices(root)
	if !slices.ContainsFunc(changed.Services, func(state workspace.LocalServiceState) bool {
		return state.Definition.ID == "serve:gateway" && state.ConfigChanged
	}) {
		t.Fatal("running wrapper change was not reported")
	}
	if _, err := s.StopProjects([]string{root}); err != nil {
		t.Fatal(err)
	}
	write("bin/docker", "#!/bin/sh\nexit 7\n")
	if err := os.Chmod(filepath.Join(root, "bin/docker"), 0700); err != nil {
		t.Fatal(err)
	}
	failed, err := s.StartProjects([]string{root}, false)
	if err == nil || !slices.ContainsFunc(failed.Services, func(item workspace.ServiceStartResult) bool {
		return item.ServiceID == "serve:gateway" && item.Status == "not_started"
	}) {
		t.Fatalf("failed preparation did not block applications: %+v %v", failed, err)
	}
	if readSyncFile(t, filepath.Join(root, "package.json")) != before {
		t.Fatal("startup rewrote project scripts")
	}
	if _, err := os.Stat(filepath.Join(root, ".repsync.json")); !os.IsNotExist(err) {
		t.Fatal("discovery wrote an override")
	}
}

func TestPackageRuntimeParallelAliases(t *testing.T) {
	for _, test := range []struct{ name, command string }{
		{"run-p", "run-p service:*"},
		{"npm-run-all", "npm-run-all --parallel service:red service:blue"},
	} {
		t.Run(test.name, func(t *testing.T) {
			root := t.TempDir()
			command, _ := json.Marshal(test.command)
			writeSyncFile(t, filepath.Join(root, "package.json"), fmt.Sprintf(`{"scripts":{"dev":"npm run all","all":%s,"service:red":"node red.mjs --port 63421","service:blue":"node blue.mjs --port 63422","unrelated":"node extra.mjs"}}`, command))
			s := workspace.NewService(filepath.Join(t.TempDir(), "registry.json"))
			t.Cleanup(s.Shutdown)
			project, err := s.Add(root)
			if err != nil {
				t.Fatal(err)
			}
			overview, err := s.LocalServices(project.Path)
			if err != nil || overview.Error != "" || len(overview.Services) != 2 {
				t.Fatalf("parallel aliases: %+v %v", overview, err)
			}
			for _, service := range overview.Services {
				if service.Definition.Port == 0 || service.Definition.ID == "unrelated" {
					t.Fatalf("incorrect selected service: %+v", service)
				}
			}
		})
	}
}

func TestRuntimePreparationDiagnosis(t *testing.T) {
	s := workspace.NewService(filepath.Join(t.TempDir(), "registry.json"))
	t.Cleanup(s.Shutdown)
	root := batchProject(t, s, workspace.LocalServiceDefinition{ID: "prepare", Name: "Prepare", OneShot: true, Command: []string{"/usr/bin/true"}})
	writeLocalServices(t, root, []workspace.LocalServiceDefinition{
		{ID: "prepare", Name: "Prepare", OneShot: true, Command: []string{"/usr/bin/true"}},
		{ID: "store", Name: "Store", ObserveOnly: true, Port: availableServicePort(t), DependsOn: []string{"prepare"}},
	})
	report, err := s.DiagnoseProject(root)
	if err != nil || !report.Ready || !slices.ContainsFunc(report.Issues, func(issue workspace.DiagnosisIssue) bool {
		return issue.Kind == "dependency_pending_setup" && issue.Level == "warning"
	}) {
		t.Fatalf("pending preparation blocked diagnosis: %+v %v", report, err)
	}
	if !report.Services[0].OneShot || !slices.Equal(report.Services[1].DependsOn, []string{"prepare"}) {
		t.Fatal("preparation metadata omitted from diagnosis")
	}
	if _, err := s.StartProjects([]string{root}, true); err != nil {
		t.Fatal(err)
	}
}

func TestPackageRuntimeDynamicPortsStayUnknown(t *testing.T) {
	for _, test := range []struct{ name, config string }{
		{"environment", "export default {server:{port:process.env.DEV_PORT || 5173}}"},
		{"later spread", "export default {server:{port:64001,...settings}}"},
		{"quoted key", "export default {server:{'port':64001}}"},
		{"shorthand", "export default {server:{port}}"},
	} {
		t.Run(test.name, func(t *testing.T) {
			root := t.TempDir()
			if err := os.Mkdir(filepath.Join(root, "view"), 0700); err != nil {
				t.Fatal(err)
			}
			writeSyncFile(t, filepath.Join(root, "package.json"), `{"scripts":{"dev":"concurrently \"npm run gateway\" \"npm run view\"","gateway":"node gateway.mjs --port 64002","view":"npm --prefix view run dev"}}`)
			writeSyncFile(t, filepath.Join(root, "view/package.json"), `{"scripts":{"dev":"vite"}}`)
			writeSyncFile(t, filepath.Join(root, "view/vite.config.mjs"), test.config)
			s := workspace.NewService(filepath.Join(t.TempDir(), "registry.json"))
			t.Cleanup(s.Shutdown)
			project, err := s.Add(root)
			if err != nil {
				t.Fatal(err)
			}
			overview, err := s.LocalServices(project.Path)
			if err != nil || overview.Error != "" || len(overview.Services) != 2 {
				t.Fatalf("discovery: %+v %v", overview, err)
			}
			if overview.Services[1].Definition.Port != 0 {
				t.Fatalf("guessed a dynamic port: %+v", overview.Services[1])
			}
		})
	}
}

func TestPackageRuntimeOpaqueCommandsAndOverrides(t *testing.T) {
	for _, test := range []struct{ name, script, hooks string }{
		{"expansion", "concurrently \"npm run gateway-$MODE\" \"npm run view\"", ""},
		{"shell control", "echo prepare && concurrently \"npm run gateway\" \"npm run view\"", ""},
		{"unknown wrapper", "node custom.mjs concurrently \"npm run gateway\" \"npm run view\"", ""},
		{"aggregate hook", "concurrently \"npm run gateway\" \"npm run view\"", `,"predev":"node prepare.mjs"`},
	} {
		t.Run(test.name, func(t *testing.T) {
			s := workspace.NewService(filepath.Join(t.TempDir(), "registry.json"))
			t.Cleanup(s.Shutdown)
			root := t.TempDir()
			script, _ := json.Marshal(test.script)
			writeSyncFile(t, filepath.Join(root, "package.json"), fmt.Sprintf(`{"scripts":{"dev":%s,"gateway":"node gateway.mjs","view":"node view.mjs"%s}}`, script, test.hooks))
			project, err := s.Add(root)
			if err != nil {
				t.Fatal(err)
			}
			root = project.Path
			overview, err := s.LocalServices(root)
			if err != nil || overview.Error != "" || len(overview.Services) != 1 || overview.Services[0].Definition.ID != "dev" || len(overview.Warnings) != 1 {
				t.Fatalf("opaque command was not retained: %+v %v", overview, err)
			}
			writeLocalServices(t, root, []workspace.LocalServiceDefinition{})
			overview, err = s.LocalServices(root)
			if err != nil || overview.Error != "" || len(overview.Services) != 0 {
				t.Fatalf("explicit empty override lost: %+v %v", overview, err)
			}
		})
	}
}
