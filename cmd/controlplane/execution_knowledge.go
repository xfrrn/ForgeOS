package main

import (
	"net/http"
	"strings"
	"time"
	"unicode"

	"forgeos/internal/store"
)

type executionKnowledgeRequest struct {
	LeaseToken string `json:"lease_token"`
	Action     string `json:"action"`
	Query      string `json:"query,omitempty"`
	Kind       string `json:"kind,omitempty"`
	Ref        string `json:"ref,omitempty"`
}

func (in executionKnowledgeRequest) valid() bool {
	if len(in.LeaseToken) != 64 {
		return false
	}
	switch in.Action {
	case "search":
		return in.Query != "" && len(in.Query) <= 512 && strings.IndexFunc(in.Query, unicode.IsControl) < 0 && in.Kind == "" && in.Ref == ""
	case "read", "cite":
		switch in.Kind {
		case "decision", "experience", "instruction", "material", "git", store.SharedKnowledgeKind:
			return in.Query == "" && in.Ref != "" && len(in.Ref) <= 32<<10 && strings.IndexFunc(in.Ref, unicode.IsControl) < 0
		}
	}
	return false
}

func registerExecutionKnowledge(mux *http.ServeMux, data *store.Store, root string, workers *workerRegistry) {
	const timeout = 5 * time.Minute
	workers.handle(mux, "POST /internal/executions/{id}/knowledge", workers.executionFor(data, timeout, func(w http.ResponseWriter, r *http.Request) {
		id, ok := pathID(w, r)
		var in executionKnowledgeRequest
		if !ok || !decodeLimited(w, r, &in, 64<<10) {
			return
		}
		in.Query = strings.TrimSpace(in.Query)
		if !in.valid() {
			problem(w, http.StatusBadRequest, "provide a lease and a knowledge search, read or citation request")
			return
		}
		ctx := r.Context()
		_ = http.NewResponseController(w).SetWriteDeadline(time.Now().Add(timeout + 5*time.Second))
		if in.Action == "cite" {
			snapshot, err := data.CiteKnowledge(ctx, id, in.LeaseToken, in.Kind, in.Ref)
			respond(w, http.StatusOK, map[string]any{"evidence": snapshot}, err)
			return
		}
		project, commit, err := data.KnowledgeScope(ctx, id, in.LeaseToken)
		if err != nil {
			respond(w, 0, nil, err)
			return
		}
		if in.Action == "read" {
			evidence, err := knowledgeRead(ctx, data, root, project, commit, in.Kind, in.Ref)
			if err == nil {
				evidence, err = data.RememberKnowledgeRead(ctx, id, in.LeaseToken, evidence)
			}
			respond(w, http.StatusOK, map[string]any{"evidence": evidence}, err)
			return
		}
		hits, searchErr := knowledgeSearch(ctx, data, project, in.Query)
		if _, _, err := data.KnowledgeScope(ctx, id, in.LeaseToken); err != nil {
			respond(w, 0, nil, err)
			return
		}
		if searchErr != nil && hits == nil {
			problem(w, http.StatusServiceUnavailable, "project knowledge search is temporarily unavailable")
			return
		}
		result := map[string]any{"hits": hits}
		if searchErr != nil {
			result["warning"] = "部分项目知识来源暂不可用，结果可能不完整。"
		}
		writeJSON(w, http.StatusOK, result)
	}))
}
