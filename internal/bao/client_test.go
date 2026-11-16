package bao

import (
	"context"
	"encoding/json"
	"encoding/pem"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	"forgeos/internal/store"
)

func TestMetadataStatusAndIdentityRotation(t *testing.T) {
	type requestCase struct {
		token, state string
		code         int
	}
	cases := make(chan requestCase, 1)
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		tc := <-cases
		if r.Method != "GET" || r.URL.Path != "/v1/secret/metadata/projects/2/ci-test/sample" || r.Header.Get("X-Vault-Token") != tc.token {
			t.Error("incorrect metadata request")
		}
		if tc.code != 200 {
			if tc.code == 307 {
				w.Header().Set("Location", "https://invalid.example/redirected")
			}
			w.WriteHeader(tc.code)
			return
		}
		entry := map[string]any{"destroyed": tc.state == "DESTROYED"}
		switch tc.state {
		case "DELETED":
			entry["deletion_time"] = time.Now().Add(-time.Minute)
		case "scheduled":
			entry["deletion_time"] = time.Now().Add(time.Hour)
		}
		json.NewEncoder(w).Encode(map[string]any{"data": map[string]any{"current_version": 2, "versions": map[string]any{"2": entry}}})
	}))
	defer server.Close()
	dir := t.TempDir()
	ca, token := filepath.Join(dir, "ca.pem"), filepath.Join(dir, "token")
	if err := os.WriteFile(ca, pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: server.Certificate().Raw}), 0600); err != nil {
		t.Fatal(err)
	}
	client, err := New(server.URL, token, ca)
	if err != nil {
		t.Fatal(err)
	}
	meta := store.SecretMeta{ProjectID: 2, Environment: "ci-test", Name: "sample"}
	for _, tc := range []struct {
		state, want string
		code        int
	}{
		{"AVAILABLE", "AVAILABLE", 200}, {"scheduled", "AVAILABLE", 200}, {"DELETED", "DELETED", 200}, {"DESTROYED", "DESTROYED", 200}, {"", "MISSING", 404}, {"", "", 403}, {"", "", 307},
	} {
		wantToken := "t-" + tc.state
		cases <- requestCase{wantToken, tc.state, tc.code}
		if err = os.WriteFile(token, []byte(wantToken), 0600); err != nil {
			t.Fatal(err)
		}
		v, err := client.Status(context.Background(), meta)
		if v.State != tc.want || (err != nil) != (tc.want == "") || (tc.code == 200 && v.Version != 2) {
			t.Fatal(v, err)
		}
	}
}
