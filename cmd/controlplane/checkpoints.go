package main

import (
	"errors"
	"net/http"
	"strconv"

	"forgeos/internal/store"
	"forgeos/schemas"
)

func registerCheckpoints(mux *http.ServeMux, data *store.Store, owner string, workers *workerRegistry) {
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
		var input struct {
			Lease    string `json:"lease_token"`
			Sequence *int   `json:"sequence"`
			After    int64  `json:"after"`
			Session  string `json:"session_id"`
		}
		if !decode(w, r, &input) {
			return
		}
		if input.Sequence == nil || *input.Sequence < 0 || *input.Sequence > 2147483647 || input.After < 0 || input.Lease == "" {
			problem(w, http.StatusBadRequest, "provide an execution lease and checkpoint sequence")
			return
		}
		page, err := data.ReadCheckpoint(r.Context(), id, input.Lease, *input.Sequence, input.After, input.Session, r.URL.Query().Get("queue_next") == "1")
		if input.Session != "" {
			respond(w, http.StatusNoContent, nil, err)
			return
		}
		if err == nil && len(page.OwnerInstructions) > 0 && r.URL.Query().Get("commands") == "1.0" {
			page.Commands, err = data.InstructionMessages(r.Context(), page.TaskID, id, *input.Sequence)
		}
		respond(w, http.StatusOK, page, err)
	}))
}
