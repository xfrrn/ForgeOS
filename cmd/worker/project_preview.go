package main

import (
	"bytes"
	"context"
	"crypto/subtle"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"sync"
	"time"

	"forgeos/internal/agents"
	"forgeos/internal/localrepo"
	"forgeos/internal/preview"
	"forgeos/internal/store"
	"forgeos/internal/validation"
	"github.com/google/uuid"
)

type projectPreviewInstance struct {
	process *os.Process
	done    chan struct{}
	port    int
	id      string
	commit  string
}

type projectPreviewRuntime struct {
	mu       sync.Mutex
	instance *projectPreviewInstance
}

type projectPreviewProcess struct {
	PowerShell, Dir, Script, LogFile, Secret, Instance, HealthPath string
	Timeout                                                        int
	API                                                            *projectPreviewAPI
}

type projectPreviewReady struct {
	Port, HTTPStatus int
	Error            string
}

type projectPreviewManifest struct {
	Script     string             `json:"script"`
	HealthPath string             `json:"health_path"`
	Timeout    int                `json:"timeout_seconds"`
	API        *projectPreviewAPI `json:"api,omitempty"`
}

type projectPreviewAPI struct {
	Prefix     string `json:"prefix"`
	HealthPath string `json:"health_path"`
	Port       int    `json:"port,omitempty"`
}

var projectPreviewPrefix = regexp.MustCompile(`^(/[A-Za-z0-9_-]+)+$`)

func validProjectHealthPath(value string) bool {
	u, err := url.ParseRequestURI(value)
	return err == nil && u.Scheme == "" && u.Host == "" && strings.HasPrefix(value, "/") &&
		!strings.HasPrefix(value, "//") && !strings.ContainsAny(value, "\\#")
}

func (api *projectPreviewAPI) matches(path string) bool {
	return api != nil && (path == api.Prefix || strings.HasPrefix(path, api.Prefix+"/"))
}

func previewManifest(ctx context.Context, repo, commit string) (projectPreviewManifest, error) {
	var config projectPreviewManifest
	raw, overflow, err := localrepo.LimitedGit(ctx, repo, 8<<10, "show", commit+":forgeos-preview.json")
	if err != nil || overflow {
		return config, errors.New("该版本未提供可读的 forgeos-preview.json")
	}
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	if decoder.Decode(&config) != nil || decoder.Decode(new(any)) != io.EOF {
		return config, errors.New("forgeos-preview.json 格式无效")
	}
	if !validProjectHealthPath(config.HealthPath) ||
		!filepath.IsLocal(config.Script) || !strings.EqualFold(filepath.Ext(config.Script), ".ps1") ||
		config.Timeout < 1 || config.Timeout > 120 {
		return config, errors.New("预览须声明项目内的 PowerShell 脚本、站内健康检查路径和 1–120 秒超时")
	}
	if config.API != nil {
		if config.API.Prefix == "" {
			config.API.Prefix = "/api"
		}
		if !projectPreviewPrefix.MatchString(config.API.Prefix) || !validProjectHealthPath(config.API.HealthPath) ||
			config.API.Port < 0 || config.API.Port > 65535 {
			return config, errors.New("API 预览须声明站内路径前缀和后端健康检查路径")
		}
	}
	return config, nil
}

func (v *projectPreviewInstance) exited() bool {
	select {
	case <-v.done:
		return true
	default:
		return false
	}
}

func (v *projectPreviewInstance) stop(ctx context.Context) error {
	if !v.exited() {
		if err := v.process.Kill(); err != nil && !errors.Is(err, os.ErrProcessDone) {
			return errors.New("无法关闭本次预览的 Windows 进程组")
		}
	}
	select {
	case <-v.done:
		return nil
	case <-ctx.Done():
		return errors.New("未能确认本次预览进程已退出")
	}
}

var previewHealthClient = &http.Client{Transport: &http.Transport{}, Timeout: 4 * time.Second,
	CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}

func (v *projectPreviewInstance) health(ctx context.Context, secret string) int {
	if v.exited() {
		return 0
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, fmt.Sprintf("http://127.0.0.1:%d%s", v.port, preview.ProjectHealthPath), nil)
	if err != nil {
		return 0
	}
	req.Header.Set(preview.ProjectProofHeader, preview.ProjectProof(secret, v.id, "request"))
	resp, err := previewHealthClient.Do(req)
	if err != nil {
		return 0
	}
	defer resp.Body.Close()
	if subtle.ConstantTimeCompare([]byte(resp.Header.Get(preview.ProjectProofHeader)), []byte(preview.ProjectProof(secret, v.id, "response"))) != 1 ||
		resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return 0
	}
	return resp.StatusCode
}

func startProjectPreview(ctx context.Context, repo string, task int64, work, secret string) (*projectPreviewInstance, int, string, error) {
	out, err := localrepo.Git(ctx, repo, "rev-parse", "--verify", "HEAD^{commit}")
	commit := strings.TrimSpace(string(out))
	if err != nil || !validation.FullCommit(commit) {
		return nil, 0, "revision", errors.New("无法锁定项目的当前提交")
	}
	config, err := previewManifest(ctx, repo, commit)
	if err != nil {
		return nil, 0, "configuration", err
	}
	dir, release, err := ciSnapshot(ctx, repo, commit)
	if err != nil {
		return nil, 0, "checkout", err
	}
	releaseOnError := true
	defer func() {
		if releaseOnError {
			release()
		}
	}()
	script := filepath.Join(dir, config.Script)
	resolved, err := filepath.EvalSymlinks(script)
	if err != nil || resolved != script {
		return nil, 0, "configuration", errors.New("预览脚本须位于当前项目快照中")
	}
	if info, err := os.Stat(script); err != nil || !info.Mode().IsRegular() {
		return nil, 0, "configuration", errors.New("项目预览脚本不存在或不可读")
	}
	shell := os.Getenv("FORGEOS_PROJECT_PREVIEW_POWERSHELL")
	if shell == "" {
		shell, err = exec.LookPath("pwsh.exe")
	} else if !filepath.IsAbs(shell) {
		err = errors.New("configure an absolute PowerShell path")
	}
	bin, binErr := os.Executable()
	if err != nil || binErr != nil {
		return nil, 0, "launch", errors.New("Worker 无法找到 PowerShell 7 或本机预览启动器")
	}
	id := uuid.NewString()
	payload, err := json.Marshal(projectPreviewProcess{PowerShell: shell, Dir: dir, Script: script,
		LogFile: filepath.Join(work, fmt.Sprintf("preview-%d.log", task)), Secret: secret, Instance: id,
		HealthPath: config.HealthPath, Timeout: config.Timeout, API: config.API})
	if err != nil {
		return nil, 0, "launch", err
	}
	command := exec.Command(bin, "project-preview-process")
	command.Env = harnessEnv()
	input, err := command.StdinPipe()
	if err != nil {
		return nil, 0, "launch", err
	}
	defer func() {
		if releaseOnError {
			input.Close()
		}
	}()
	stream, err := command.StdoutPipe()
	if err != nil {
		return nil, 0, "launch", err
	}
	if err := command.Start(); err != nil {
		stream.Close()
		return nil, 0, "launch", errors.New("无法启动本机预览运行进程")
	}
	if _, err := input.Write(append(payload, '\n')); err != nil {
		_ = command.Process.Kill()
		_ = command.Wait()
		return nil, 0, "launch", errors.New("无法发送预览启动配置")
	}
	v := &projectPreviewInstance{process: command.Process, done: make(chan struct{}), id: id, commit: commit}
	ready := make(chan projectPreviewReady, 1)
	go func() {
		var result projectPreviewReady
		if json.NewDecoder(io.LimitReader(stream, 8192)).Decode(&result) != nil {
			result.Error = "本机预览运行进程没有返回启动结果"
		}
		ready <- result
		_ = command.Wait()
		_ = input.Close()
		close(v.done)
		release()
	}()
	releaseOnError = false
	select {
	case value := <-ready:
		if value.Port < 1 || value.Port > 65535 || value.HTTPStatus < 200 || value.HTTPStatus >= 300 {
			_ = input.Close()
			if value.Error == "" {
				value.Error = "本机预览没有通过健康检查"
			}
			return nil, 0, "readiness", errors.New(value.Error)
		}
		v.port = value.Port
		return v, value.HTTPStatus, "", nil
	case <-ctx.Done():
		_ = input.Close()
		_ = v.process.Kill()
		return nil, 0, "launch", errors.New("预览启动已中断")
	}
}

func (w *worker) applyProjectPreview(ctx context.Context, job store.Job) store.Result {
	p := job.Preview
	r := &store.ProjectPreviewReport{ProjectID: p.ProjectID, OperationID: job.TaskID, Action: p.Action, State: "stopped", Outcome: "ok"}
	result := store.Result{ProjectPreview: r}
	fail := func(stage string, err error) store.Result {
		r.State, r.Outcome = "unknown", "failed"
		r.Error = &store.ProjectPreviewError{Stage: stage, Message: err.Error()}
		result.ExitCode, result.Error = 1, r.Error.Message
		return result
	}
	value, _ := w.projectPreviews.LoadOrStore(p.ProjectID, &projectPreviewRuntime{})
	state := value.(*projectPreviewRuntime)
	state.mu.Lock()
	defer state.mu.Unlock()
	if err := ctx.Err(); err != nil {
		return fail("lease", errors.New("预览任务已中断"))
	}
	v := state.instance
	if v != nil && v.exited() {
		state.instance, v = nil, nil
	}
	if p.Action == "stop" {
		if v != nil {
			if err := v.stop(ctx); err != nil {
				return fail("stop", err)
			}
			state.instance = nil
		}
		return result
	}
	publish := func(v *projectPreviewInstance, status int) store.Result {
		r.State, r.Instance, r.Commit, r.HTTPStatus = "running", v.id, v.commit, status
		result.PreviewPort = v.port
		return result
	}
	if v != nil {
		status := v.health(ctx, p.Secret)
		if status == 0 {
			return fail("readiness", errors.New("已登记的预览未通过健康检查，请先停止再启动"))
		}
		return publish(v, status)
	}
	if p.Action == "status" {
		return result
	}
	repo, err := localrepo.ResolveAssigned(ctx, w.repoRoot, job.RepoPath, job.RepoRelativePath)
	if err != nil {
		return fail("repository", errors.New("无法核对项目仓库"))
	}
	v, status, stage, err := startProjectPreview(ctx, repo, job.TaskID, job.Workspace, p.Secret)
	if err != nil {
		return fail(stage, err)
	}
	state.instance = v
	return publish(v, status)
}

func (w *worker) closeProjectPreviews() {
	w.projectPreviews.Range(func(_, value any) bool {
		state := value.(*projectPreviewRuntime)
		state.mu.Lock()
		defer state.mu.Unlock()
		if state.instance != nil {
			ctx, cancel := context.WithTimeout(context.Background(), 4*time.Second)
			defer cancel()
			_ = state.instance.stop(ctx)
		}
		return true
	})
}

func (w *worker) executeProjectPreview(ctx context.Context, job store.Job) store.Result {
	p := job.Preview
	if p == nil || p.ProjectID < 1 || !store.ValidProjectPreviewAction(p.Action) {
		return failed("invalid project preview operation", nil)
	}
	failure := func(message string) store.Result {
		r := failed(message, nil)
		r.ProjectPreview = &store.ProjectPreviewReport{ProjectID: p.ProjectID, OperationID: job.TaskID,
			Action: p.Action, Outcome: "failed", State: "unknown", Error: &store.ProjectPreviewError{Stage: "agent", Message: message}}
		return r
	}
	ctx, cancel := context.WithTimeout(ctx, 5*time.Minute)
	defer cancel()
	expected := filepath.Join(w.workspaceRoot, fmt.Sprintf("execution-%d", job.ExecutionID))
	if job.ExecutionID < 1 || !strings.EqualFold(filepath.Clean(job.Workspace), expected) {
		return failure("预览操作的工作区无效")
	}
	if err := os.Mkdir(expected, 0700); err != nil {
		return failure("无法创建预览操作工作区")
	}
	var (
		once   sync.Once
		result store.Result
	)
	apply := func() store.Result {
		once.Do(func() { result = w.applyProjectPreview(ctx, job) })
		return result
	}
	listener, err := net.Listen("tcp4", "127.0.0.1:0")
	if err != nil {
		return failure("无法连接本机预览 Agent 工具")
	}
	path := "/" + uuid.NewString()
	server := &http.Server{ReadHeaderTimeout: 5 * time.Second, Handler: http.HandlerFunc(func(rw http.ResponseWriter, r *http.Request) {
		if r.URL.Path != path {
			http.NotFound(rw, r)
			return
		}
		if r.Method != http.MethodPost {
			rw.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
		previewAgentTool(rw, r, apply)
	})}
	go func() { _ = server.Serve(listener) }()
	defer server.Close()
	endpoint, _ := json.Marshal("http://" + listener.Addr().String() + path)
	// The Owner already selected the operation. The session exposes only
	// this one tool, bound to that operation with zero model-supplied inputs.
	disabled := slices.DeleteFunc(store.StructuredCodexProfile().DisabledFeatures, func(name string) bool {
		return name == "code_mode" || name == "code_mode_host"
	})
	adapter := codexCLI{bin: w.codex, version: w.codexVersion, sandbox: store.CodexPlannerSandbox,
		disabledFeatures: disabled,
		options: []string{"-c", "mcp_servers.forgeos_preview={url=" + string(endpoint) +
			`,required=true,tool_timeout_sec=150,enabled_tools=["control_preview"],default_tools_approval_mode="approve"}`,
			"-c", `mcp_servers.forgeos_preview.tools.control_preview.approval_mode="approve"`, "--skip-git-repo-check"}}
	prompt := agents.Builtin("preview-lifecycle")
	if job.Agent != nil {
		settings := *job.Agent
		settings.Packages = nil
		ctx = w.agentContext(ctx, &settings)
		prompt = settings.Apply(prompt)
	}
	prompt += " The authorized action is " + p.Action + ". Call control_preview exactly once, with no arguments. Return its JSON receipt."
	modelCtx := ctx
	if p.Action != "start" {
		var stopModel context.CancelFunc
		modelCtx, stopModel = context.WithTimeout(ctx, 15*time.Second)
		defer stopModel()
	}
	_, _ = withResource(modelCtx, &executionResources{worker: w, job: job}, "model", func() (string, error) {
		return adapter.runTurn(modelCtx, processLog{}, expected, prompt, "")
	})
	once.Do(func() {
		if p.Action == "stop" || p.Action == "status" {
			result = w.applyProjectPreview(ctx, job)
		} else {
			result = failure("预览启停 Agent 未调用启动工具，请重试")
		}
	})
	return result
}

func previewAgentTool(w http.ResponseWriter, r *http.Request, apply func() store.Result) {
	var input struct {
		ID     json.RawMessage
		Method string
		Params struct {
			Name            string
			ProtocolVersion string
		}
	}
	if json.NewDecoder(http.MaxBytesReader(w, r.Body, 8<<10)).Decode(&input) != nil {
		http.Error(w, "invalid MCP request", 400)
		return
	}
	if len(input.ID) == 0 {
		w.WriteHeader(http.StatusAccepted)
		return
	}
	type object = map[string]any
	reply := object{"jsonrpc": "2.0", "id": input.ID}
	switch input.Method {
	case "initialize":
		reply["result"] = object{"protocolVersion": input.Params.ProtocolVersion, "serverInfo": object{"name": "forgeos_preview", "version": "1.0"}, "capabilities": object{"tools": object{}}}
	case "ping":
		reply["result"] = object{}
	case "tools/list":
		reply["result"] = object{"tools": []object{{
			"name": "control_preview", "description": "Run the exact Owner-authorized preview operation on this Worker.",
			"inputSchema": object{"type": "object", "properties": object{}, "additionalProperties": false},
			"annotations": object{"readOnlyHint": false, "destructiveHint": false, "openWorldHint": false, "idempotentHint": true},
		}}}
	case "tools/call":
		if input.Params.Name == "control_preview" {
			result := apply()
			data, _ := json.Marshal(result.ProjectPreview)
			reply["result"] = object{"content": []object{{"type": "text", "text": string(data)}}}
		}
	}
	if reply["result"] == nil {
		reply["error"] = object{"code": -32601, "message": "method not available"}
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(reply)
}
