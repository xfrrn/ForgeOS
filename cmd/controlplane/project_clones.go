package main

import (
	"context"
	"errors"
	"log"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"time"

	"forgeos/internal/bao"
	"forgeos/internal/localrepo"
	"forgeos/internal/store"
)

type gitCredential struct {
	username string
	token    string
}

type cloneRemote struct {
	credential *gitCredential
	URL        string
	Host       string
	Repository string
	Parts      []string
}

var (
	clonePart    = regexp.MustCompile(`^[a-zA-Z0-9._][a-zA-Z0-9._-]{0,99}$`)
	cloneSSH     = regexp.MustCompile(`^git@([a-zA-Z0-9.-]+):([^/].*)$`)
	cloneHost    = regexp.MustCompile(`^[a-z0-9]([a-z0-9.-]{0,251}[a-z0-9])?$`)
	errCloneURL  = errors.New("请填写受支持的 Git 仓库链接，且地址中不能包含密码、查询参数或分支页面。")
	errCloneHost = errors.New("此 Git 服务器尚未配置。默认支持 GitHub、GitLab 和 Bitbucket。")
)

func validateCloneHost(host, extraHosts string) error {
	if !cloneHost.MatchString(host) || strings.Contains(host, "..") || !filepath.IsLocal(host) {
		return errCloneURL
	}
	if host == "github.com" || host == "gitlab.com" || host == "bitbucket.org" || slices.ContainsFunc(strings.Split(extraHosts, ","), func(v string) bool {
		return strings.EqualFold(strings.TrimSpace(v), host)
	}) {
		return nil
	}
	return errCloneHost
}

func parseCloneRemote(raw string, extraHosts string) (cloneRemote, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" || len(raw) > 1600 || strings.ContainsAny(raw, " \r\n\t\\\x00") {
		return cloneRemote{}, errCloneURL
	}
	if ssh := cloneSSH.FindStringSubmatch(raw); ssh != nil {
		raw = "ssh://git@" + ssh[1] + "/" + ssh[2]
	}
	u, err := url.Parse(raw)
	if err != nil || u.Opaque != "" || u.RawQuery != "" || u.ForceQuery || u.Fragment != "" ||
		u.RawPath != "" || u.EscapedPath() != u.Path || u.Hostname() == "" ||
		(strings.Contains(u.Host, ":") && u.Port() == "") {
		return cloneRemote{}, errCloneURL
	}
	if u.Scheme == "ssh" {
		if u.User == nil || u.User.String() != "git" || (u.Port() != "" && u.Port() != "22") {
			return cloneRemote{}, errCloneURL
		}
	} else if u.Scheme != "https" || u.User != nil || (u.Port() != "" && u.Port() != "443") {
		return cloneRemote{}, errCloneURL
	}
	host := strings.ToLower(u.Hostname())
	if err := validateCloneHost(host, extraHosts); err != nil {
		return cloneRemote{}, err
	}
	path := strings.TrimSuffix(strings.Trim(u.Path, "/"), ".git")
	parts := strings.Split(path, "/")
	if len(parts) < 2 || len(parts) > 12 || ((host == "github.com" || host == "bitbucket.org") && len(parts) != 2) {
		return cloneRemote{}, errCloneURL
	}
	for _, part := range parts {
		if !clonePart.MatchString(part) || !filepath.IsLocal(part) || strings.HasSuffix(part, ".") || strings.EqualFold(part, ".git") {
			return cloneRemote{}, errCloneURL
		}
	}
	if host == "github.com" {
		path = strings.ToLower(path)
		parts = strings.Split(path, "/")
	}
	exact := (&url.URL{Scheme: "https", Host: host, Path: "/" + path + ".git"}).String()
	return cloneRemote{URL: exact, Host: host, Repository: path, Parts: parts}, nil
}

type projectCloneService struct {
	credentials *bao.Client
	data        *store.Store
	localRoot   string
	extraHosts  string
}

type cloneDirectorySettings struct {
	Root      string `json:"root"`
	Directory string `json:"directory"`
	LocalRoot string `json:"local_root"`
}

type cloneJobView struct {
	ID      int64                   `json:"id"`
	Status  string                  `json:"status"`
	Path    string                  `json:"path,omitempty"`
	Error   string                  `json:"error,omitempty"`
	Project *store.ProjectWorkspace `json:"project,omitempty"`
}

func (c projectCloneService) settings(ctx context.Context) (cloneDirectorySettings, error) {
	dir, err := c.data.CloneDirectory(ctx)
	base := localrepo.ManagedRoot()
	if dir == "" {
		dir = base
	}
	return cloneDirectorySettings{Root: base, Directory: dir, LocalRoot: c.localRoot}, err
}

func (c projectCloneService) register(mux *http.ServeMux, owner string) {
	mux.HandleFunc("GET /projects/clone-settings", authorized(owner, func(w http.ResponseWriter, r *http.Request) {
		settings, err := c.settings(r.Context())
		respond(w, http.StatusOK, settings, err)
	}))
	mux.HandleFunc("PUT /projects/clone-settings", authorized(owner, func(w http.ResponseWriter, r *http.Request) {
		var input struct {
			Directory string `json:"directory"`
		}
		if !decode(w, r, &input) {
			return
		}
		dir, err := localrepo.ResolveManagedDirectory(strings.TrimSpace(input.Directory))
		if err != nil {
			problem(w, http.StatusBadRequest, "请选择 ForgeOS 托管根目录内可用的目录。克隆目录需要位于现有 Git 工作树之外。")
			return
		}
		if err := c.data.SetCloneDirectory(r.Context(), dir); err != nil {
			respond(w, 0, nil, err)
			return
		}
		respond(w, http.StatusOK, cloneDirectorySettings{Root: localrepo.ManagedRoot(), Directory: dir, LocalRoot: c.localRoot}, nil)
	}))
	mux.HandleFunc("POST /projects/clone", authorized(owner, func(w http.ResponseWriter, r *http.Request) {
		var input struct {
			URL          string `json:"url"`
			CredentialID int64  `json:"credential_id"`
		}
		if !decode(w, r, &input) {
			return
		}
		remote, err := parseCloneRemote(input.URL, c.extraHosts)
		if err != nil {
			problem(w, http.StatusBadRequest, err.Error())
			return
		}
		settings, err := c.settings(r.Context())
		if err != nil {
			respond(w, 0, nil, err)
			return
		}
		if settings.Root == "" {
			problem(w, http.StatusServiceUnavailable, "当前服务没有配置 ForgeOS 托管仓库根目录。")
			return
		}
		if input.CredentialID != 0 {
			if _, err = c.checkCredential(r.Context(), input.CredentialID, remote); err != nil {
				credentialProblem(w, err)
				return
			}
		}
		job, err := c.data.QueueRepositoryClone(r.Context(), remote.URL, remote.Repository, settings.Directory, input.CredentialID)
		if errors.Is(err, store.ErrConflict) {
			problem(w, 409, "该仓库正使用另一个身份克隆。请等当前任务结束后再切换。")
			return
		}
		if err != nil {
			respond(w, 0, nil, err)
			return
		}
		status := http.StatusAccepted
		if job.Status == "done" {
			current, err := c.data.Project(r.Context(), job.ProjectID)
			if err != nil {
				respond(w, 0, nil, err)
				return
			}
			if current.RepositoryCredentialID != input.CredentialID {
				problem(w, 409, "该仓库已接入。请打开已有项目，在仓库同步面板更改访问身份。")
				return
			}
			status = http.StatusOK
		}
		c.respondJob(w, r, job, status)
	}))
	mux.HandleFunc("GET /projects/clone/jobs/{id}", authorized(owner, func(w http.ResponseWriter, r *http.Request) {
		id, ok := pathID(w, r)
		if !ok {
			return
		}
		job, err := c.data.RepositoryClone(r.Context(), id)
		if err != nil {
			respond(w, 0, nil, err)
			return
		}
		c.respondJob(w, r, job, http.StatusOK)
	}))
}

func (c projectCloneService) respondJob(w http.ResponseWriter, r *http.Request, job store.RepositoryClone, status int) {
	view := cloneJobView{ID: job.ID, Status: job.Status, Path: job.Path, Error: job.Error}
	if job.ProjectID != 0 {
		project, err := c.data.ProjectWorkspace(r.Context(), job.ProjectID)
		if err != nil {
			respond(w, 0, nil, err)
			return
		}
		view.Project = &project
	}
	respond(w, status, view, nil)
}

func (c projectCloneService) run(ctx context.Context) {
	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()
	for ctx.Err() == nil {
		job, err := c.data.ClaimRepositoryClone(ctx)
		if err != nil && ctx.Err() == nil {
			log.Printf("claim repository clone: %v", err)
		}
		if job != nil && err == nil {
			c.execute(ctx, *job)
			continue
		}
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
	}
}

func (c projectCloneService) execute(ctx context.Context, job store.RepositoryClone) {
	timeout, cancel := context.WithTimeout(ctx, 12*time.Minute)
	defer cancel()
	path, err := c.clone(timeout, job)
	settle, stop := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
	defer stop()
	switch {
	case err == nil:
		// Keep a ready checkout if the database connection is lost during commit.
		err = c.data.CompleteRepositoryClone(settle, job.ID, job.Lease, path)
	case ctx.Err() != nil:
		err = c.data.RequeueRepositoryClone(settle, job.ID, job.Lease)
	default:
		err = c.data.FailRepositoryClone(settle, job.ID, job.Lease, err.Error())
	}
	if err != nil {
		log.Printf("settle repository clone %d: %v", job.ID, err)
	}
}

func (c projectCloneService) clone(ctx context.Context, job store.RepositoryClone) (string, error) {
	remote, err := parseCloneRemote(job.URL, c.extraHosts)
	if err != nil {
		return "", err
	}
	if err = c.authenticateRemote(ctx, job.CredentialID, &remote); err != nil {
		if errors.Is(err, errCredentialScope) || errors.Is(err, errCredentialVault) {
			return "", err
		}
		return "", errors.New("所选仓库凭据不可用。")
	}
	parent := filepath.Join(append([]string{job.Directory, remote.Host}, remote.Parts[:len(remote.Parts)-1]...)...)
	parent, err = localrepo.ResolveManagedDirectory(parent)
	if err != nil {
		return "", errors.New("无法创建克隆目录。请检查 ForgeOS 专属目录及服务账号的写入权限。")
	}
	name := remote.Parts[len(remote.Parts)-1]
	path, err := os.MkdirTemp(parent, name+"-"+strconv.FormatInt(job.ID, 10)+"-")
	if err != nil {
		return "", errors.New("无法在 ForgeOS 专属目录创建仓库文件夹。")
	}
	var ready bool
	defer func() {
		if ready {
			return
		}
		if actual, err := filepath.EvalSymlinks(path); err == nil && actual == path && filepath.Dir(actual) == parent {
			if err := os.RemoveAll(actual); err != nil {
				log.Printf("remove unregistered clone %d: %v", job.ID, err)
			}
		}
	}()
	cmd, err := projectHTTPSGit(ctx, path, remote, "clone", "--origin", "origin", "--template=", "--no-recurse-submodules", "--", remote.URL, ".")
	if err != nil {
		return "", err
	}
	if err := cmd.Run(); err != nil {
		if errors.Is(ctx.Err(), context.DeadlineExceeded) {
			return "", errors.New("克隆超时，请检查服务器网络后重试。")
		}
		return "", errors.New("Git 克隆失败。请核对仓库链接、服务器网络和访问权限。")
	}
	repo, err := localrepo.ResolveRepo(ctx, c.localRoot, path)
	if err != nil {
		return "", errors.New("克隆结果没有通过 Git 工作树检查。")
	}
	origin, err := localrepo.Git(ctx, repo, "remote", "get-url", "origin")
	if err != nil || strings.TrimSpace(string(origin)) != remote.URL {
		return "", errors.New("克隆结果的远程地址与接入请求不一致。")
	}
	ready = true
	return repo, nil
}

func projectHTTPSGit(ctx context.Context, dir string, remote cloneRemote, args ...string) (*exec.Cmd, error) {
	opts := []string{"-c", "protocol.https.allow=always", "-c", "http.followRedirects=false",
		"-c", "http.lowSpeedLimit=1", "-c", "http.lowSpeedTime=60", "-c", "credential.helper=", "-c", "credential.interactive=false"}
	if remote.credential != nil {
		// Git invokes the scoped helper on an authentication challenge. Values
		// come from the child environment and never become shell source or argv.
		const helper = `!f() { if test "$1" = get; then printf 'username=%s\npassword=%s\n' "$FORGEOS_REPO_USER" "$FORGEOS_REPO_TOKEN"; fi; }; f`
		opts = append(opts, "-c", "credential.useHttpPath=true", "-c", "credential."+remote.URL+".helper="+helper)
	} else if remote.Host == "github.com" {
		opts = append(opts, "-c", "credential.https://github.com.helper=!gh auth git-credential")
	}
	cmd := localrepo.GitCommand(ctx, dir, append(opts, args...)...)
	if remote.credential != nil {
		cmd.Env = append(cmd.Env, "FORGEOS_REPO_USER="+remote.credential.username, "FORGEOS_REPO_TOKEN="+remote.credential.token, "GIT_ASKPASS=", "SSH_ASKPASS=")
	}
	proxy, err := cloneProxy(ctx, dir, remote.URL)
	if err != nil {
		return nil, err
	}
	if proxy != "" {
		cmd.Env = append(cmd.Env, "GIT_CONFIG_COUNT=1", "GIT_CONFIG_KEY_0=http.proxy", "GIT_CONFIG_VALUE_0="+proxy)
	}
	return cmd, nil
}

func cloneProxy(ctx context.Context, dir, target string) (string, error) {
	proxy := strings.TrimSpace(os.Getenv("FORGEOS_GIT_CLONE_PROXY"))
	if proxy == "" {
		// Respect the service account's existing proxy without loading its other
		// Git options, includes, URL rewrites or credential helpers into clone.
		if home, err := os.UserHomeDir(); err == nil {
			value, err := localrepo.Git(ctx, dir, "config", "--file", filepath.Join(home, ".gitconfig"),
				"--get-urlmatch", "http.proxy", target)
			if err == nil {
				proxy = strings.TrimSpace(string(value))
			}
		}
	}
	if proxy == "" {
		return "", nil
	}
	if !strings.Contains(proxy, "://") {
		proxy = "http://" + proxy
	}
	parsed, err := url.Parse(proxy)
	if err != nil || len(proxy) > 2048 || parsed.Hostname() == "" || parsed.RawQuery != "" ||
		parsed.Fragment != "" || (parsed.Path != "" && parsed.Path != "/") ||
		!slices.Contains([]string{"http", "https", "socks4", "socks4a", "socks5", "socks5h"}, parsed.Scheme) {
		return "", errors.New("克隆代理地址无效，请检查 ForgeOS 服务账号的 Git 代理设置。")
	}
	return proxy, nil
}
