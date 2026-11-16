package main

import (
	"errors"
	"net/http"
	"strconv"
	"strings"

	"forgeos/internal/store"
)

func registerManagement(mux *http.ServeMux, data *store.Store, project, token, owner, worker string) error {
	if project == "" && token == "" {
		return nil
	}
	id, err := strconv.ParseInt(project, 10, 64)
	if err != nil || id <= 0 || strings.TrimSpace(token) == "" || owner == token || worker == token {
		return errors.New("set FORGEOS_MANAGEMENT_PROJECT_ID and a distinct FORGEOS_MANAGEMENT_TOKEN")
	}
	mux.HandleFunc("POST /internal/management/claim", authorized(token, func(w http.ResponseWriter, r *http.Request) {
		job, err := data.ClaimManagement(r.Context(), id)
		if err == nil && job == nil {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		respond(w, http.StatusOK, job, err)
	}))
	mux.HandleFunc("POST /internal/management/result", authorized(token, func(w http.ResponseWriter, r *http.Request) {
		var input struct {
			TaskID   int64                  `json:"task_id"`
			Revision int                    `json:"revision"`
			Lease    string                 `json:"lease"`
			Review   store.ManagementReview `json:"review"`
		}
		if !decode(w, r, &input) {
			return
		}
		err := data.CompleteManagement(r.Context(), id, input.TaskID, input.Revision, input.Lease, input.Review)
		respond(w, http.StatusNoContent, nil, err)
	}))
	return nil
}
