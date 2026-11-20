package main

import (
	"net/http"
	"strconv"
	"strings"

	"forgeos/internal/store"
)

func registerQuestionRoutes(mux *http.ServeMux, data *store.Store, owner string) {
	mux.HandleFunc("GET /tasks/{id}/questions/{question}/messages", authorized(owner, func(w http.ResponseWriter, r *http.Request) {
		task, ok := pathID(w, r)
		if !ok {
			return
		}
		id, err := strconv.ParseInt(r.PathValue("question"), 10, 64)
		if err != nil || id < 1 {
			problem(w, 400, "invalid question id")
			return
		}
		messages, err := data.QuestionMessages(r.Context(), task, id)
		respond(w, http.StatusOK, messages, err)
	}))
	mux.HandleFunc("GET /tasks/decision-questions", authorized(owner, func(w http.ResponseWriter, r *http.Request) {
		q, project, ok := ownerProjectQuery(w, r)
		if !ok {
			return
		}
		var before int64
		if value := q.Get("before"); value != "" {
			var err error
			before, err = strconv.ParseInt(value, 10, 64)
			if err != nil || before <= 0 {
				problem(w, 400, "before must be a positive question ID")
				return
			}
		}
		page, err := data.PendingQuestions(r.Context(), project, before)
		respond(w, 200, page, err)
	}))
	mux.HandleFunc("POST /tasks/{id}/questions", authorized(owner, func(w http.ResponseWriter, r *http.Request) {
		id, ok := pathID(w, r)
		if !ok {
			return
		}
		var input store.QuestionRequest
		if !decode(w, r, &input) {
			return
		}
		if !input.Valid() {
			problem(w, 400, "provide execution_id, commit_sha, request_key, question and evidence")
			return
		}
		task, err := data.AskAtAcceptance(r.Context(), id, input)
		respond(w, 200, task, err)
	}))
	mux.HandleFunc("POST /tasks/{id}/questions/{question}/answer", authorized(owner, func(w http.ResponseWriter, r *http.Request) {
		id, ok := pathID(w, r)
		if !ok {
			return
		}
		qID, err := strconv.ParseInt(r.PathValue("question"), 10, 64)
		if err != nil || qID < 1 {
			problem(w, 400, "invalid question id")
			return
		}
		var input struct {
			Answer string `json:"answer"`
		}
		if !decode(w, r, &input) {
			return
		}
		answer := strings.TrimSpace(input.Answer)
		if answer == "" || len(answer) > 4096 || strings.ContainsRune(answer, 0) {
			problem(w, 400, "provide an answer up to 4096 bytes")
			return
		}
		task, err := data.AnswerQuestion(r.Context(), id, qID, answer)
		respond(w, 200, task, err)
	}))
}
