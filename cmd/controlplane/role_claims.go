package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"

	"forgeos/internal/store"
)

type roleClaimBuffer struct {
	header http.Header
	body   bytes.Buffer
	status int
	err    error
}

func (b *roleClaimBuffer) Header() http.Header { return b.header }
func (b *roleClaimBuffer) WriteHeader(status int) {
	if b.status == 0 {
		b.status = status
	}
}
func (b *roleClaimBuffer) Write(p []byte) (int, error) {
	if b.body.Len()+len(p) > 32<<20 {
		b.err = errors.New("role claim response too large")
		return 0, b.err
	}
	if b.status == 0 {
		b.status = 200
	}
	return b.body.Write(p)
}

func idempotentRoleClaim(data *store.Store, project int64, role string, next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		key, node := r.Header.Get("X-ForgeOS-Claim-Key"), r.Header.Get("X-ForgeOS-Node")
		if key == "" && node == "" {
			next(w, r)
			return
		}
		body, err := io.ReadAll(http.MaxBytesReader(w, r.Body, 16<<10))
		if err != nil {
			problem(w, 400, "invalid claim request body")
			return
		}
		request := append([]byte(r.URL.RequestURI()+"\x00"), body...)
		status, response, err := data.RoleClaim(r.Context(), project, role, node, key, request, func(ctx context.Context) (int, []byte, error) {
			call := r.WithContext(ctx)
			call.Body = io.NopCloser(bytes.NewReader(body))
			buffer := &roleClaimBuffer{header: make(http.Header)}
			next(buffer, call)
			if buffer.status == 200 && buffer.err == nil && r.URL.Query().Get("supports_role_resume") == "1" {
				var job map[string]json.RawMessage
				if err := json.Unmarshal(buffer.body.Bytes(), &job); err != nil {
					return 0, nil, err
				}
				job["recover_native"] = json.RawMessage("true")
				raw, err := json.Marshal(job)
				return buffer.status, raw, err
			}
			return buffer.status, buffer.body.Bytes(), buffer.err
		})
		if err != nil {
			respond(w, 0, nil, err)
			return
		}
		w.Header().Set("X-ForgeOS-Claim-Key", key)
		w.Header().Set("Cache-Control", "no-store")
		if status != 204 {
			w.Header().Set("Content-Type", "application/json")
		}
		w.WriteHeader(status)
		_, _ = w.Write(response)
	}
}
