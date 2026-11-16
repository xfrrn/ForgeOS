package main

import (
	"forgeos/internal/preview"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestPreviewManagerRejectsInvalidRequestsBeforeDocker(t *testing.T) {
	m := &previewManager{token: "test-secret"}
	path := "/instances/" + strings.Repeat("a", 48)
	for _, c := range []struct {
		token, path, id, body string
		want                  int
	}{
		{"", path, "", "{}", 401},
		{m.token, "/instances/bad", "", "{}", 404},
		{m.token, path, "bad", "", 404},
		{m.token, path, "", `{"repo":"project","commit":"HEAD"}`, 400},
	} {
		req := httptest.NewRequest("PUT", c.path, strings.NewReader(c.body))
		if c.token != "" {
			req.Header.Set("Authorization", "Bearer "+c.token)
		}
		req.Header.Set(preview.InstanceHeader, c.id)
		w := httptest.NewRecorder()
		m.ServeHTTP(w, req)
		if w.Code != c.want {
			t.Fatalf("%s returned %d, want %d", c.path, w.Code, c.want)
		}
	}
}

func TestPreviewApplicationHeaders(t *testing.T) {
	h := http.Header{"Accept": {"application/json"}, "Authorization": {"Bearer private"}, "Cookie": {"private"}, "Set-Cookie": {"test"}, "Location": {"/next"}, "X-Forgeos-Commit": {"untrusted"}}
	req, resp := http.Header{}, http.Header{}
	preview.CopyRequestHeaders(req, h)
	preview.CopyResponseHeaders(resp, h)
	if len(req) != 1 || req.Get("Accept") != "application/json" || len(resp) != 1 || resp.Get("Location") != "/next" {
		t.Fatal(req, resp)
	}
}
