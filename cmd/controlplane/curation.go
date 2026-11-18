package main

import (
	"errors"
	"net/http"
	"os"
	"slices"
	"strconv"
	"strings"
	"unicode/utf8"

	"forgeos/internal/store"
)

func registerCurator(mux *http.ServeMux, data *store.Store, owner, manager string, workers *workerRegistry) error {
	scope, token := os.Getenv("FORGEOS_CURATOR_PROJECT_ID"), os.Getenv("FORGEOS_CURATOR_TOKEN")
	if scope == "" && token == "" {
		return nil
	}
	id, err := strconv.ParseInt(scope, 10, 64)
	if err != nil || id < 1 || strings.TrimSpace(token) == "" || workers.match(token) != nil ||
		slices.Contains([]string{owner, manager, os.Getenv("FORGEOS_TEST_AGENT_TOKEN"), os.Getenv("FORGEOS_REVIEWER_TOKEN")}, token) {
		return errors.New("set FORGEOS_CURATOR_PROJECT_ID and a distinct FORGEOS_CURATOR_TOKEN")
	}
	mux.HandleFunc("POST /internal/curator/claim", authorized(token, idempotentRoleClaim(data, id, "curator", func(w http.ResponseWriter, r *http.Request) {
		if r.ContentLength != 0 && !decode(w, r, &struct{}{}) {
			return
		}
		job, err := data.ClaimCuration(r.Context(), id, false, r.URL.Query().Get("supports_budget_deadline") == "1")
		if job != nil {
			job.CaptureProcess = r.URL.Query().Get("supports_process") == "1"
		}
		if job == nil && err == nil {
			w.WriteHeader(204)
			return
		}
		respond(w, 200, job, err)
	})))
	mux.HandleFunc("POST /internal/curator/process", authorized(token, func(w http.ResponseWriter, r *http.Request) {
		var report store.ManagementProcessReport
		if decodeLimited(w, r, &report, 64<<10) {
			if report.CurationID <= 0 || report.CurationRole != "curator" {
				problem(w, 403, "Curator identity only reports extraction processes")
				return
			}
			respond(w, 204, nil, data.ObserveManagementProcess(r.Context(), id, report))
		}
	}))
	mux.HandleFunc("POST /internal/curator/result", authorized(token, func(w http.ResponseWriter, r *http.Request) {
		var out struct {
			ID         int64            `json:"id"`
			Lease      string           `json:"lease"`
			Extraction store.Extraction `json:"extraction"`
		}
		if decodeLimited(w, r, &out, 256<<10) {
			respond(w, 204, nil, data.RecordExtraction(r.Context(), id, out.ID, out.Lease, out.Extraction))
		}
	}))
	return nil
}

func registerCurationReview(mux *http.ServeMux, data *store.Store, project int64, token string) {
	mux.HandleFunc("POST /internal/management/curation/result", authorized(token, func(w http.ResponseWriter, r *http.Request) {
		var out struct {
			ID     int64                  `json:"id"`
			Lease  string                 `json:"lease"`
			Review store.ManagementReview `json:"review"`
		}
		if decodeLimited(w, r, &out, 64<<10) {
			respond(w, 204, nil, data.ReviewCuration(r.Context(), project, out.ID, out.Lease, out.Review))
		}
	}))
}

func registerCurationReads(mux *http.ServeMux, data *store.Store, owner string, workers *workerRegistry) {
	mux.HandleFunc("GET /projects/{id}/tasks/{task}/events/{event}/curation", authorized(owner, func(w http.ResponseWriter, r *http.Request) {
		project, ok := pathID(w, r)
		if !ok {
			return
		}
		task, taskErr := strconv.ParseInt(r.PathValue("task"), 10, 64)
		event, eventErr := strconv.ParseInt(r.PathValue("event"), 10, 64)
		if taskErr != nil || eventErr != nil || task < 1 || event < 1 {
			problem(w, 400, "invalid task or event id")
			return
		}
		out, err := data.EventCuration(r.Context(), project, task, event)
		respond(w, 200, out, err)
	}))
	mux.HandleFunc("GET /projects/{id}/decisions/{decision}/curation", authorized(owner, func(w http.ResponseWriter, r *http.Request) {
		project, ok := pathID(w, r)
		if !ok {
			return
		}
		decision, err := strconv.ParseInt(r.PathValue("decision"), 10, 64)
		if err != nil || decision < 1 {
			problem(w, 400, "invalid decision id")
			return
		}
		out, err := data.DecisionCuration(r.Context(), project, decision)
		respond(w, 200, out, err)
	}))
	mux.HandleFunc("GET /projects/{id}/materials/{key}/revisions/{revision}/curation", authorized(owner, func(w http.ResponseWriter, r *http.Request) {
		project, ok := pathID(w, r)
		if !ok {
			return
		}
		n, err := strconv.ParseInt(r.PathValue("revision"), 10, 32)
		key := r.PathValue("key")
		if err != nil || n < 1 || !store.ValidMaterialKey(key) {
			problem(w, 400, "invalid material revision")
			return
		}
		out, err := data.MaterialCuration(r.Context(), project, key, int(n))
		respond(w, 200, out, err)
	}))
	registerProjectRead(mux, "experience", owner, workers, func(w http.ResponseWriter, r *http.Request, project int64) {
		query := strings.TrimSpace(r.URL.Query().Get("q"))
		if query != "" {
			if len(query) > 512 || !utf8.ValidString(query) || strings.ContainsRune(query, 0) || r.URL.Query().Has("before") {
				problem(w, http.StatusBadRequest, "invalid experience search query or cursor")
				return
			}
			out, err := data.SearchExperience(r.Context(), project, query)
			respond(w, http.StatusOK, out, err)
			return
		}
		var before int64
		if r.URL.Query().Has("before") {
			var err error
			before, err = strconv.ParseInt(r.URL.Query().Get("before"), 10, 64)
			if err != nil || before < 1 {
				problem(w, 400, "invalid experience cursor")
				return
			}
		}
		out, err := data.ProjectExperience(r.Context(), project, before)
		respond(w, 200, out, err)
	})
}
