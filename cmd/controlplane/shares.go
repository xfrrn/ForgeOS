package main

import (
	"encoding/hex"
	"net/http"
	"strconv"

	"forgeos/internal/store"
)

func registerShares(mux *http.ServeMux, data *store.Store, owner string, workers workerAccess) {
	mux.HandleFunc("POST /projects/{id}/shares", authorized(owner, func(w http.ResponseWriter, r *http.Request) {
		id, ok := pathID(w, r)
		if !ok {
			return
		}
		var input store.ShareDraft
		if !decodeLimited(w, r, &input, 64<<10) {
			return
		}
		if !input.Valid(id) {
			problem(w, 400, "choose a fixed source revision, a different recipient, and a redacted title and text")
			return
		}
		out, err := data.DraftShare(r.Context(), id, input)
		respond(w, 200, out, err)
	}))
	ids := func(w http.ResponseWriter, r *http.Request) (int64, int64, bool) {
		id, ok := pathID(w, r)
		if !ok {
			return 0, 0, false
		}
		share, err := strconv.ParseInt(r.PathValue("share"), 10, 64)
		if err != nil || share < 1 {
			problem(w, 400, "invalid share id")
			return 0, 0, false
		}
		return id, share, true
	}
	mux.HandleFunc("GET /projects/{id}/shares/{share}", authorized(owner, func(w http.ResponseWriter, r *http.Request) {
		id, share, ok := ids(w, r)
		if !ok {
			return
		}
		out, err := data.ShareReceipt(r.Context(), id, share)
		respond(w, 200, out, err)
	}))
	mux.HandleFunc("POST /projects/{id}/shares/{share}/publication", authorized(owner, func(w http.ResponseWriter, r *http.Request) {
		id, share, ok := ids(w, r)
		if !ok {
			return
		}
		var input struct {
			Action string `json:"action"`
			Digest string `json:"digest"`
		}
		if !decode(w, r, &input) {
			return
		}
		digest, err := hex.DecodeString(input.Digest)
		if err != nil || len(digest) != 32 || (input.Action != "publish" && input.Action != "revoke") {
			problem(w, 400, "provide the copy digest and choose publish or revoke")
			return
		}
		out, err := data.SetSharePublished(r.Context(), id, share, input.Digest, input.Action == "publish")
		respond(w, 200, out, err)
	}))
	beforeID := func(w http.ResponseWriter, r *http.Request) (int64, bool) {
		var before int64
		if r.URL.Query().Has("before") {
			var err error
			before, err = strconv.ParseInt(r.URL.Query().Get("before"), 10, 64)
			if err != nil || before < 1 {
				problem(w, 400, "invalid shared experience cursor")
				return 0, false
			}
		}
		return before, true
	}
	mux.HandleFunc("GET /projects/{id}/shares", authorized(owner, func(w http.ResponseWriter, r *http.Request) {
		id, ok := pathID(w, r)
		if !ok {
			return
		}
		if before, ok := beforeID(w, r); ok {
			out, err := data.OutgoingShares(r.Context(), id, before)
			respond(w, 200, out, err)
		}
	}))
	registerProjectRead(mux, "shared-experience", owner, workers, func(w http.ResponseWriter, r *http.Request, id int64) {
		if before, ok := beforeID(w, r); ok {
			out, err := data.SharedExperience(r.Context(), id, before)
			respond(w, 200, out, err)
		}
	})
}
