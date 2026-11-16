package main

import (
	"errors"
	"net/http"
	"strconv"
	"strings"

	"forgeos/internal/store"
)

func registerManagement(mux *http.ServeMux, data *store.Store, project, token, owner string, workers workerAccess) error {
	if project == "" && token == "" {
		return nil
	}
	id, err := strconv.ParseInt(project, 10, 64)
	if err != nil || id <= 0 || strings.TrimSpace(token) == "" || owner == token || workers.match(token) != nil {
		return errors.New("set FORGEOS_MANAGEMENT_PROJECT_ID and a distinct FORGEOS_MANAGEMENT_TOKEN")
	}
	mux.HandleFunc("POST /internal/management/claim", authorized(token, func(w http.ResponseWriter, r *http.Request) {
		var capabilities struct {
			Shares bool `json:"supports_shares"`
		}
		if r.ContentLength != 0 && !decode(w, r, &capabilities) {
			return
		}
		job, err := data.ClaimManagement(r.Context(), id)
		if err == nil && job == nil && capabilities.Shares {
			job, err = data.ClaimShareReview(r.Context(), id)
		}
		if err == nil && job == nil {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		respond(w, http.StatusOK, job, err)
	}))
	mux.HandleFunc("POST /internal/management/result", authorized(token, func(w http.ResponseWriter, r *http.Request) {
		var input struct {
			ShareID  int64                  `json:"share_id"`
			TaskID   int64                  `json:"task_id"`
			Revision int                    `json:"revision"`
			Lease    string                 `json:"lease"`
			Review   store.ManagementReview `json:"review"`
		}
		if !decode(w, r, &input) {
			return
		}
		var err error
		if input.ShareID != 0 {
			if input.TaskID != 0 || input.Revision != 0 || input.ShareID < 1 {
				problem(w, 400, "ambiguous management review target")
				return
			}
			err = data.ReviewShare(r.Context(), id, input.ShareID, input.Lease, input.Review)
		} else {
			err = data.CompleteManagement(r.Context(), id, input.TaskID, input.Revision, input.Lease, input.Review)
		}
		respond(w, http.StatusNoContent, nil, err)
	}))
	return nil
}
