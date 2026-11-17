package main

import (
	"encoding/json"
	"errors"
	"net/http"
	"strconv"

	"forgeos/internal/store"
	"forgeos/schemas"
)

func registerCheckpoints(mux *http.ServeMux, data *store.Store, owner string, workers *workerRegistry) {
	for _, action := range []string{"pause-model", "resume-model"} {
		mux.HandleFunc("POST /executions/{id}/"+action, authorized(owner, func(w http.ResponseWriter, r *http.Request) {
			id, ok := pathID(w, r)
			if !ok {
				return
			}
			var input struct {
				Instance int64 `json:"instance"`
			}
			if !decode(w, r, &input) {
				return
			}
			if input.Instance <= 0 {
				problem(w, 400, "provide a positive process instance")
				return
			}
			respond(w, http.StatusNoContent, nil, data.ControlModel(r.Context(), id, input.Instance, action == "resume-model"))
		}))
	}
	workers.handle(mux, "POST /internal/executions/{id}/model-pause", workers.execution(data, func(w http.ResponseWriter, r *http.Request) {
		id, ok := pathID(w, r)
		if !ok {
			return
		}
		var input store.PausePoll
		if !decode(w, r, &input) {
			return
		}
		p, err := data.PollModelPause(r.Context(), id, input)
		if errors.Is(err, store.ErrModelResumeClaimed) {
			problem(w, http.StatusPreconditionFailed, err.Error())
			return
		}
		if err == nil && p == nil {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		respond(w, http.StatusOK, p, err)
	}))
	mux.HandleFunc("POST /executions/{id}/instructions", authorized(owner, func(w http.ResponseWriter, r *http.Request) {
		id, ok := pathID(w, r)
		if !ok {
			return
		}
		var input struct {
			Sequence *int `json:"sequence"`
			store.OwnerInstruction
		}
		if !decodeLimited(w, r, &input, 32<<10) {
			return
		}
		if input.Sequence == nil {
			problem(w, http.StatusBadRequest, "provide the active checkpoint sequence")
			return
		}
		respond(w, http.StatusNoContent, nil, data.QueueInstruction(r.Context(), id, *input.Sequence, input.OwnerInstruction))
	}))
	mux.HandleFunc("GET /tasks/{id}/checkpoints", authorized(owner, func(w http.ResponseWriter, r *http.Request) {
		id, ok := pathID(w, r)
		if !ok {
			return
		}
		execution, err := strconv.ParseInt(r.URL.Query().Get("execution_id"), 10, 64)
		if err != nil || execution < 1 {
			problem(w, http.StatusBadRequest, "provide a positive execution ID")
			return
		}
		rows, err := data.Checkpoints(r.Context(), id, execution)
		respond(w, http.StatusOK, rows, err)
	}))
	mux.HandleFunc("GET /tasks/{id}/executions/{execution}/checkpoints/{sequence}/messages", authorized(owner, func(w http.ResponseWriter, r *http.Request) {
		task, ok := pathID(w, r)
		if !ok {
			return
		}
		execution, err := strconv.ParseInt(r.PathValue("execution"), 10, 64)
		seq, seqErr := strconv.ParseInt(r.PathValue("sequence"), 10, 32)
		if err != nil || execution < 1 || seqErr != nil || seq < 0 {
			problem(w, http.StatusBadRequest, "provide an execution and a receiving checkpoint")
			return
		}
		messages, err := data.InstructionMessages(r.Context(), task, execution, int(seq))
		respond(w, http.StatusOK, messages, err)
	}))
	workers.handle(mux, "POST /internal/executions/{id}/checkpoint-question", workers.execution(data, func(w http.ResponseWriter, r *http.Request) {
		id, ok := pathID(w, r)
		if !ok {
			return
		}
		var input store.CheckpointQuestion
		if !decode(w, r, &input) {
			return
		}
		if input.ReferenceDecisionID != 0 && r.URL.Query().Get("acp") != "1.0" {
			problem(w, http.StatusBadRequest, "decision lookup requires ACP 1.0")
			return
		}
		answer, err := data.AskAtCheckpoint(r.Context(), id, input)
		if errors.Is(err, store.ErrQuestionClaimed) {
			problem(w, http.StatusPreconditionFailed, err.Error())
			return
		}
		if err == nil && r.URL.Query().Get("acp") == "1.0" {
			messages := []schemas.AgentMessage{}
			if answer != "" {
				messages, err = data.CheckpointMessages(r.Context(), id, *input.Sequence)
			}
			respond(w, http.StatusOK, map[string]any{"messages": messages}, err)
			return
		}
		respond(w, http.StatusOK, map[string]string{"answer": answer}, err)
	}))
	workers.handle(mux, "POST /internal/executions/{id}/checkpoints", workers.execution(data, func(w http.ResponseWriter, r *http.Request) {
		id, ok := pathID(w, r)
		if !ok {
			return
		}
		var input store.CheckpointRequest
		if !decode(w, r, &input) {
			return
		}
		if input.Sequence == nil || *input.Sequence < 0 || *input.Sequence > 2147483647 || input.After < 0 || input.Lease == "" {
			problem(w, http.StatusBadRequest, "provide an execution lease and checkpoint sequence")
			return
		}
		page, err := data.WorkerCheckpoint(r.Context(), id, input, r.URL.Query().Get("queue_next") == "1")
		if input.Session != "" {
			respond(w, http.StatusNoContent, nil, err)
			return
		}
		if err == nil && len(page.OwnerInstructions) > 0 && r.URL.Query().Get("commands") == "1.0" {
			page.Commands, err = data.InstructionMessages(r.Context(), page.TaskID, id, *input.Sequence)
		}
		if err == nil && input.History && input.After == 0 && *input.Sequence > 0 {
			var messages []schemas.AgentMessage
			messages, err = data.CheckpointMessages(r.Context(), id, *input.Sequence-1)
			if errors.Is(err, store.ErrNotFound) {
				err = nil
			} else if err == nil {
				page.AnswerMessages, err = json.Marshal(messages)
			}
		}
		respond(w, http.StatusOK, page, err)
	}))
}
