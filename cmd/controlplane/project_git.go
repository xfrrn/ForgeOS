package main

import (
	"context"
	"errors"
	"net/http"
	"os"
	"strings"
	"sync"
	"time"

	"forgeos/internal/localrepo"
)

type projectUpstream struct {
	remote      cloneRemote
	name        string
	ref         string
	trackingRef string
}

func (c projectCloneService) registerProjectGit(mux *http.ServeMux, owner string) {
	var pulls sync.Map
	for _, route := range []struct{ method, action string }{{"GET", "local"}, {"GET", "remote"}, {"POST", "pull"}} {
		mux.HandleFunc(route.method+" /projects/{id}/git/"+route.action, authorizedFor(owner, 90*time.Second, func(w http.ResponseWriter, r *http.Request) {
			id, ok := pathID(w, r)
			if !ok {
				return
			}
			ctx := r.Context()
			project, err := c.data.Project(ctx, id)
			if err != nil {
				respond(w, 0, nil, err)
				return
			}
			repo, err := localrepo.ResolveRepo(ctx, c.localRoot, project.Path)
			if err != nil {
				problem(w, http.StatusConflict, "项目的本地 Git 仓库不可用。")
				return
			}
			_ = http.NewResponseController(w).SetWriteDeadline(time.Now().Add(95 * time.Second))
			if route.action == "pull" {
				if _, busy := pulls.LoadOrStore(repo, struct{}{}); busy {
					problem(w, http.StatusConflict, "该项目正在拉取，请稍后刷新。")
					return
				}
				defer pulls.Delete(repo)
			}
			local, err := readProjectCommit(ctx, repo)
			if err != nil {
				problem(w, http.StatusConflict, err.Error())
				return
			}
			if route.action == "local" {
				respond(w, http.StatusOK, local, nil)
				return
			}
			if route.action == "pull" {
				if err := projectPullReady(ctx, repo, local); err != nil {
					problem(w, http.StatusConflict, err.Error())
					return
				}
			}
			upstream, err := findProjectUpstream(ctx, repo, local.Branch)
			if err != nil {
				problem(w, http.StatusConflict, err.Error())
				return
			}
			if err = c.authenticateRemote(ctx, project.RepositoryCredentialID, &upstream.remote); err != nil {
				credentialProblem(w, err)
				return
			}
			tmp, err := os.MkdirTemp("", "forgeos-project-git-")
			if err != nil {
				problem(w, http.StatusInternalServerError, "无法创建 Git 操作临时目录。")
				return
			}
			defer os.RemoveAll(tmp)
			remote, err := readProjectRemote(ctx, tmp, upstream)
			if err != nil {
				problem(w, http.StatusBadGateway, err.Error())
				return
			}
			if route.action == "remote" {
				respond(w, http.StatusOK, remote, nil)
				return
			}
			if remote.SHA != local.SHA {
				if err := fetchProjectCommit(ctx, repo, tmp, upstream.remote, remote.SHA, local.SHA); err != nil {
					problem(w, http.StatusBadGateway, err.Error())
					return
				}
			}
			if upstream.trackingRef != "" {
				if _, err := localrepo.Git(ctx, repo, "update-ref", upstream.trackingRef, remote.SHA); err != nil {
					problem(w, http.StatusConflict, "已读取远端提交，但无法更新本地远端跟踪记录。")
					return
				}
			}
			updated, err := fastForwardProject(ctx, repo, local, remote.SHA)
			if err != nil {
				problem(w, http.StatusConflict, err.Error())
				return
			}
			after, err := readProjectCommit(ctx, repo)
			if err != nil {
				problem(w, http.StatusConflict, err.Error())
				return
			}
			respond(w, http.StatusOK, struct {
				Local   projectCommit `json:"local"`
				Remote  projectCommit `json:"remote"`
				Updated bool          `json:"updated"`
			}{after, remote, updated}, nil)
		}))
	}
}

func findProjectUpstream(ctx context.Context, repo, branch string) (projectUpstream, error) {
	if branch == "" {
		return projectUpstream{}, errors.New("请先在本地切换到要跟踪远端的分支。")
	}
	out, err := localrepo.Git(ctx, repo, "for-each-ref", "--format=%(upstream:remotename)%00%(upstream:remoteref)%00%(upstream)", "refs/heads/"+branch)
	if err != nil {
		return projectUpstream{}, errors.New("无法读取当前分支的远端配置。")
	}
	parts := strings.Split(strings.TrimSpace(string(out)), "\x00")
	if len(parts) != 3 {
		return projectUpstream{}, errors.New("无法读取当前分支的远端配置。")
	}
	name, ref, tracking := parts[0], parts[1], parts[2]
	if name == "" && ref == "" {
		name, ref = "origin", "refs/heads/"+branch
	}
	if name == "" || !strings.HasPrefix(ref, "refs/heads/") {
		return projectUpstream{}, errors.New("当前分支没有可用的远端分支。")
	}
	if _, err := localrepo.Git(ctx, repo, "check-ref-format", ref); err != nil {
		return projectUpstream{}, errors.New("远端分支名称无效。")
	}
	url, err := localrepo.Git(ctx, repo, "config", "--get", "remote."+name+".url")
	if err != nil {
		return projectUpstream{}, errors.New("当前仓库没有可用的远端地址。")
	}
	remote, err := parseCloneRemote(string(url), os.Getenv("FORGEOS_GIT_CLONE_HOSTS"))
	if err != nil {
		return projectUpstream{}, errors.New("远端地址无法用于仓库同步。请使用 ForgeOS 支持的 Git 服务。")
	}
	if !strings.HasPrefix(tracking, "refs/remotes/") {
		tracking = ""
	}
	return projectUpstream{remote: remote, name: name, ref: ref, trackingRef: tracking}, nil
}

func readProjectRemote(ctx context.Context, dir string, upstream projectUpstream) (projectCommit, error) {
	cmd, err := projectHTTPSGit(ctx, dir, upstream.remote, "ls-remote", "--refs", "--", upstream.remote.URL, upstream.ref)
	if err != nil {
		return projectCommit{}, err
	}
	out, err := cmd.Output()
	sha, found, ok := strings.Cut(strings.TrimSpace(string(out)), "\t")
	if err != nil {
		return projectCommit{}, errors.New("无法读取远端提交，请检查网络和仓库访问权限。")
	}
	if !ok || found != upstream.ref || !gitCommit.MatchString(sha) {
		return projectCommit{}, errors.New("远端没有找到当前分支对应的提交。")
	}
	return projectCommit{Branch: strings.TrimPrefix(found, "refs/heads/"), SHA: sha, Remote: upstream.name}, nil
}

func projectPullReady(ctx context.Context, repo string, local projectCommit) error {
	if local.Branch == "" {
		return errors.New("请先在本地切换到要更新的分支。")
	}
	out, err := localrepo.Git(ctx, repo, "status", "--porcelain", "--untracked-files=normal")
	if err != nil || len(out) != 0 {
		return errors.New("请先提交本地改动或用 Git stash 收起改动，再拉取远端更新。")
	}
	return nil
}

func fetchProjectCommit(ctx context.Context, repo, dir string, remote cloneRemote, sha, local string) error {
	if len(sha) != len(local) {
		return errors.New("本地与远端仓库使用了不同的 Git 对象格式。")
	}
	format := "sha1"
	if len(sha) == 64 {
		format = "sha256"
	}
	objects, err := localrepo.Git(ctx, repo, "rev-parse", "--path-format=absolute", "--git-path", "objects")
	if err != nil || len(strings.TrimSpace(string(objects))) == 0 {
		return errors.New("无法读取项目的 Git 对象目录。")
	}
	if _, err := localrepo.Git(ctx, dir, "init", "--bare", "--template=", "--object-format="+format, "."); err != nil {
		return errors.New("无法初始化临时 Git 对象库。")
	}
	cmd, err := projectHTTPSGit(ctx, dir, remote, "fetch", "--no-tags", "--no-recurse-submodules",
		"--no-write-fetch-head", "--no-auto-maintenance", "--", remote.URL, sha)
	if err != nil {
		return err
	}
	cmd.Env = append(cmd.Env, "GIT_OBJECT_DIRECTORY="+strings.TrimSpace(string(objects)))
	if err := cmd.Run(); err != nil {
		return errors.New("无法拉取选定的远端提交，请检查网络和仓库访问权限。")
	}
	return nil
}

func fastForwardProject(ctx context.Context, repo string, before projectCommit, target string) (bool, error) {
	current, err := readProjectCommit(ctx, repo)
	if err != nil || current.SHA != before.SHA || current.Branch != before.Branch {
		return false, errors.New("本地分支在拉取期间已变化，请刷新后重试。")
	}
	if err := projectPullReady(ctx, repo, current); err != nil {
		return false, err
	}
	if current.SHA == target {
		return false, nil
	}
	if _, err := localrepo.Git(ctx, repo, "merge-base", "--is-ancestor", target, current.SHA); err == nil {
		return false, nil
	}
	if _, err := localrepo.Git(ctx, repo, "merge-base", "--is-ancestor", current.SHA, target); err != nil {
		return false, errors.New("本地与远端分支已分叉，或历史不完整。请在本地核对后再同步。")
	}
	if _, err := localrepo.Git(ctx, repo, "-c", "submodule.recurse=false", "merge", "--ff-only", "--no-edit", "--no-autostash", target); err != nil {
		return false, errors.New("无法快进本地分支，请检查工作树状态。")
	}
	return true, nil
}
