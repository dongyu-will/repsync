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

const help = `repsync — AI 项目控制（JSON 输出；open 启动桌面控制端）

  repsync open                             打开桌面应用并等待控制入口
  repsync check-rules PATH                  检查项目规则，可独立运行
  repsync doctor PATH                       检查项目接入，不执行项目命令
  repsync ai-resources [PATH]                盘点 Skill、MCP、CLI 与项目适配
  repsync ai-usage [PATH]                    读取最近 30 天本地使用证据与覆盖范围
  repsync review PATH                       手动读取收尾状态及版本
  repsync review-check PATH VERSION FILE     从 JSON 文件记录外部检查
  repsync review-evidence PATH VERSION FILE  从 JSON 文件记录阶段或数据核验
  repsync review-resources PATH VERSION FILE 保存检查对象数组，VERSION 来自 configVersion
  repsync review-task-start PATH VERSION TITLE
  repsync review-task-finish PATH TASK_ID VERSION FILE
  repsync review-handover PATH TASK_ID        读取已保存的 Markdown 交接
  repsync resources                         列出共享文件资源
  repsync resource-add ID SOURCE TARGET     登记来源及项目内目标相对路径
  repsync resource-remove ID                移除资源登记，保留项目文件和历史
  repsync resource-status PATH              查看项目采用的共享文件状态
  repsync resource-preview PATH ID          预览共享文件更新，用 sync-apply 应用
  repsync resource-history PATH             查看共享文件历史，用 sync-restore 恢复
  repsync projects                          列出已登记项目
  repsync add PATH                          登记目录，不执行项目命令
  repsync remove PATH                       移除登记，保留目录和历史
  repsync relocate OLD_PATH NEW_PATH        更换登记路径，不搬动文件
  repsync git-workspace PATH                查看工作树、分支和清理候选及版本
  repsync git-cleanup PATH VERSION          清理已审查的候选，保留恢复记录
  repsync git-restore PATH RECORD_ID         恢复清理的本地分支及工作树
  repsync git-stashes PATH                  查看仓库共享的 stash
  repsync git-stash-show PATH OID            查看 stash 改动及恢复版本
  repsync git-stash-apply PATH OID VERSION    恢复到当前干净工作树，保留 stash
  repsync status [PATH ...]                 查看服务、端口及运行编号
  repsync start [--dry-run | --wait] [--timeout 30s] PATH [PATH ...]
  repsync stop-projects PATH [PATH ...]      批量停止选定项目的所属服务
  repsync stop PATH SERVICE_ID RUN_ID        停止指定实例
  repsync logs PATH SERVICE_ID RUN_ID        读取指定实例日志
  repsync pipelines PATH                    查看流水线定义及版本
  repsync run [--wait] [--timeout 30s] PATH PIPELINE_ID VERSION
  repsync runs PATH                         查看流水线运行记录
  repsync pipeline-logs PATH RUN_ID          查看流水线状态与日志
  repsync pipeline-stop PATH RUN_ID          停止指定流水线
  repsync sync-source [FILE]                 查看或接入公共来源
  repsync sync-status                        查看各项目采用状态
  repsync sync-preview PATH                  预览公共规范更新
  repsync policy-preview PATH                预览派生的 AI 规则说明
  repsync sync-apply TOKEN                   应用已审查的预览
  repsync sync-history PATH                  查看更新与恢复记录
  repsync sync-restore RECORD_ID             恢复未产生后续改动的更新

不传参数时打开桌面应用。路径可用绝对或相对路径；操作只接受已登记项目。
start 可重复调用，复用配置一致的服务。wait 检查 TCP 就绪，未声明端口会标为 running_unchecked。
等待超时或中断保留已启动任务，按回执处理；run 的 VERSION 来自 pipelines，防止执行过期配置。
退出码：0 成功，1 操作失败，2 参数错误。REPSYNC_SOCKET 指定本机控制 socket。
开发 CLI 使用 REPSYNC_APP 指定打包桌面程序的绝对路径；打包程序可自行 open。
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
		return Response{}, fmt.Errorf("无法连接 repsync，请先执行 repsync open 或启动桌面应用: %w", err)
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
		flags.BoolVar(&dryRun, "dry-run", false, "只预检，不启动")
	}
	if args[0] == "start" || args[0] == "run" {
		flags.BoolVar(&wait, "wait", false, "等待端口就绪或流水线结束")
		flags.DurationVar(&timeout, "timeout", 30*time.Second, "等待时限")
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
			return errors.New("JSON 文件只能包含一个值")
		}
		return nil
	}
	if (wait && dryRun) || timeout <= 0 || timeout > 2*time.Hour {
		return usage("dry-run 不能与 wait 同用；timeout 必须大于 0 且不超过 2h")
	}
	switch request.Action {
	case "ai-resources", "ai-usage":
		if len(paths) > 1 {
			return usage("用法: repsync " + request.Action + " [PATH]")
		}
		if len(paths) == 1 {
			request.Path = paths[0]
		}
	case "projects", "sync-status", "open", "resources":
		if len(paths) != 0 {
			return usage(request.Action + " 不接受路径参数")
		}
	case "add", "remove", "pipelines", "runs", "sync-preview", "policy-preview", "sync-history", "doctor", "resource-status", "resource-history", "git-workspace", "git-stashes", "review":
		if len(paths) != 1 {
			return usage("用法: repsync " + request.Action + " PATH")
		}
		request.Path = paths[0]
	case "review-check", "review-evidence", "review-resources":
		if len(paths) != 3 {
			return usage("用法: repsync " + request.Action + " PATH VERSION JSON_FILE")
		}
		request.Path, request.Version = paths[0], paths[1]
		var input any
		if request.Action == "review-check" {
			request.ReviewCheck = &workspace.ReviewCheckInput{}
			input = request.ReviewCheck
		} else if request.Action == "review-evidence" {
			request.ReviewEvidence = &workspace.ReviewEvidenceInput{}
			input = request.ReviewEvidence
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
			return usage("用法: repsync review-task-start PATH VERSION TITLE")
		}
		request.Path, request.Version, request.Title = paths[0], paths[1], paths[2]
	case "review-task-finish":
		if len(paths) != 4 {
			return usage("用法: repsync review-task-finish PATH TASK_ID VERSION JSON_FILE")
		}
		request.Path, request.RunID, request.Version = paths[0], paths[1], paths[2]
		request.ReviewFinish = &workspace.ReviewFinishInput{}
		if err := readInput(paths[3], request.ReviewFinish); err != nil {
			return usage(err.Error())
		}
	case "review-handover":
		if len(paths) != 2 {
			return usage("用法: repsync review-handover PATH TASK_ID")
		}
		request.Path, request.RunID = paths[0], paths[1]
	case "git-stash-show":
		if len(paths) != 2 {
			return usage("用法: repsync git-stash-show PATH OID")
		}
		request.Path, request.ObjectID = paths[0], paths[1]
	case "git-stash-apply":
		if len(paths) != 3 {
			return usage("用法: repsync git-stash-apply PATH OID VERSION")
		}
		request.Path, request.ObjectID, request.Version = paths[0], paths[1], paths[2]
	case "git-cleanup", "git-restore":
		if len(paths) != 2 {
			return usage("用法: repsync " + request.Action + " PATH VERSION_OR_RECORD_ID")
		}
		request.Path = paths[0]
		if request.Action == "git-cleanup" {
			request.Version = paths[1]
		} else {
			request.RunID = paths[1]
		}
	case "start", "stop-projects":
		if len(paths) == 0 {
			return usage("请明确指定项目路径")
		}
		request.Paths = paths
	case "status":
		request.Paths = paths
	case "resource-add":
		if len(paths) != 3 {
			return usage("用法: repsync resource-add ID SOURCE TARGET")
		}
		request.ResourceID, request.Path, request.Target = paths[0], paths[1], paths[2]
	case "resource-remove":
		if len(paths) != 1 {
			return usage("用法: repsync resource-remove ID")
		}
		request.ResourceID = paths[0]
	case "resource-preview":
		if len(paths) != 2 {
			return usage("用法: repsync resource-preview PATH ID")
		}
		request.Path, request.ResourceID = paths[0], paths[1]
	case "relocate":
		if len(paths) != 2 {
			return usage("用法: repsync relocate OLD_PATH NEW_PATH")
		}
		request.Path, request.Destination = paths[0], paths[1]
	case "run":
		if len(paths) != 3 {
			return usage("用法: repsync run [--wait] PATH PIPELINE_ID VERSION")
		}
		request.Path, request.ServiceID, request.Version = paths[0], paths[1], paths[2]
	case "pipeline-logs", "pipeline-stop":
		if len(paths) != 2 {
			return usage("用法: repsync " + request.Action + " PATH RUN_ID")
		}
		request.Path, request.RunID = paths[0], paths[1]
	case "sync-apply", "sync-restore":
		if len(paths) != 1 {
			return usage("请指定预览或记录编号")
		}
		request.RunID = paths[0]
	case "sync-source":
		if len(paths) > 1 {
			return usage("用法: repsync sync-source [FILE]")
		}
		if len(paths) == 1 {
			request.Path = paths[0]
		}
	case "stop", "logs":
		if len(paths) != 3 {
			return usage("用法: repsync " + request.Action + " PATH SERVICE_ID RUN_ID")
		}
		request.Path, request.ServiceID, request.RunID = paths[0], paths[1], paths[2]
	default:
		return usage("未知命令，请使用 repsync --help")
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
	case "run":
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
				latest.Error = "等待已结束（" + ctx.Err().Error() + "），任务保留，请按回执检查或停止"
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
				latest.OK, latest.Error = false, "流水线未成功，请查看运行结果与日志"
			}
		}
		if complete {
			return latest
		}
		select {
		case <-ctx.Done():
			latest.OK, latest.Error = false, "等待已结束（"+ctx.Err().Error()+"），任务保留，请按回执检查或停止"
			return latest
		case <-ticker.C:
		}
	}
}
