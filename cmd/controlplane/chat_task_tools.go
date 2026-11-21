package main

import (
	"encoding/json"
	"errors"
	"net/http"

	"forgeos/internal/chatkernel"
	"forgeos/internal/store"
	"github.com/google/uuid"
)

func (c projectChat) taskTool(project int64, w http.ResponseWriter, r *http.Request) {
	var in struct {
		ID          int64
		Lease, Name string
		Arguments   json.RawMessage
	}
	if !decodeLimited(w, r, &in, 8<<10) {
		return
	}
	lease, err := uuid.Parse(in.Lease)
	if err != nil || in.ID < 1 {
		problem(w, 400, "invalid task tool lease")
		return
	}
	ctx := r.Context()
	if _, err = c.data.LeasedChatModel(ctx, project, in.ID, lease.String()); err != nil {
		respond(w, 0, nil, err)
		return
	}
	var result any
	var progressID int64
	sources := []store.ChatSource{}
	switch in.Name {
	case chatkernel.ResolveTask:
		var ref store.TaskReference
		if json.Unmarshal(in.Arguments, &ref) != nil {
			problem(w, 400, "invalid task reference")
			return
		}
		var matches store.ChatTaskMatches
		matches, err = c.data.ResolveChatTask(ctx, project, in.ID, ref)
		result = matches
		for _, m := range matches.Matches {
			if m.TaskID > 0 {
				sources = append(sources, store.ChatTaskSource(m.TaskID))
			}
		}
	case chatkernel.TaskProgress:
		var arg struct {
			TaskID int64 `json:"task_id"`
		}
		if json.Unmarshal(in.Arguments, &arg) != nil || arg.TaskID < 1 {
			problem(w, 400, "invalid task id")
			return
		}
		progressID = arg.TaskID
		result, sources, err = c.data.ChatTaskProgress(ctx, project, arg.TaskID)
	default:
		problem(w, 400, "unknown chat tool")
		return
	}
	if errors.Is(err, store.ErrTaskReference) {
		problem(w, 400, "invalid task reference")
		return
	}
	if err == nil {
		err = c.data.AddChatToolEvidence(ctx, project, in.ID, lease.String(), sources, progressID)
	}
	respond(w, 200, map[string]any{"data": result, "sources": sources}, err)
}
