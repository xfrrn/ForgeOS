package main

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"forgeos/internal/store"
	"github.com/google/uuid"
)

func registerChatStream(mux *http.ServeMux, data *store.Store, owner string, nodes chatNodes) {
	mux.HandleFunc("POST /internal/management-chat/progress", nodes.authorize(func(project int64, w http.ResponseWriter, r *http.Request) {
		var in struct {
			ID     int64
			Lease  string
			Answer string
		}
		if !decodeLimited(w, r, &in, 128<<10) {
			return
		}
		lease, err := uuid.Parse(in.Lease)
		if err != nil || in.ID <= 0 || len(in.Answer) > 16<<10 || strings.ContainsRune(in.Answer, 0) {
			problem(w, 400, "invalid chat progress")
			return
		}
		respond(w, http.StatusNoContent, nil, data.UpdateChatProgress(r.Context(), project, in.ID, lease.String(), in.Answer))
	}))
	mux.HandleFunc("GET /projects/{id}/chat/turns/{turn}/stream", authorized(owner, func(w http.ResponseWriter, r *http.Request) {
		project, ok := pathID(w, r)
		if !ok {
			return
		}
		id, err := strconv.ParseInt(r.PathValue("turn"), 10, 64)
		if err != nil || id <= 0 {
			problem(w, 400, "invalid turn")
			return
		}
		// End before the server's normal write deadline. EventSource reconnects
		// with the Owner session and receives the current snapshot immediately.
		until := time.Now().Add(18 * time.Second)
		lastStatus, lastAnswer := "", ""
		for time.Now().Before(until) {
			turn, err := data.ChatTurn(r.Context(), project, id)
			if err != nil {
				if lastStatus == "" {
					respond(w, 0, nil, err)
				}
				return
			}
			if turn.Status != lastStatus || turn.Answer != lastAnswer {
				if lastStatus == "" {
					w.Header().Set("Content-Type", "text/event-stream")
					w.Header().Set("Cache-Control", "no-store")
					w.Header().Set("X-Accel-Buffering", "no")
					fmt.Fprint(w, "retry: 300\n")
				}
				raw, _ := json.Marshal(turn)
				if _, err = fmt.Fprintf(w, "data: %s\n\n", raw); err != nil {
					return
				}
				if http.NewResponseController(w).Flush() != nil {
					return
				}
				lastStatus, lastAnswer = turn.Status, turn.Answer
			}
			if turn.Status == "completed" || turn.Status == "failed" {
				return
			}
			delay := 200 * time.Millisecond
			if turn.Status == "queued" {
				delay = time.Second
			}
			select {
			case <-r.Context().Done():
				return
			case <-time.After(delay):
			}
		}
	}))
}
