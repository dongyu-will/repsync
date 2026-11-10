package workspace

import (
	"context"
	"crypto/rand"
	"errors"
	"fmt"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"time"
	"unicode"
)

type CloneState struct {
	ID          string  `json:"id"`
	Remote      string  `json:"remote"`
	Destination string  `json:"destination"`
	Status      string  `json:"status"`
	Log         string  `json:"log"`
	Error       string  `json:"error"`
	Cloned      bool    `json:"cloned"`
	Project     Project `json:"project"`
}

type activeClone struct {
	mu     sync.Mutex
	state  CloneState
	log    *processLog
	cancel context.CancelFunc
}

var scpRemote = regexp.MustCompile(`^[A-Za-z0-9._][A-Za-z0-9._-]*@[A-Za-z0-9][A-Za-z0-9.-]*:[^\s]+$`)

func validateCloneRemote(remote string) error {
	if remote == "" || len(remote) > 4096 || strings.IndexFunc(remote, unicode.IsControl) >= 0 {
		return errors.New("请输入有效的远程仓库地址")
	}
	if !strings.Contains(remote, "://") {
		if scpRemote.MatchString(remote) {
			return nil
		}
		return errors.New("请输入 HTTPS 或 SSH 仓库地址，例如 git@github.com:owner/repo.git")
	}
	address, err := url.Parse(remote)
	if err != nil || address.Hostname() == "" || strings.HasPrefix(address.Hostname(), "-") || address.Path == "" || address.Path == "/" {
		return errors.New("远程仓库地址不完整")
	}
	switch address.Scheme {
	case "https", "http", "ssh", "git":
	default:
		return errors.New("仅支持 HTTP(S)、SSH 和 Git 远程地址")
	}
	if address.RawQuery != "" || address.Fragment != "" {
		return errors.New("仓库地址不能包含查询参数或片段，请使用 Git 克隆地址")
	}
	if address.User != nil {
		_, password := address.User.Password()
		if address.Scheme != "ssh" || password || strings.HasPrefix(address.User.Username(), "-") {
			return errors.New("请勿在地址中填写密码或令牌，请使用本机 Git 凭据或 SSH 密钥")
		}
	}
	return nil
}

// StartClone reserves a new directory and clones without executing project commands.
func (s *Service) StartClone(remote, parent, name string) (CloneState, error) {
	remote, parent, name = strings.TrimSpace(remote), strings.TrimSpace(parent), strings.TrimSpace(name)
	if err := validateCloneRemote(remote); err != nil {
		return CloneState{}, err
	}
	if !filepath.IsAbs(parent) {
		return CloneState{}, errors.New("请选择保存仓库的上级目录")
	}
	if name == "" || name == "." || name == ".." || filepath.Base(name) != name || strings.ContainsAny(name, "/\\\x00") {
		return CloneState{}, errors.New("目录名称必须是单个文件夹名称")
	}
	gitBinary, err := exec.LookPath("git")
	if err != nil {
		return CloneState{}, errors.New("未找到 Git，请先安装 Git 并确认应用可以访问")
	}
	canonical, err := filepath.EvalSymlinks(parent)
	if err != nil {
		return CloneState{}, fmt.Errorf("上级目录不可访问：%w", err)
	}
	root, err := os.OpenRoot(canonical)
	if err != nil {
		return CloneState{}, err
	}
	defer root.Close()
	s.taskMu.Lock()
	defer s.taskMu.Unlock()
	if s.closing {
		return CloneState{}, errors.New("应用正在退出")
	}
	if s.activeClone != nil {
		s.activeClone.mu.Lock()
		running := s.activeClone.state.Status == "running"
		s.activeClone.mu.Unlock()
		if running {
			return CloneState{}, errors.New("已有仓库正在克隆，请等待完成或停止后再试")
		}
	}
	id := rand.Text()
	logDir := filepath.Join(filepath.Dir(s.path), "clones")
	if err := os.MkdirAll(logDir, 0700); err != nil {
		return CloneState{}, err
	}
	file, err := os.OpenFile(filepath.Join(logDir, id+".log"), os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
	if err != nil {
		return CloneState{}, err
	}
	if err := root.Mkdir(name, 0700); err != nil {
		file.Close()
		os.Remove(file.Name())
		if errors.Is(err, os.ErrExist) {
			return CloneState{}, errors.New("目标目录已存在，请使用已有本地目录或更换目录名称")
		}
		return CloneState{}, fmt.Errorf("创建目标目录：%w", err)
	}
	state := CloneState{ID: id, Remote: remote, Destination: filepath.Join(canonical, name), Status: "running"}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Minute)
	active := &activeClone{state: state, log: &processLog{file: file, tail: true}, cancel: cancel}
	s.activeClone = active
	s.taskWG.Add(1)
	go func() {
		defer s.taskWG.Done()
		defer cancel()
		s.executeClone(ctx, gitBinary, canonical, name, active)
	}()
	return state, nil
}

func (s *Service) executeClone(ctx context.Context, gitBinary, parent, name string, active *activeClone) {
	cmd := exec.CommandContext(ctx, gitBinary, "-c", "protocol.file.allow=never", "-c", "protocol.ext.allow=never", "-c", "core.hooksPath=/dev/null", "clone", "--progress", "--", active.state.Remote, name)
	cmd.Dir = parent
	env := commandEnvironment(false)
	// Keep credential helpers and SSH-agent configuration; disallow terminal prompts.
	for _, entry := range env {
		key, _, _ := strings.Cut(entry, "=")
		if key != "GIT_TERMINAL_PROMPT" && key != "GIT_ASKPASS" && key != "SSH_ASKPASS" {
			cmd.Env = append(cmd.Env, entry)
		}
	}
	cmd.Env = append(cmd.Env, "GIT_TERMINAL_PROMPT=0", "GIT_ASKPASS=/usr/bin/false", "SSH_ASKPASS=/usr/bin/false")
	if os.Getenv("GIT_SSH_COMMAND") == "" && os.Getenv("GIT_SSH") == "" {
		cmd.Env = append(cmd.Env, "GIT_SSH_COMMAND=ssh -oBatchMode=yes")
	}
	cmd.Stdout, cmd.Stderr = active.log, active.log
	configureCommandProcess(cmd, false)
	err := cmd.Run()
	logErr := active.log.Close()
	active.mu.Lock()
	defer active.mu.Unlock()
	active.state.Cloned = err == nil
	if ctx.Err() != nil {
		active.state.Status = "cancelled"
		active.state.Error = "克隆已停止；如有部分文件，保留在目标目录。"
		if errors.Is(ctx.Err(), context.DeadlineExceeded) {
			active.state.Status = "timed_out"
			active.state.Error = "克隆超过 30 分钟，已停止；请检查网络后使用新目录重试。"
		}
		return
	}
	if err != nil {
		active.state.Status = "failure"
		active.state.Error = "克隆失败，请查看输出；检查地址、网络及本机仓库访问权限。如有部分文件，保留在目标目录。"
		return
	}
	if logErr != nil {
		active.state.Status = "failure"
		active.state.Error = "仓库已克隆，但日志保存失败：" + logErr.Error()
		return
	}
	project, err := s.Add(active.state.Destination)
	if err != nil {
		active.state.Status = "failure"
		active.state.Error = "仓库已克隆，但登记失败，可改用已有本地目录添加：" + err.Error()
		return
	}
	active.state.Project = project
	active.state.Status = "success"
}

// CloneStatus returns the last clone's bounded progress output.
func (s *Service) CloneStatus(id string) (CloneState, error) {
	s.taskMu.Lock()
	active := s.activeClone
	s.taskMu.Unlock()
	if active == nil {
		return CloneState{}, errors.New("没有这条克隆任务，应用可能已重启")
	}
	active.mu.Lock()
	defer active.mu.Unlock()
	if active.state.ID != id {
		return CloneState{}, errors.New("克隆任务已被替换")
	}
	state := active.state
	state.Log = active.log.Text()
	return state, nil
}

// CancelClone stops only the clone subprocess owned by this service.
func (s *Service) CancelClone(id string) error {
	s.taskMu.Lock()
	active := s.activeClone
	s.taskMu.Unlock()
	if active == nil {
		return errors.New("没有正在克隆的仓库")
	}
	active.mu.Lock()
	defer active.mu.Unlock()
	if active.state.ID != id {
		return errors.New("克隆任务标识不一致")
	}
	if active.state.Status == "running" {
		active.cancel()
	}
	return nil
}
