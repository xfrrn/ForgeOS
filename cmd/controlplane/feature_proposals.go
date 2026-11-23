package main

import (
	"net/http"
	"strconv"
	"strings"

	"forgeos/internal/localrepo"
	"forgeos/internal/store"
)

func registerFeatureProposals(mux *http.ServeMux, chat projectChat, owner string, nodes chatNodes) {
	const path = "/projects/{id}/features/{feature}/proposals"
	handle := func(pattern string, action func(http.ResponseWriter, *http.Request, int64, int64, int64)) {
		mux.HandleFunc(pattern, authorized(owner, func(w http.ResponseWriter, r *http.Request) {
			project, ok := pathID(w, r)
			if !ok {
				return
			}
			feature, ok := requestedFeature(w, r, chat.data, project)
			if !ok {
				return
			}
			var round int64
			if raw := r.PathValue("round"); raw != "" {
				var err error
				round, err = strconv.ParseInt(raw, 10, 64)
				if err != nil || round < 1 {
					problem(w, 400, "无效方案轮次")
					return
				}
			}
			action(w, r, project, feature, round)
		}))
	}
	handle("GET "+path, func(w http.ResponseWriter, r *http.Request, p, f, _ int64) {
		list, err := chat.data.FeatureProposalList(r.Context(), p, f)
		respond(w, 200, list, err)
	})
	handle("GET "+path+"/{round}", func(w http.ResponseWriter, r *http.Request, p, f, id int64) {
		detail, err := chat.data.FeatureProposal(r.Context(), p, f, id)
		respond(w, 200, detail, err)
	})
	handle("POST "+path, func(w http.ResponseWriter, r *http.Request, p, f, _ int64) {
		var in store.ProposalRequest
		if !decodeLimited(w, r, &in, 32<<10) {
			return
		}
		if _, ready := nodes[p]; !ready {
			problem(w, 409, "请先为此项目连接讨论节点")
			return
		}
		ctx := r.Context()
		project, err := chat.data.Project(ctx, p)
		if err != nil {
			respond(w, 0, nil, err)
			return
		}
		repo, err := localrepo.ResolveRepo(ctx, chat.root, project.Path)
		if err != nil {
			problem(w, 409, "无法读取项目仓库，请检查本地路径")
			return
		}
		sha, err := localrepo.Git(ctx, repo, "rev-parse", "--verify", "HEAD")
		if err != nil {
			problem(w, 409, "无法冻结仓库版本，请确认项目有可读取的提交")
			return
		}
		id, err := chat.data.StartFeatureProposal(ctx, p, f, in, strings.TrimSpace(string(sha)))
		respond(w, 201, map[string]int64{"id": id}, err)
	})
	handle("GET "+path+"/{round}/delivery", func(w http.ResponseWriter, r *http.Request, p, f, id int64) {
		receipt, err := chat.data.DesignReceipt(r.Context(), p, f, id)
		respond(w, 200, receipt, err)
	})
	handle("POST "+path+"/{round}/dispatch", chat.dispatchDesign)
	handle("POST "+path+"/{round}/revise-step", func(w http.ResponseWriter, r *http.Request, p, f, id int64) {
		var in struct {
			Task      int64
			Execution int64
			Note      string
		}
		if !decodeLimited(w, r, &in, 8192) {
			return
		}
		err := chat.data.ReviseFeatureStep(r.Context(), p, f, id, in.Task, in.Execution, in.Note)
		respond(w, 200, map[string]bool{"queued": err == nil}, err)
	})
	handle("POST "+path+"/{round}/actions", func(w http.ResponseWriter, r *http.Request, p, f, id int64) {
		var in store.ProposalAction
		if !decodeLimited(w, r, &in, 64<<10) {
			return
		}
		err := chat.data.ActOnProposal(r.Context(), p, f, id, in)
		respond(w, 204, nil, err)
	})
}
