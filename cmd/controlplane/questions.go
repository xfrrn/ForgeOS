package main

import (
	"net/http"
	"strconv"
	"strings"

	"forgeos/internal/store"
)

func registerQuestionRoutes(mux *http.ServeMux, data *store.Store, owner string) {
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
