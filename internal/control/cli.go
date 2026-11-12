package control

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"time"

	"repsync/internal/workspace"
)

const help = `repsync — local project control (JSON output; open starts the desktop app)

  repsync open                             Open desktop app and wait for the control endpoint
  repsync checks PATH                       Discover project check configurations and commands
  repsync check [--wait] PATH CHECK_ID VERSION
                                            Run a discovered check using the project tools
  repsync doctor PATH                       Check project setup without running project commands
  repsync project-map PATH                  Read feature modules and locate their implementation references
  repsync ai-resources [PATH]                Inventory Skills, MCPs, CLIs, and project fit
  repsync ai-resource-record [PATH] FILE     Save AI update findings and manual from JSON
  repsync ai-resource-assess [PATH] FILE     Save candidate or installed-resource assessment from JSON
  repsync ai-usage [PATH]                    Read 30 days of local usage evidence and coverage
  repsync project-memory PATH               Read confirmed project memories and sources
  repsync project-memory-propose PATH FILE  Propose AI-inferred memories from JSON
  repsync codex-resources PATH              List project Skills and MCPs added by repsync
  repsync codex-resource-preview PATH FILE  Preview setup or removal from JSON
  repsync codex-resource-apply PATH VERSION FILE
  repsync review PATH                       Read current review state and version
  repsync review-check PATH VERSION FILE     Record an external check from JSON
  repsync review-evidence PATH VERSION FILE  Record stage or data verification from JSON
  repsync review-resources PATH VERSION FILE Save resources; VERSION is configVersion
  repsync review-profile PATH VERSION FILE   Save applicability; VERSION is configVersion
  repsync review-issue PATH VERSION FILE     Record keep, defer, or reopen decisions
  repsync review-task-context PATH VERSION FILE Record task and change scope
  repsync review-task-start PATH VERSION TITLE
  repsync review-task-finish PATH TASK_ID VERSION FILE
  repsync review-handover PATH TASK_ID        Read saved Markdown handoff
  repsync resources                         List shared file resources
  repsync resource-add ID SOURCE TARGET     Register a source and project-relative destination
  repsync resource-remove ID                Remove registration; preserve project files and history
  repsync resource-status PATH              Read adopted shared file status
  repsync resource-preview PATH ID          Preview shared file update; apply with sync-apply
  repsync resource-history PATH             Read shared file history; restore with sync-restore
  repsync projects                          List registered projects
  repsync add PATH                          Register folder without running project commands
  repsync remove PATH                       Remove registration; preserve folder and history
  repsync relocate OLD_PATH NEW_PATH        Update registration path without moving files
  repsync git-workspace PATH                Inspect worktrees, branches, cleanup candidates, and version
  repsync git-summary PATH                  Read branches and workspaces without cleanup inspection
  repsync git-policy PATH VERSION FILE     Save automatic cleanup settings from JSON; does not run cleanup
  repsync git-cleanup PATH VERSION          Clean up reviewed candidates with recovery records
  repsync git-restore PATH RECORD_ID         Restore cleaned local branches and worktrees
  repsync git-stashes PATH                  List repository-wide stashes
  repsync git-stash-show PATH OID            Preview stash changes and apply version
  repsync git-stash-apply PATH OID VERSION    Apply to clean worktree; preserve stash
  repsync status [PATH ...]                 Read services, ports, and run IDs
  repsync start [--dry-run | --wait] [--timeout 30s] PATH [PATH ...]
  repsync stop-projects PATH [PATH ...]      Stop owned services for selected projects
  repsync stop PATH SERVICE_ID RUN_ID        Stop specified service instance
  repsync logs PATH SERVICE_ID RUN_ID        Read specified service logs
  repsync pipelines PATH                    Discover local CI/CD entries and version
  repsync workflows PATH                    Read repository GitHub Actions definitions
  repsync run [--wait] [--timeout 30s] PATH PIPELINE_ID VERSION
  repsync runs PATH                         Read pipeline run history
  repsync pipeline-logs PATH RUN_ID          Read pipeline status and logs
  repsync pipeline-stop PATH RUN_ID          Stop specified pipeline
  repsync sync-source [FILE]                 Read or connect shared rules source
  repsync sync-status                        Read project adoption status
  repsync sync-preview PATH                  Preview shared rules update
  repsync sync-apply TOKEN                   Apply reviewed preview
  repsync sync-history PATH                  Read update and restore history
  repsync sync-restore RECORD_ID             Restore update if no later changes exist

With no arguments, opens the desktop app. Paths may be absolute or relative; operations require registered projects.
Repeated start reuses services with matching configuration. wait checks TCP readiness; undeclared ports yield running_unchecked.
Timeouts or interruptions preserve started tasks; follow the receipt. run requires VERSION from pipelines to reject stale configuration.
Exit codes: 0 success, 1 operation failed, 2 invalid arguments. REPSYNC_SOCKET selects the local control socket.
Development CLI builds use REPSYNC_APP for the absolute packaged executable path; packaged apps can open themselves.
`

func Call(ctx context.Context, socket string, request Request) (Response, error) {
	payload, err := json.Marshal(request)
	if err != nil {
		return Response{}, err
	}
	transport := &http.Transport{DialContext: func(ctx context.Context, _, _ string) (net.Conn, error) {
		return (&net.Dialer{}).DialContext(ctx, "unix", socket)
	}}
	defer transport.CloseIdleConnections()
	client := &http.Client{Transport: transport, Timeout: 30 * time.Second}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, "http://repsync/v1/control", bytes.NewReader(payload))
	if err != nil {
		return Response{}, err
	}
	req.Header.Set("Content-Type", "application/json")
	result, err := client.Do(req)
	if err != nil {
		return Response{}, fmt.Errorf("cannot connect to repsync; run repsync open or start the desktop app first: %w", err)
	}
	defer result.Body.Close()
	var response Response
	if err := json.NewDecoder(io.LimitReader(result.Body, 4*1024*1024)).Decode(&response); err != nil {
		return Response{}, err
	}
	return response, nil
}

func RunCLI(ctx context.Context, args []string, out, diagnostics io.Writer) int {
	if len(args) == 0 || args[0] == "help" || args[0] == "--help" || args[0] == "-h" {
		fmt.Fprint(out, help)
		return 0
	}
	flags := flag.NewFlagSet(args[0], flag.ContinueOnError)
	flags.SetOutput(diagnostics)
	dryRun, wait := false, false
	timeout := 30 * time.Second
	if args[0] == "start" {
		flags.BoolVar(&dryRun, "dry-run", false, "Preflight only; do not start")
	}
	if args[0] == "start" || args[0] == "run" || args[0] == "check" {
		flags.BoolVar(&wait, "wait", false, "Wait for port readiness or pipeline completion")
		flags.DurationVar(&timeout, "timeout", 30*time.Second, "Wait timeout")
	}
	if err := flags.Parse(args[1:]); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			fmt.Fprint(out, help)
			return 0
		}
		writeResponse(out, nil, err)
		return 2
	}
	paths := flags.Args()
	request := Request{Action: args[0], DryRun: dryRun}
	usage := func(text string) int { writeResponse(out, nil, errors.New(text)); return 2 }
	readInput := func(file string, value any) error {
		f, err := os.Open(file)
		if err != nil {
			return err
		}
		defer f.Close()
		decoder := json.NewDecoder(io.LimitReader(f, 128*1024+1))
		decoder.DisallowUnknownFields()
		if err := decoder.Decode(value); err != nil {
			return err
		}
		if err := decoder.Decode(new(any)); !errors.Is(err, io.EOF) {
			return errors.New("JSON file must contain exactly one value")
		}
		return nil
	}
	if (wait && dryRun) || timeout <= 0 || timeout > 2*time.Hour {
		return usage("dry-run cannot be combined with wait; timeout must be positive and at most 2h")
	}
	switch request.Action {
	case "git-policy":
		if len(paths) != 3 {
			return usage("Usage: repsync git-policy PATH POLICY_VERSION JSON_FILE")
		}
		request.Path, request.Version = paths[0], paths[1]
		request.GitPolicy = &workspace.GitCleanupPolicy{}
		if err := readInput(paths[2], request.GitPolicy); err != nil {
			return usage(err.Error())
		}
	case "project-memory-propose":
		if len(paths) != 2 {
			return usage("Usage: repsync project-memory-propose PATH JSON_FILE")
		}
		request.Path = paths[0]
		request.Memory = &workspace.ProjectMemoryInput{}
		if err := readInput(paths[1], request.Memory); err != nil {
			return usage(err.Error())
		}
	case "codex-resource-preview", "codex-resource-apply":
		expected := 2
		if request.Action == "codex-resource-apply" {
			expected = 3
		}
		if len(paths) != expected {
			if expected == 3 {
				return usage("Usage: repsync codex-resource-apply PATH VERSION JSON_FILE")
			}
			return usage("Usage: repsync codex-resource-preview PATH JSON_FILE")
		}
		request.Path = paths[0]
		if expected == 3 {
			request.Version = paths[1]
		}
		request.CodexResource = &workspace.CodexResourceChange{}
		if err := readInput(paths[expected-1], request.CodexResource); err != nil {
			return usage(err.Error())
		}
	case "ai-resource-record", "ai-resource-assess":
		if len(paths) < 1 || len(paths) > 2 {
			return usage("Usage: repsync " + request.Action + " [PATH] JSON_FILE")
		}
		if len(paths) == 2 {
			request.Path = paths[0]
		}
		var input any
		if request.Action == "ai-resource-assess" {
			request.AIResourceAssessment = &workspace.AIResourceAssessmentInput{}
			input = request.AIResourceAssessment
		} else {
			request.AIResourceNote = &workspace.AIResourceNoteInput{}
			input = request.AIResourceNote
		}
		if err := readInput(paths[len(paths)-1], input); err != nil {
			return usage(err.Error())
		}
	case "ai-resources", "ai-usage":
		if len(paths) > 1 {
			return usage("Usage: repsync " + request.Action + " [PATH]")
		}
		if len(paths) == 1 {
			request.Path = paths[0]
		}
	case "projects", "sync-status", "open", "resources":
		if len(paths) != 0 {
			return usage(request.Action + " does not accept path arguments")
		}
	case "add", "remove", "pipelines", "workflows", "runs", "sync-preview", "checks", "sync-history", "doctor", "resource-status", "resource-history", "git-workspace", "git-summary", "git-stashes", "review", "codex-resources", "project-memory", "project-map":
		if len(paths) != 1 {
			return usage("Usage: repsync " + request.Action + " PATH")
		}
		request.Path = paths[0]
	case "review-check", "review-evidence", "review-resources", "review-profile", "review-task-context", "review-issue":
		if len(paths) != 3 {
			return usage("Usage: repsync " + request.Action + " PATH VERSION JSON_FILE")
		}
		request.Path, request.Version = paths[0], paths[1]
		var input any
		if request.Action == "review-check" {
			request.ReviewCheck = &workspace.ReviewCheckInput{}
			input = request.ReviewCheck
		} else if request.Action == "review-evidence" {
			request.ReviewEvidence = &workspace.ReviewEvidenceInput{}
			input = request.ReviewEvidence
		} else if request.Action == "review-profile" {
			request.ReviewProfile = &workspace.ReviewProfile{}
			input = request.ReviewProfile
		} else if request.Action == "review-task-context" {
			request.ReviewTask = &workspace.ReviewTaskContext{}
			input = request.ReviewTask
		} else if request.Action == "review-issue" {
			request.ReviewIssue = &workspace.ReviewAcknowledgement{}
			input = request.ReviewIssue
		} else {
			values := []workspace.ReviewResource{}
			request.ReviewResources = &values
			input = request.ReviewResources
		}
		if err := readInput(paths[2], input); err != nil {
			return usage(err.Error())
		}
	case "review-task-start":
		if len(paths) != 3 {
			return usage("Usage: repsync review-task-start PATH VERSION TITLE")
		}
		request.Path, request.Version, request.Title = paths[0], paths[1], paths[2]
	case "review-task-finish":
		if len(paths) != 4 {
			return usage("Usage: repsync review-task-finish PATH TASK_ID VERSION JSON_FILE")
		}
		request.Path, request.RunID, request.Version = paths[0], paths[1], paths[2]
		request.ReviewFinish = &workspace.ReviewFinishInput{}
		if err := readInput(paths[3], request.ReviewFinish); err != nil {
			return usage(err.Error())
		}
	case "review-handover":
		if len(paths) != 2 {
			return usage("Usage: repsync review-handover PATH TASK_ID")
		}
		request.Path, request.RunID = paths[0], paths[1]
	case "git-stash-show":
		if len(paths) != 2 {
			return usage("Usage: repsync git-stash-show PATH OID")
		}
		request.Path, request.ObjectID = paths[0], paths[1]
	case "git-stash-apply":
		if len(paths) != 3 {
			return usage("Usage: repsync git-stash-apply PATH OID VERSION")
		}
		request.Path, request.ObjectID, request.Version = paths[0], paths[1], paths[2]
	case "git-cleanup", "git-restore":
		if len(paths) != 2 {
			return usage("Usage: repsync " + request.Action + " PATH VERSION_OR_RECORD_ID")
		}
		request.Path = paths[0]
		if request.Action == "git-cleanup" {
			request.Version = paths[1]
		} else {
			request.RunID = paths[1]
		}
	case "start", "stop-projects":
		if len(paths) == 0 {
			return usage("Specify project paths explicitly")
		}
		request.Paths = paths
	case "status":
		request.Paths = paths
	case "resource-add":
		if len(paths) != 3 {
			return usage("Usage: repsync resource-add ID SOURCE TARGET")
		}
		request.ResourceID, request.Path, request.Target = paths[0], paths[1], paths[2]
	case "resource-remove":
		if len(paths) != 1 {
			return usage("Usage: repsync resource-remove ID")
		}
		request.ResourceID = paths[0]
	case "resource-preview":
		if len(paths) != 2 {
			return usage("Usage: repsync resource-preview PATH ID")
		}
		request.Path, request.ResourceID = paths[0], paths[1]
	case "relocate":
		if len(paths) != 2 {
			return usage("Usage: repsync relocate OLD_PATH NEW_PATH")
		}
		request.Path, request.Destination = paths[0], paths[1]
	case "run", "check":
		if len(paths) != 3 {
			return usage("Usage: repsync run/check [--wait] PATH COMMAND_ID VERSION")
		}
		request.Path, request.ServiceID, request.Version = paths[0], paths[1], paths[2]
	case "pipeline-logs", "pipeline-stop":
		if len(paths) != 2 {
			return usage("Usage: repsync " + request.Action + " PATH RUN_ID")
		}
		request.Path, request.RunID = paths[0], paths[1]
	case "sync-apply", "sync-restore":
		if len(paths) != 1 {
			return usage("Specify a preview or record ID")
		}
		request.RunID = paths[0]
	case "sync-source":
		if len(paths) > 1 {
			return usage("Usage: repsync sync-source [FILE]")
		}
		if len(paths) == 1 {
			request.Path = paths[0]
		}
	case "stop", "logs":
		if len(paths) != 3 {
			return usage("Usage: repsync " + request.Action + " PATH SERVICE_ID RUN_ID")
		}
		request.Path, request.ServiceID, request.RunID = paths[0], paths[1], paths[2]
	default:
		return usage("Unknown command; use repsync --help")
	}
	canonical := func(path string) (string, error) {
		absolute, err := filepath.Abs(path)
		if err != nil {
			return "", err
		}
		if actual, err := filepath.EvalSymlinks(absolute); err == nil {
			return actual, nil
		}
		return absolute, nil
	}
	if request.Path != "" {
		path, err := canonical(request.Path)
		if err != nil {
			return usage(err.Error())
		}
		request.Path = path
	}
	if request.Destination != "" {
		actual, err := canonical(request.Destination)
		if err != nil {
			return usage(err.Error())
		}
		request.Destination = actual
	}
	for i, path := range request.Paths {
		actual, err := canonical(path)
		if err != nil {
			return usage(err.Error())
		}
		request.Paths[i] = actual
	}
	socket, err := SocketPath()
	if err != nil {
		writeResponse(out, nil, err)
		return 1
	}
	if request.Action == "open" {
		data, err := OpenDesktop(ctx, socket)
		writeResponse(out, data, err)
		if err != nil {
			return 1
		}
		return 0
	}
	response, err := Call(ctx, socket, request)
	if err != nil {
		writeResponse(out, nil, err)
		return 1
	}
	if wait && response.OK {
		response = waitForResult(ctx, socket, request, response, timeout)
	}
	if err := json.NewEncoder(out).Encode(response); err != nil {
		fmt.Fprintln(diagnostics, err)
		return 1
	}
	if !response.OK {
		return 1
	}
	return 0
}

// Run uses cancellation from the process entry point and leaves background tasks owned by the app.
func Run(ctx context.Context) int { return RunCLI(ctx, os.Args[1:], os.Stdout, os.Stderr) }

func waitForResult(ctx context.Context, socket string, request Request, initial Response, timeout time.Duration) Response {
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	poll := Request{}
	switch request.Action {
	case "start":
		var batch workspace.ServiceBatch
		if err := json.Unmarshal(initial.Data, &batch); err != nil {
			return Response{Error: err.Error()}
		}
		poll.Action = "readiness"
		for _, item := range batch.Services {
			poll.Runs = append(poll.Runs, workspace.ServiceRunRef{Path: item.Path, ServiceID: item.ServiceID, RunID: item.State.RunID})
		}
	case "run", "check":
		var run workspace.PipelineRun
		if err := json.Unmarshal(initial.Data, &run); err != nil {
			return Response{Error: err.Error()}
		}
		poll = Request{Action: "pipeline-logs", Path: request.Path, RunID: run.ID}
	}
	latest := initial
	ticker := time.NewTicker(150 * time.Millisecond)
	defer ticker.Stop()
	for {
		response, err := Call(ctx, socket, poll)
		if err != nil {
			latest.OK, latest.Error = false, err.Error()
			if ctx.Err() != nil {
				latest.Error = "Wait ended (" + ctx.Err().Error() + "); tasks preserved; inspect or stop them using the receipt"
			}
			return latest
		}
		latest = response
		if !response.OK {
			return response
		}
		complete := true
		if poll.Action == "readiness" {
			var batch workspace.ServiceBatch
			if err := json.Unmarshal(response.Data, &batch); err != nil {
				return Response{Error: err.Error()}
			}
			for _, item := range batch.Services {
				if item.Status == "pending" {
					complete = false
				}
			}
		} else {
			var output workspace.PipelineOutput
			if err := json.Unmarshal(response.Data, &output); err != nil {
				return Response{Error: err.Error()}
			}
			complete = output.Run.Status != "running"
			if complete && output.Run.Status != "success" {
				latest.OK, latest.Error = false, "Pipeline did not succeed; review results and logs"
			}
		}
		if complete {
			return latest
		}
		select {
		case <-ctx.Done():
			latest.OK, latest.Error = false, "Wait ended ("+ctx.Err().Error()+"); tasks preserved; inspect or stop them using the receipt"
			return latest
		case <-ticker.C:
		}
	}
}
