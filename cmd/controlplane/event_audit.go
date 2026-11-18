package main

import (
	"context"
	"encoding/json"
	"net/http"
	"strconv"
)

func taskEventRead(read func(context.Context, int64, int64, int64) (json.RawMessage, error)) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
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
		out, err := read(r.Context(), project, task, event)
		respond(w, 200, out, err)
	}
}
