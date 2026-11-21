package main

import (
	"net/http"
	"strconv"
	"strings"
	"unicode/utf8"

	"forgeos/internal/store"
)

func experiencePathID(w http.ResponseWriter, r *http.Request, key string) (int64, bool) {
	id, err := strconv.ParseInt(r.PathValue(key), 10, 64)
	if err != nil || id <= 0 {
		problem(w, 400, "invalid experience "+key)
		return 0, false
	}
	return id, true
}

func registerExperienceLibrary(mux *http.ServeMux, data *store.Store, owner string) {
	const library = "/projects/{id}/knowledge/experience"
	mux.HandleFunc("GET "+library, authorized(owner, func(w http.ResponseWriter, r *http.Request) {
		project, ok := pathID(w, r)
		if !ok {
			return
		}
		q := r.URL.Query()
		status, query := q.Get("status"), strings.TrimSpace(q.Get("q"))
		if status == "" {
			status = "current"
		}
		switch status {
		case "current", "pending", "history", "all":
		default:
			problem(w, 400, "invalid experience filter")
			return
		}
		if len(query) > 512 || !utf8.ValidString(query) || strings.ContainsRune(query, 0) {
			problem(w, 400, "invalid experience query")
			return
		}
		var before int64
		if q.Has("before") {
			var err error
			before, err = strconv.ParseInt(q.Get("before"), 10, 64)
			if err != nil || before <= 0 {
				problem(w, 400, "invalid experience cursor")
				return
			}
		}
		out, err := data.ExperienceEntries(r.Context(), project, before, status, query)
		respond(w, 200, out, err)
	}))
	mux.HandleFunc("GET "+library+"/{entry}", authorized(owner, func(w http.ResponseWriter, r *http.Request) {
		project, ok := pathID(w, r)
		if !ok {
			return
		}
		entry, ok := experiencePathID(w, r, "entry")
		if !ok {
			return
		}
		out, err := data.ExperienceEntry(r.Context(), project, entry)
		respond(w, 200, out, err)
	}))
	submit := authorized(owner, func(w http.ResponseWriter, r *http.Request) {
		project, ok := pathID(w, r)
		if !ok {
			return
		}
		var entry int64
		if r.PathValue("entry") != "" {
			entry, ok = experiencePathID(w, r, "entry")
			if !ok {
				return
			}
		}
		var input store.ExperienceChangeInput
		if !decodeLimited(w, r, &input, 2<<20) {
			return
		}
		if !input.Valid(entry == 0) {
			problem(w, 400, "provide a request key, expected revision, source, title, Markdown text and change reason")
			return
		}
		out, err := data.SubmitExperienceChange(r.Context(), project, entry, input)
		respond(w, 200, out, err)
	})
	mux.HandleFunc("POST "+library, submit)
	mux.HandleFunc("POST "+library+"/{entry}/changes", submit)
	changeHandler := func(handle func(http.ResponseWriter, *http.Request, int64, int64, int64)) http.HandlerFunc {
		return authorized(owner, func(w http.ResponseWriter, r *http.Request) {
			project, ok := pathID(w, r)
			if !ok {
				return
			}
			entry, ok := experiencePathID(w, r, "entry")
			if !ok {
				return
			}
			change, ok := experiencePathID(w, r, "change")
			if !ok {
				return
			}
			handle(w, r, project, entry, change)
		})
	}
	const change = library + "/{entry}/changes/{change}"
	mux.HandleFunc("GET "+change+"/curation", changeHandler(func(w http.ResponseWriter, r *http.Request, p, e, c int64) {
		out, err := data.ExperienceChangeCuration(r.Context(), p, e, c)
		respond(w, 200, out, err)
	}))
	mux.HandleFunc("POST "+change+"/adopt", changeHandler(func(w http.ResponseWriter, r *http.Request, p, e, c int64) {
		var input struct {
			Number int `json:"fact_number"`
		}
		if !decode(w, r, &input) {
			return
		}
		if input.Number < 1 || input.Number > 8 {
			problem(w, 400, "choose an extracted fact")
			return
		}
		out, err := data.AdoptExperienceChange(r.Context(), p, e, c, input.Number)
		respond(w, 200, out, err)
	}))
	mux.HandleFunc("POST "+change+"/discard", changeHandler(func(w http.ResponseWriter, r *http.Request, p, e, c int64) {
		if !decode(w, r, &struct{}{}) {
			return
		}
		out, err := data.DiscardExperienceChange(r.Context(), p, e, c)
		respond(w, 200, out, err)
	}))
	mux.HandleFunc("POST "+library+"/{entry}/withdraw", authorized(owner, func(w http.ResponseWriter, r *http.Request) {
		project, ok := pathID(w, r)
		if !ok {
			return
		}
		entry, ok := experiencePathID(w, r, "entry")
		if !ok {
			return
		}
		var input struct {
			Revision *int `json:"expected_revision"`
		}
		if !decode(w, r, &input) {
			return
		}
		if input.Revision == nil || *input.Revision < 0 {
			problem(w, 400, "provide the expected revision")
			return
		}
		out, err := data.WithdrawExperience(r.Context(), project, entry, *input.Revision)
		respond(w, 200, out, err)
	}))
}
