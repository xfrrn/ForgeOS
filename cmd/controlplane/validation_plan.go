package main

import (
	"net/http"

	"forgeos/internal/localrepo"
	"forgeos/internal/store"
	"forgeos/internal/validation"
)

func registerValidationPlan(mux *http.ServeMux, data *store.Store, root, owner string, workers workerAccess) {
	registerProjectRead(mux, "validation-plan", owner, workers, func(w http.ResponseWriter, r *http.Request, id int64) {
		project, err := data.Project(r.Context(), id)
		if err != nil {
			respond(w, 0, nil, err)
			return
		}
		repo, err := localrepo.ResolveRepo(r.Context(), root, project.Path)
		if err != nil {
			problem(w, 409, "registered repository is unavailable")
			return
		}
		q := r.URL.Query()
		plan, err := validation.ReadPlan(r.Context(), repo, q.Get("commit"), q.Get("level"))
		if err != nil {
			detail := err.Error()
			if len(detail) > 2048 {
				detail = "validation configuration does not satisfy the schema"
			}
			problem(w, http.StatusUnprocessableEntity, detail)
			return
		}
		respond(w, 200, plan, nil)
	})
}
