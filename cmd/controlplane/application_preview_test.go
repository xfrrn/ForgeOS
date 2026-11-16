package main

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"forgeos/internal/preview"
)

func TestApplicationPreviewOnlyForwardsApplicationData(t *testing.T) {
	id := strings.Repeat("a", 48)
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		if r.Method != "POST" || r.URL.RequestURI() != "/a%2Fb?q=next" || string(body) != "change" || r.Header.Get("Authorization") != "Bearer service-only" || r.Header.Get(preview.InstanceHeader) != id || r.Header.Get("Cookie") != "" {
			t.Error("incorrect forwarded request")
		}
		w.Header().Set("Set-Cookie", "session=untrusted")
		w.Header().Set("X-ForgeOS-Commit", "untrusted")
		w.Header().Set("Location", "/updated")
		w.WriteHeader(303)
	}))
	defer upstream.Close()
	b := previewBackend{strings.TrimPrefix(upstream.URL, "http://"), "service-only", &http.Client{CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}}
	for _, c := range []struct {
		origin string
		status int
	}{{"https://other.invalid", 403}, {"https://p-" + id + ".forge.test:9443", 303}} {
		r := httptest.NewRequest("POST", "http://p-"+id+".localhost:8091/a%2Fb?q=next", strings.NewReader("change"))
		r.Header.Set("Origin", c.origin)
		r.Header.Set("Cookie", "private")
		r.Header.Set("Authorization", "Bearer owner")
		r.Header.Set(preview.InstanceHeader, "spoofed")
		w := httptest.NewRecorder()
		b.forward(w, r, id, "forge.test:9443")
		if w.Code != c.status || w.Header().Get("Set-Cookie") != "" || w.Header().Get("X-ForgeOS-Commit") != "" || (c.status == 303 && w.Header().Get("Location") != "/updated") {
			t.Fatal(w.Code, w.Header())
		}
	}
}
