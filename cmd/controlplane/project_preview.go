package main

import (
	"context"
	"crypto/subtle"
	"errors"
	"fmt"
	"net/http"
	"net/http/httputil"
	"net/url"
	"os"
	"strings"
	"time"

	"forgeos/internal/preview"
	"forgeos/internal/store"
	"github.com/google/uuid"
)

func registerProjectPreview(mux *http.ServeMux, data *store.Store, owner, gateway string, port int) {
	worker := strings.TrimSpace(os.Getenv("FORGEOS_PROJECT_PREVIEW_WORKER"))
	show := func(w http.ResponseWriter, r *http.Request, project int64, status int) {
		v, err := data.ProjectPreview(r.Context(), project)
		address := ""
		if v.State == "running" && v.Key != "" {
			if v.Phase != "READY" && v.Phase != "RUNNING" && !projectPreviewReady(r.Context(), data, v.Key) {
				v.State = "unknown"
			} else if gateway != "" && r.Host == "forgeos."+gateway {
				address = "https://p-" + v.Key + "." + gateway + "/"
			} else {
				address = fmt.Sprintf("http://p-%s.localhost:%d/", v.Key, port)
			}
		}
		w.Header().Set("Cache-Control", "no-store")
		respond(w, status, struct {
			store.ProjectPreviewView
			Enabled bool   `json:"enabled"`
			URL     string `json:"url,omitempty"`
		}{v, worker != "" && port > 0, address}, err)
	}
	const path = "/projects/{id}/preview"
	mux.HandleFunc("GET "+path, authorized(owner, func(w http.ResponseWriter, r *http.Request) {
		if id, ok := pathID(w, r); ok {
			show(w, r, id, http.StatusOK)
		}
	}))
	mux.HandleFunc("POST "+path, authorized(owner, func(w http.ResponseWriter, r *http.Request) {
		id, ok := pathID(w, r)
		if !ok {
			return
		}
		if worker == "" || port == 0 {
			problem(w, http.StatusServiceUnavailable, "尚未指定项目预览 Worker。")
			return
		}
		var req struct {
			Action  string    `json:"action"`
			Request uuid.UUID `json:"request_id"`
		}
		if !decodeLimited(w, r, &req, 2048) {
			return
		}
		if !store.ValidProjectPreviewAction(req.Action) || req.Request == uuid.Nil {
			problem(w, http.StatusBadRequest, "需要有效的预览操作和请求 ID。")
			return
		}
		if err := data.QueueProjectPreview(r.Context(), id, worker, req.Action, req.Request); err != nil {
			respond(w, 0, nil, err)
			return
		}
		show(w, r, id, http.StatusAccepted)
	}))
}

var projectPreviewTransport = &http.Transport{
	ResponseHeaderTimeout: 20 * time.Second,
}

func projectPreviewReady(ctx context.Context, data *store.Store, key string) bool {
	target, err := data.ProjectPreviewTarget(ctx, key)
	if err != nil {
		return false
	}
	ctx, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()
	request, err := http.NewRequestWithContext(ctx, http.MethodGet,
		fmt.Sprintf("http://127.0.0.1:%d%s", target.Port, preview.ProjectHealthPath), nil)
	if err != nil {
		return false
	}
	request.Header.Set(preview.ProjectProofHeader, preview.ProjectProof(target.Secret, target.Instance, "request"))
	response, err := projectPreviewTransport.RoundTrip(request)
	if err != nil {
		return false
	}
	defer response.Body.Close()
	return response.StatusCode >= 200 && response.StatusCode < 300 &&
		subtle.ConstantTimeCompare([]byte(response.Header.Get(preview.ProjectProofHeader)),
			[]byte(preview.ProjectProof(target.Secret, target.Instance, "response"))) == 1
}

func sameOriginPreview(r *http.Request, key, gateway string) bool {
	origin := r.Header.Get("Origin")
	return r.Header.Get("Sec-Fetch-Site") != "cross-site" &&
		(origin == "" || origin == "http://"+r.Host || gateway != "" && origin == "https://p-"+key+"."+gateway)
}

func serveProjectPreview(w http.ResponseWriter, r *http.Request, data *store.Store, key, gateway string) {
	target, err := data.ProjectPreviewTarget(r.Context(), key)
	if errors.Is(err, store.ErrNotFound) {
		http.NotFound(w, r)
		return
	}
	if err != nil {
		http.Error(w, "项目预览暂不可用。", http.StatusServiceUnavailable)
		return
	}
	if strings.HasPrefix(r.URL.Path, "/.forgeos-preview/") {
		http.NotFound(w, r)
		return
	}
	if r.Method != http.MethodGet && r.Method != http.MethodHead && r.Method != http.MethodOptions &&
		!sameOriginPreview(r, key, gateway) {
		http.Error(w, "cross-origin preview request", http.StatusForbidden)
		return
	}
	if r.ContentLength > 8<<20 {
		http.Error(w, "preview request exceeds 8 MiB", http.StatusRequestEntityTooLarge)
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, 8<<20)
	upstream := &url.URL{Scheme: "http", Host: fmt.Sprintf("127.0.0.1:%d", target.Port)}
	proxy := httputil.ReverseProxy{
		Transport: projectPreviewTransport,
		Rewrite: func(req *httputil.ProxyRequest) {
			req.SetURL(upstream)
			req.Out.Header.Del("Authorization")
			req.Out.Header.Del("Cookie")
			req.Out.Header.Set(preview.ProjectProofHeader, preview.ProjectProof(target.Secret, target.Instance, "request"))
		},
		ModifyResponse: func(resp *http.Response) error {
			proof := resp.Header.Get(preview.ProjectProofHeader)
			resp.Header.Del(preview.ProjectProofHeader)
			resp.Header.Del("Set-Cookie")
			if subtle.ConstantTimeCompare([]byte(proof), []byte(preview.ProjectProof(target.Secret, target.Instance, "response"))) != 1 {
				return errors.New("project preview instance changed")
			}
			return nil
		},
		ErrorHandler: func(w http.ResponseWriter, _ *http.Request, _ error) {
			http.Error(w, "项目预览已离线，请回到工作台检查状态。", http.StatusServiceUnavailable)
		},
	}
	proxy.ServeHTTP(w, r)
}
