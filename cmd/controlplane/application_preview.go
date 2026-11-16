package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"forgeos/internal/preview"
)

type previewBackend struct {
	addr, token string
	client      *http.Client
}

func newPreviewBackend() (*previewBackend, error) {
	addr, token, err := preview.ManagerConfig()
	if err != nil || addr == "" {
		return nil, err
	}
	return &previewBackend{addr, token, &http.Client{Transport: &http.Transport{DisableCompression: true}, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}}, nil
}

type managerFailure struct {
	status  int
	message string
}

func (e *managerFailure) Error() string { return e.message }

func (b *previewBackend) change(ctx context.Context, id string, lease *preview.Lease) error {
	if b == nil {
		return &managerFailure{503, "preview manager is not configured"}
	}
	method := "DELETE"
	var body []byte
	if lease != nil {
		method = "PUT"
		var err error
		body, err = json.Marshal(lease)
		if err != nil {
			return err
		}
	}
	req, err := http.NewRequestWithContext(ctx, method, "http://"+b.addr+"/instances/"+id, bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+b.token)
	resp, err := b.client.Do(req)
	if err != nil {
		return &managerFailure{503, "preview manager unavailable"}
	}
	defer resp.Body.Close()
	if resp.StatusCode == 204 {
		return nil
	}
	if resp.StatusCode == 422 {
		msg, err := io.ReadAll(io.LimitReader(resp.Body, 512))
		if err == nil {
			return &managerFailure{422, strings.TrimSpace(string(msg))}
		}
	}
	return &managerFailure{503, "preview manager could not complete the request"}
}
func (b *previewBackend) forward(w http.ResponseWriter, r *http.Request, id, gateway string) {
	if b == nil {
		http.Error(w, "preview manager unavailable", 503)
		return
	}
	switch r.Method {
	case "GET", "HEAD", "OPTIONS":
	case "POST", "PUT", "DELETE", "PATCH":
		origin := r.Header.Get("Origin")
		if r.Header.Get("Sec-Fetch-Site") == "cross-site" || (origin != "" && origin != "http://"+r.Host && (gateway == "" || origin != "https://p-"+id+"."+gateway)) {
			http.Error(w, "cross-origin preview mutation", 403)
			return
		}
	default:
		w.WriteHeader(405)
		return
	}
	if r.ContentLength > 8<<20 {
		http.Error(w, "preview request exceeds 8 MiB", 413)
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 22*time.Second)
	defer cancel()
	target := url.URL{Scheme: "http", Host: b.addr, Path: r.URL.Path, RawPath: r.URL.RawPath, RawQuery: r.URL.RawQuery, ForceQuery: r.URL.ForceQuery}
	req, err := http.NewRequestWithContext(ctx, r.Method, target.String(), http.MaxBytesReader(w, r.Body, 8<<20))
	if err != nil {
		http.Error(w, "invalid application request", 400)
		return
	}
	if r.ContentLength == 0 {
		req.Body = http.NoBody
	}
	req.ContentLength = r.ContentLength
	preview.CopyRequestHeaders(req.Header, r.Header)
	req.Header.Set("Authorization", "Bearer "+b.token)
	req.Header.Set(preview.InstanceHeader, id)
	resp, err := b.client.Do(req)
	if err != nil {
		var size *http.MaxBytesError
		if errors.As(err, &size) {
			http.Error(w, "preview request exceeds 8 MiB", 413)
		} else {
			http.Error(w, "preview manager unavailable", 503)
		}
		return
	}
	defer resp.Body.Close()
	preview.CopyResponseHeaders(w.Header(), resp.Header)
	w.WriteHeader(resp.StatusCode)
	_, _ = io.Copy(w, resp.Body)
}
