// Package control connects the CLI to the same workspace service owned by the desktop app.
package control

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"sync"
	"syscall"
	"time"

	"repsync/internal/workspace"
)

type Request struct {
	ResourceID      string                           `json:"resourceId,omitempty"`
	Target          string                           `json:"target,omitempty"`
	Action          string                           `json:"action"`
	Paths           []string                         `json:"paths,omitempty"`
	Path            string                           `json:"path,omitempty"`
	Destination     string                           `json:"destination,omitempty"`
	ServiceID       string                           `json:"serviceId,omitempty"`
	Version         string                           `json:"version,omitempty"`
	ObjectID        string                           `json:"objectId,omitempty"`
	RunID           string                           `json:"runId,omitempty"`
	Runs            []workspace.ServiceRunRef        `json:"runs,omitempty"`
	DryRun          bool                             `json:"dryRun,omitempty"`
	Title           string                           `json:"title,omitempty"`
	ReviewCheck     *workspace.ReviewCheckInput      `json:"reviewCheck,omitempty"`
	ReviewEvidence  *workspace.ReviewEvidenceInput   `json:"reviewEvidence,omitempty"`
	ReviewFinish    *workspace.ReviewFinishInput     `json:"reviewFinish,omitempty"`
	ReviewResources *[]workspace.ReviewResource      `json:"reviewResources,omitempty"`
	ReviewProfile   *workspace.ReviewProfile         `json:"reviewProfile,omitempty"`
	ReviewTask      *workspace.ReviewTaskContext     `json:"reviewTask,omitempty"`
	ReviewIssue     *workspace.ReviewAcknowledgement `json:"reviewIssue,omitempty"`
	CodexResource   *workspace.CodexResourceChange   `json:"codexResource,omitempty"`
}

type Response struct {
	OK    bool            `json:"ok"`
	Data  json.RawMessage `json:"data,omitempty"`
	Error string          `json:"error,omitempty"`
}

type ProjectStatus struct {
	Project  workspace.Project               `json:"project"`
	Services workspace.LocalServicesOverview `json:"services"`
	Error    string                          `json:"error,omitempty"`
}

type Server struct {
	http   *http.Server
	lock   *os.File
	socket string
	once   sync.Once
}

func SocketPath() (string, error) {
	if value := os.Getenv("REPSYNC_SOCKET"); value != "" {
		if !filepath.IsAbs(value) {
			return "", errors.New("REPSYNC_SOCKET 必须是绝对路径")
		}
		return value, nil
	}
	directory, err := os.UserConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(directory, "repsync", "control.sock"), nil
}

// Listen enforces one desktop owner per socket; an acquired lock permits stale socket recovery.
func Listen(socket string, service *workspace.Service) (*Server, error) {
	if err := os.MkdirAll(filepath.Dir(socket), 0700); err != nil {
		return nil, err
	}
	lockPath := socket + ".lock"
	if info, err := os.Lstat(lockPath); err == nil && !info.Mode().IsRegular() {
		return nil, errors.New("控制锁必须是普通文件")
	}
	lock, err := os.OpenFile(lockPath, os.O_CREATE|os.O_RDWR, 0600)
	if err != nil {
		return nil, err
	}
	if err := syscall.Flock(int(lock.Fd()), syscall.LOCK_EX|syscall.LOCK_NB); err != nil {
		lock.Close()
		return nil, errors.New("已有 repsync 实例持有本地控制入口")
	}
	fail := func(err error) (*Server, error) {
		syscall.Flock(int(lock.Fd()), syscall.LOCK_UN)
		lock.Close()
		return nil, err
	}
	if info, err := os.Lstat(socket); err == nil {
		if info.Mode()&os.ModeSocket == 0 {
			return fail(errors.New("控制入口路径已存在且不是 socket"))
		}
		if err := os.Remove(socket); err != nil {
			return fail(err)
		}
	} else if !errors.Is(err, os.ErrNotExist) {
		return fail(err)
	}
	listener, err := net.Listen("unix", socket)
	if err != nil {
		return fail(err)
	}
	if err := os.Chmod(socket, 0600); err != nil {
		listener.Close()
		return fail(err)
	}
	server := &Server{lock: lock, socket: socket}
	server.http = &http.Server{Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.Method != http.MethodPost || r.URL.Path != "/v1/control" || r.Header.Get("Origin") != "" {
			w.WriteHeader(http.StatusBadRequest)
			writeResponse(w, nil, errors.New("无效控制请求"))
			return
		}
		var request Request
		decoder := json.NewDecoder(http.MaxBytesReader(w, r.Body, 128*1024))
		decoder.DisallowUnknownFields()
		if err := decoder.Decode(&request); err != nil {
			w.WriteHeader(http.StatusBadRequest)
			writeResponse(w, nil, err)
			return
		}
		if err := decoder.Decode(new(any)); !errors.Is(err, io.EOF) {
			w.WriteHeader(http.StatusBadRequest)
			writeResponse(w, nil, errors.New("请求只能包含一个 JSON 对象"))
			return
		}
		data, err := dispatch(service, request)
		writeResponse(w, data, err)
	}), ReadHeaderTimeout: 5 * time.Second, ReadTimeout: 10 * time.Second, WriteTimeout: 30 * time.Second, IdleTimeout: 15 * time.Second}
	go func() { _ = server.http.Serve(listener) }()
	return server, nil
}

func (s *Server) Close() {
	s.once.Do(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if err := s.http.Shutdown(ctx); err != nil {
			_ = s.http.Close()
		}
		_ = os.Remove(s.socket)
		_ = syscall.Flock(int(s.lock.Fd()), syscall.LOCK_UN)
		_ = s.lock.Close()
	})
}

func writeResponse(w io.Writer, data any, err error) {
	response := Response{OK: err == nil}
	if data != nil {
		response.Data, _ = json.Marshal(data)
	}
	if err != nil {
		response.Error = err.Error()
	}
	_ = json.NewEncoder(w).Encode(response)
}

func dispatch(service *workspace.Service, request Request) (any, error) {
	switch request.Action {
	case "codex-resources":
		return service.ProjectCodexResources(request.Path)
	case "codex-resource-preview", "codex-resource-apply":
		if request.CodexResource == nil {
			return nil, errors.New("缺少 Codex 接入变更")
		}
		if request.Action == "codex-resource-preview" {
			return service.PreviewCodexResource(request.Path, *request.CodexResource)
		}
		return nil, service.ApplyCodexResource(request.Path, *request.CodexResource, request.Version)
	case "ai-resources":
		return service.AIResources(request.Path)
	case "ai-usage":
		return service.AIUsage(request.Path)
	case "review":
		return service.ReviewProject(request.Path)
	case "review-check":
		if request.ReviewCheck == nil {
			return nil, errors.New("缺少检查记录")
		}
		return service.RecordReviewCheck(request.Path, request.Version, *request.ReviewCheck)
	case "review-evidence":
		if request.ReviewEvidence == nil {
			return nil, errors.New("缺少核验记录")
		}
		return service.RecordReviewEvidence(request.Path, request.Version, *request.ReviewEvidence)
	case "review-resources":
		if request.ReviewResources == nil {
			return nil, errors.New("缺少检查对象列表")
		}
		return nil, service.SaveReviewResources(request.Path, request.Version, *request.ReviewResources)
	case "review-task-start":
		return service.StartReviewTask(request.Path, request.Version, request.Title)
	case "review-profile":
		if request.ReviewProfile == nil {
			return nil, errors.New("缺少项目适用项")
		}
		return nil, service.SaveReviewProfile(request.Path, request.Version, *request.ReviewProfile)
	case "review-task-context":
		if request.ReviewTask == nil {
			return nil, errors.New("缺少任务范围")
		}
		return service.StartReviewTaskWithContext(request.Path, request.Version, *request.ReviewTask)
	case "review-issue":
		if request.ReviewIssue == nil {
			return nil, errors.New("缺少提醒处理状态")
		}
		i := request.ReviewIssue
		return service.SetReviewIssueState(request.Path, request.Version, i.Key, i.State, i.Reason)
	case "review-task-finish":
		if request.ReviewFinish == nil {
			return nil, errors.New("缺少交接内容")
		}
		return service.FinishReviewTask(request.Path, request.RunID, request.Version, *request.ReviewFinish)
	case "review-handover":
		return service.ReviewHandover(request.Path, request.RunID)
	case "git-stashes":
		return service.GitStashes(request.Path)
	case "git-stash-show":
		return service.GitStashChanges(request.Path, request.ObjectID)
	case "git-stash-apply":
		result, err := service.ApplyGitStash(request.Path, request.ObjectID, request.Version)
		if err == nil && !result.Applied {
			err = errors.New(result.Message)
		}
		return result, err
	case "git-workspace":
		return service.GitWorkspace(request.Path)
	case "git-cleanup":
		return service.CleanupGitWorkspace(request.Path, request.Version)
	case "git-restore":
		return service.RestoreGitCleanup(request.Path, request.RunID)
	case "doctor":
		report, err := service.DiagnoseProject(request.Path)
		if err == nil && !report.Ready {
			err = errors.New("项目接入检查发现阻塞项")
		}
		return report, err
	case "resources":
		return service.Resources()
	case "resource-add":
		return service.ConnectResource(request.ResourceID, request.Path, request.Target)
	case "resource-remove":
		return nil, service.RemoveResource(request.ResourceID)
	case "resource-status":
		return service.ResourceStatus(request.Path)
	case "resource-preview":
		return service.PreviewResource(request.Path, request.ResourceID)
	case "resource-history":
		return service.ResourceHistory(request.Path)
	case "projects":
		return service.List()
	case "add":
		return service.Add(request.Path)
	case "remove":
		return nil, service.RemoveProject(request.Path)
	case "relocate":
		return service.RelocateProject(request.Path, request.Destination)
	case "ping":
		return struct {
			PID int `json:"pid"`
		}{os.Getpid()}, nil
	case "status":
		projects, err := service.List()
		if err != nil {
			return nil, err
		}
		results := []ProjectStatus{}
		requested := map[string]bool{}
		for _, path := range request.Paths {
			requested[path] = true
		}
		all := len(requested) == 0
		for _, project := range projects {
			if !all && !requested[project.Path] {
				continue
			}
			overview, err := service.LocalServices(project.Path)
			result := ProjectStatus{Project: project, Services: overview}
			if err != nil {
				result.Error = err.Error()
			}
			results = append(results, result)
			delete(requested, project.Path)
		}
		if len(requested) > 0 {
			return results, errors.New("部分目录未登记，请先执行 add")
		}
		return results, nil
	case "start":
		return service.StartProjects(request.Paths, request.DryRun)
	case "stop-projects":
		return service.StopProjects(request.Paths)
	case "readiness":
		return service.ServiceReadiness(request.Runs)
	case "stop":
		return nil, service.StopLocalService(request.Path, request.ServiceID, request.RunID)
	case "logs":
		return service.LocalServiceLog(request.Path, request.ServiceID, request.RunID)
	case "pipelines":
		return service.Pipelines(request.Path)
	case "run":
		return service.StartPipeline(request.Path, request.ServiceID, request.Version)
	case "runs":
		return service.PipelineRuns(request.Path)
	case "pipeline-logs":
		return service.PipelineLog(request.Path, request.RunID)
	case "pipeline-stop":
		return nil, service.StopPipeline(request.Path, request.RunID)
	case "sync-source":
		if request.Path != "" {
			return service.ConnectSharedSource(request.Path)
		}
		return service.SharedSource()
	case "sync-status":
		return service.SharedAdoptions()
	case "sync-preview":
		return service.PreviewUpdate(request.Path)
	case "policy-preview":
		return service.PreviewPolicyGuidance(request.Path)
	case "sync-apply":
		return service.ApplyUpdate(request.RunID)
	case "sync-history":
		return service.UpdateHistory(request.Path)
	case "sync-restore":
		return service.RestoreUpdate(request.RunID)
	default:
		return nil, fmt.Errorf("未知操作 %q", request.Action)
	}
}
