package main

import (
	"context"
	"errors"
	"io/fs"
	"net/http"
	"strings"
	"unicode"

	"forgeos/internal/localrepo"
	"forgeos/internal/store"
)

type documentProjects interface {
	Project(context.Context, int64) (store.Project, error)
}

func documentRepository(w http.ResponseWriter, r *http.Request, data documentProjects, root string, id int64) (string, bool) {
	project, err := data.Project(r.Context(), id)
	if err != nil {
		respond(w, 0, nil, err)
		return "", false
	}
	repo, err := localrepo.ResolveRepo(r.Context(), root, project.Path)
	if err != nil {
		problem(w, 409, "registered repository unavailable")
		return "", false
	}
	return repo, true
}

func registerDocumentBrowser(mux *http.ServeMux, data documentProjects, root, owner string, workers *workerRegistry) {
	resolve := func(w http.ResponseWriter, r *http.Request, id int64) (string, string, bool) {
		q := r.URL.Query()
		revision := q.Get("commit")
		if q.Has("commit") && !gitCommit.MatchString(revision) {
			problem(w, 400, "请提供完整的 Git 提交 SHA。")
			return "", "", false
		}
		repo, ok := documentRepository(w, r, data, root, id)
		if !ok {
			return "", "", false
		}
		commit, err := localrepo.DocumentRevision(r.Context(), repo, revision)
		if errors.Is(err, localrepo.ErrDocumentRevision) {
			problem(w, 422, "该提交不在当前仓库 HEAD 的历史中。")
			return "", "", false
		}
		if err != nil {
			problem(w, 409, "仓库当前没有可读的 HEAD 提交。")
			return "", "", false
		}
		return repo, commit, true
	}
	registerProjectRead(mux, "documents/tree", owner, workers, func(w http.ResponseWriter, r *http.Request, id int64) {
		repo, commit, ok := resolve(w, r, id)
		if !ok {
			return
		}
		files, err := localrepo.DocumentFiles(r.Context(), repo, commit)
		if err != nil {
			problem(w, 422, "无法读取该提交的完整文档目录。")
			return
		}
		var branch string
		if !r.URL.Query().Has("commit") {
			ref, err := localrepo.Git(r.Context(), repo, "symbolic-ref", "--quiet", "HEAD")
			if err == nil {
				name := strings.TrimSpace(string(ref))
				tip, err := localrepo.Git(r.Context(), repo, "rev-parse", "--verify", name)
				if err == nil && strings.TrimSpace(string(tip)) == commit {
					branch = strings.TrimPrefix(name, "refs/heads/")
				}
			}
		}
		writeJSON(w, 200, struct {
			Commit    string                  `json:"commit"`
			Branch    string                  `json:"branch,omitempty"`
			Documents []localrepo.TrackedBlob `json:"documents"`
		}{commit, branch, files})
	})
	registerProjectRead(mux, "documents/read", owner, workers, func(w http.ResponseWriter, r *http.Request, id int64) {
		name := r.URL.Query().Get("path")
		if name == "." || !fs.ValidPath(name) || strings.IndexFunc(name, unicode.IsControl) >= 0 {
			problem(w, 400, "请提供仓库内的文档相对路径。")
			return
		}
		repo, commit, ok := resolve(w, r, id)
		if !ok {
			return
		}
		files, err := localrepo.DocumentFiles(r.Context(), repo, commit, name)
		if err != nil {
			problem(w, 422, "无法读取该提交中的文档路径。")
			return
		}
		if len(files) != 1 || files[0].Path != name {
			problem(w, 404, "该提交的文档目录中没有此路径。")
			return
		}
		content, issue, err := localrepo.ReadDocumentText(r.Context(), repo, files[0].Blob, 8<<20)
		switch {
		case err != nil:
			problem(w, 422, "无法读取该提交中的文档。")
		case issue == "size_limit":
			problem(w, 413, "文档超过 8 MiB 预览上限。")
		case issue == "non_text":
			problem(w, 415, "此文档不是可预览的 UTF-8 文本。")
		default:
			writeJSON(w, 200, struct {
				Commit string `json:"commit"`
				localrepo.TrackedBlob
				Content string `json:"content"`
			}{commit, files[0], content})
		}
	})
}
