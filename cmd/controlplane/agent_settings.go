package main

import (
	"encoding/json"
	"errors"
	"forgeos/internal/skilllib"
	"io"
	"net/http"
	"strings"

	"forgeos/internal/agents"
	"forgeos/internal/store"
	"github.com/google/uuid"
)

func registerAgentSettings(mux *http.ServeMux, data *store.Store, owner string, workers *workerRegistry) {
	mux.HandleFunc("GET /projects/agent-skills", authorized(owner, func(w http.ResponseWriter, r *http.Request) {
		skills, err := data.AgentSkills(r.Context())
		respond(w, http.StatusOK, skills, err)
	}))
	mux.HandleFunc("POST /projects/agent-skills", authorized(owner, func(w http.ResponseWriter, r *http.Request) {
		var skill store.SkillOption
		var err error
		if strings.HasPrefix(r.Header.Get("Content-Type"), "multipart/form-data") {
			r.Body = http.MaxBytesReader(w, r.Body, skilllib.MaxBytes+(2<<20))
			reader, readErr := r.MultipartReader()
			if readErr != nil {
				problem(w, 400, "无法读取上传目录。")
				return
			}
			var files []skilllib.File
			remaining := int64(skilllib.MaxBytes)
			for {
				part, readErr := reader.NextPart()
				if readErr == io.EOF {
					break
				}
				if readErr != nil {
					problem(w, 400, "上传目录不完整或超过大小限制。")
					return
				}
				raw, readErr := io.ReadAll(io.LimitReader(part, remaining+1))
				if readErr != nil {
					problem(w, 400, "上传文件不完整。")
					return
				}
				remaining -= int64(len(raw))
				if remaining < 0 || len(files) >= skilllib.MaxFiles {
					problem(w, 413, skilllib.ErrPackage.Error())
					return
				}
				files = append(files, skilllib.File{Path: part.FormName(), Data: raw})
			}
			skill, err = data.ImportSkillPackage(r.Context(), files)
		} else {
			var in struct {
				Document string `json:"document"`
			}
			if !decodeLimited(w, r, &in, 6*store.MaxSkillBytes+1024) {
				return
			}
			skill, err = data.ImportAgentSkill(r.Context(), in.Document)
		}
		if errors.Is(err, skilllib.ErrDocument) || errors.Is(err, skilllib.ErrPackage) {
			problem(w, 400, err.Error())
			return
		}
		respond(w, http.StatusOK, skill, err)
	}))
	mux.HandleFunc("GET /projects/agent-models", authorized(owner, func(w http.ResponseWriter, r *http.Request) {
		respond(w, http.StatusOK, agentModels(), nil)
	}))
	for _, path := range []string{"/projects/agent-settings", "/projects/{id}/agents"} {
		mux.HandleFunc("GET "+path, authorized(owner, func(w http.ResponseWriter, r *http.Request) {
			var scope int64
			if r.PathValue("id") != "" {
				var ok bool
				scope, ok = pathID(w, r)
				if !ok {
					return
				}
			}
			entries, err := data.AgentEntries(r.Context(), scope)
			respond(w, 200, entries, err)
		}))
		mux.HandleFunc("PUT "+path, authorized(owner, func(w http.ResponseWriter, r *http.Request) {
			var scope int64
			if r.PathValue("id") != "" {
				var ok bool
				scope, ok = pathID(w, r)
				if !ok {
					return
				}
			}
			var in struct {
				Key       string           `json:"key"`
				Previous  *int64           `json:"previous"`
				Effective *int64           `json:"effective"`
				Settings  *agents.Settings `json:"settings"`
				Reset     bool             `json:"reset"`
			}
			if !decodeLimited(w, r, &in, 128<<10) {
				return
			}
			d, ok := agents.Lookup(in.Key)
			if !ok || in.Previous == nil || in.Effective == nil || in.Reset != (in.Settings == nil) || (in.Settings != nil && !d.Valid(*in.Settings)) {
				problem(w, 400, "请检查用途、启动提示词、Harness 和模型标识。")
				return
			}
			err := data.SaveAgentConfig(r.Context(), scope, d, *in.Previous, *in.Effective, in.Settings)
			if err == store.ErrSkillSelection {
				problem(w, 400, err.Error())
				return
			}
			if err == store.ErrConflict {
				problem(w, 409, "配置已更新，请重新读取后再保存。")
				return
			}
			respond(w, 204, nil, err)
		}))
	}
	// Each claim supplies a random single-assignment capability. Only its hash is stored.
	workers.handle(mux, "POST /internal/agent-observations", func(w http.ResponseWriter, r *http.Request) {
		var in store.AgentObservation
		if !decodeLimited(w, r, &in, 2048) {
			return
		}
		if _, err := uuid.Parse(in.Ticket); err != nil || !agents.ValidModel(in.Model) || (in.Harness != "claude" && in.Harness != "codex") || (in.Outcome != "completed" && in.Outcome != "failed") {
			problem(w, 400, "invalid agent observation")
			return
		}
		if in.Session != "" {
			if _, err := uuid.Parse(in.Session); err != nil {
				problem(w, 400, "invalid native session")
				return
			}
		}
		respond(w, 204, nil, data.ObserveAgent(r.Context(), in))
	})
}
func rolePurpose(role string, job map[string]json.RawMessage) string {
	if role != "management" {
		return role
	}
	for field, key := range map[string]string{"question": "question", "share": "share", "test_plan": "coverage", "feedback": "feedback", "repository_analysis": "analysis", "curation": "knowledge-review", "issue_classification": "classification"} {
		if len(job[field]) > 0 && string(job[field]) != "null" {
			return key
		}
	}
	return "plan-review"
}
