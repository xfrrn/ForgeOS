package main

import (
	"context"
	"errors"
	"net/http"
	"os"
	"strings"
	"time"
	"unicode"

	"forgeos/internal/bao"
	"forgeos/internal/localrepo"
	"forgeos/internal/store"
)

var errCredentialScope = errors.New("所选凭据已停用或不适用于这个仓库。")
var errCredentialVault = errors.New("无法访问仓库凭据存储，请检查 OpenBao 配置和权限。")

// A repository target includes its full path. An account target is exactly the
// allowed HTTPS origin; only an explicit account scope can create one.
func credentialTarget(scope, raw, extraHosts string) (string, error) {
	switch scope {
	case "", "repository":
		remote, err := parseCloneRemote(raw, extraHosts)
		return remote.URL, err
	case "account":
		host := strings.ToLower(strings.TrimSpace(raw))
		host = strings.TrimSuffix(strings.TrimPrefix(host, "https://"), "/")
		host = strings.TrimSuffix(host, ":443")
		if err := validateCloneHost(host, extraHosts); err != nil {
			if errors.Is(err, errCloneURL) {
				return "", errors.New("请填写 Git 平台域名，例如 github.com。")
			}
			return "", err
		}
		return "https://" + host, nil
	default:
		return "", errors.New("请选择单个仓库或 Git 账号作为适用范围。")
	}
}

func (c projectCloneService) checkCredential(ctx context.Context, id int64, remote cloneRemote) (store.RepositoryCredential, error) {
	meta, err := c.data.RepositoryCredential(ctx, id)
	if err != nil {
		return meta, err
	}
	if !meta.Enabled || (meta.URL != remote.URL && meta.URL != "https://"+remote.Host) {
		return meta, errCredentialScope
	}
	return meta, nil
}

func (c projectCloneService) authenticateRemote(ctx context.Context, id int64, remote *cloneRemote) error {
	remote.credential = nil
	if id == 0 {
		return nil
	}
	meta, err := c.checkCredential(ctx, id, *remote)
	if err != nil {
		return err
	}
	token, err := c.credentials.RepositoryToken(ctx, meta.VaultKey, meta.Version)
	if err != nil {
		return errCredentialVault
	}
	remote.credential = &gitCredential{username: meta.Username, token: token}
	return nil
}

func credentialProblem(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, errCredentialVault), errors.Is(err, bao.ErrUnavailable):
		problem(w, 503, errCredentialVault.Error())
	case errors.Is(err, errCredentialScope):
		problem(w, 409, err.Error())
	default:
		respond(w, 0, nil, err)
	}
}

// credentialUpstream resolves the same branch and allowed remote used by sync.
func (c projectCloneService) credentialUpstream(ctx context.Context, project store.Project) (cloneRemote, error) {
	repo, err := localrepo.ResolveRepo(ctx, c.localRoot, project.Path)
	if err != nil {
		return cloneRemote{}, errors.New("本地 Git 仓库不可用。")
	}
	commit, err := readProjectCommit(ctx, repo)
	if err != nil {
		return cloneRemote{}, err
	}
	upstream, err := findProjectUpstream(ctx, repo, commit.Branch)
	return upstream.remote, err
}

func validRepositoryToken(token string) bool {
	return len(token) > 0 && len(token) <= 4096 && strings.IndexFunc(token, func(r rune) bool { return unicode.IsSpace(r) || unicode.IsControl(r) }) < 0
}

func (c projectCloneService) registerCredentials(mux *http.ServeMux, owner string) {
	const path = "/projects/repository-credentials"
	mux.HandleFunc("GET "+path, authorized(owner, func(w http.ResponseWriter, r *http.Request) {
		items, err := c.data.RepositoryCredentials(r.Context())
		respond(w, 200, map[string]any{"items": items, "configured": c.credentials != nil}, err)
	}))
	mux.HandleFunc("POST "+path, authorized(owner, func(w http.ResponseWriter, r *http.Request) {
		var input struct{ Name, Scope, URL, Username, Token string }
		if !decode(w, r, &input) {
			return
		}
		target, err := credentialTarget(input.Scope, input.URL, c.extraHosts)
		if err != nil {
			problem(w, 400, err.Error())
			return
		}
		name, username := strings.TrimSpace(input.Name), strings.TrimSpace(input.Username)
		if name == "" || len(name) > 160 || strings.IndexFunc(name, unicode.IsControl) >= 0 || username == "" || len(username) > 256 || strings.Contains(username, ":") || strings.IndexFunc(username, unicode.IsControl) >= 0 || !validRepositoryToken(input.Token) {
			problem(w, 400, "请填写有效的凭据名称、Git 用户名和访问令牌。")
			return
		}
		if c.credentials == nil {
			credentialProblem(w, errCredentialVault)
			return
		}
		id, err := c.data.ReserveRepositoryCredential(r.Context(), store.RepositoryCredential{Name: name, URL: target, Username: username})
		if errors.Is(err, store.ErrConflict) {
			problem(w, 409, "该范围已有同名凭据，可在列表中更换令牌。")
			return
		}
		if err != nil {
			credentialProblem(w, err)
			return
		}
		if err = c.saveRepositoryToken(r.Context(), id, input.Token); err != nil {
			problem(w, 503, "凭据已登记，令牌尚未确认保存。请刷新列表并检查 OpenBao 后重试。")
			return
		}
		respond(w, 201, map[string]int64{"id": id}, nil)
	}))
	mux.HandleFunc("PUT "+path, authorized(owner, func(w http.ResponseWriter, r *http.Request) {
		var input struct {
			ID            int64
			Action, Token string
		}
		if !decode(w, r, &input) {
			return
		}
		var err error
		switch {
		case input.ID > 0 && input.Action == "disable" && input.Token == "":
			err = c.data.DisableRepositoryCredential(r.Context(), input.ID)
		case input.ID > 0 && input.Action == "replace" && validRepositoryToken(input.Token):
			err = c.saveRepositoryToken(r.Context(), input.ID, input.Token)
		default:
			problem(w, 400, "请选择凭据并检查令牌。")
			return
		}
		if err != nil {
			credentialProblem(w, err)
			return
		}
		w.WriteHeader(204)
	}))
	mux.HandleFunc("POST /projects/clone-check", authorizedFor(owner, 20*time.Second, func(w http.ResponseWriter, r *http.Request) {
		var input struct {
			URL          string
			CredentialID int64 `json:"credential_id"`
		}
		if !decode(w, r, &input) {
			return
		}
		remote, err := parseCloneRemote(input.URL, c.extraHosts)
		if err != nil {
			problem(w, 400, err.Error())
			return
		}
		if err = c.authenticateRemote(r.Context(), input.CredentialID, &remote); err != nil {
			credentialProblem(w, err)
			return
		}
		dir, err := os.MkdirTemp("", "forgeos-check-")
		if err != nil {
			problem(w, 500, "无法创建 Git 操作临时目录。")
			return
		}
		defer os.RemoveAll(dir)
		cmd, err := projectHTTPSGit(r.Context(), dir, remote, "ls-remote", "--", remote.URL, "HEAD")
		if err != nil || cmd.Run() != nil {
			problem(w, 502, "无法读取仓库，请检查仓库链接、访问权限和控制面网络。")
			return
		}
		w.WriteHeader(204)
	}))
	const identity = "/projects/{id}/git/identity"
	mux.HandleFunc("GET "+identity, authorized(owner, func(w http.ResponseWriter, r *http.Request) {
		id, ok := pathID(w, r)
		if !ok {
			return
		}
		project, err := c.data.Project(r.Context(), id)
		if err != nil {
			respond(w, 0, nil, err)
			return
		}
		// A missing checkout still permits switching back to the local identity.
		upstream, _ := c.credentialUpstream(r.Context(), project)
		respond(w, 200, map[string]any{"credential_id": project.RepositoryCredentialID, "url": upstream.URL}, nil)
	}))
	mux.HandleFunc("PUT "+identity, authorized(owner, func(w http.ResponseWriter, r *http.Request) {
		id, ok := pathID(w, r)
		if !ok {
			return
		}
		var input struct {
			CredentialID *int64 `json:"credential_id"`
		}
		if !decode(w, r, &input) {
			return
		}
		if input.CredentialID == nil || *input.CredentialID < 0 {
			problem(w, 400, "请选择仓库身份。")
			return
		}
		if *input.CredentialID != 0 {
			project, err := c.data.Project(r.Context(), id)
			if err != nil {
				respond(w, 0, nil, err)
				return
			}
			remote, err := c.credentialUpstream(r.Context(), project)
			if err != nil {
				problem(w, 409, err.Error())
				return
			}
			if _, err = c.checkCredential(r.Context(), *input.CredentialID, remote); err != nil {
				credentialProblem(w, err)
				return
			}
		}
		respond(w, 204, nil, c.data.SetProjectRepositoryIdentity(r.Context(), id, *input.CredentialID))
	}))
}

func (c projectCloneService) saveRepositoryToken(ctx context.Context, id int64, token string) error {
	return c.data.RotateRepositoryCredential(ctx, id, func(key string) (int, error) {
		return c.credentials.SetRepositoryToken(ctx, key, token)
	})
}
