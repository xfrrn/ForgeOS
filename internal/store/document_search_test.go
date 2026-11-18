package store

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os/exec"
	"strings"
	"sync/atomic"
	"testing"
)

func TestDocumentEmbeddingRetryKeepsWholeBlobAndByteOffsets(t *testing.T) {
	ctx := context.Background()
	s := New(isolatedDatabase(t))
	if err := s.Migrate(ctx); err != nil {
		t.Fatal(err)
	}
	repo := t.TempDir()
	p, err := s.CreateProject(ctx, repo)
	if err != nil {
		t.Fatal(err)
	}
	if out, err := exec.Command("git", "-C", repo, "init", "-q").CombinedOutput(); err != nil {
		t.Fatal(err, string(out))
	}
	body := strings.Repeat("巡检 handbook ", 200) + "verify checksum"
	cmd := exec.Command("git", "-C", repo, "hash-object", "-w", "--stdin")
	cmd.Stdin = strings.NewReader(body)
	out, err := cmd.Output()
	if err != nil {
		t.Fatal(err)
	}
	blob := strings.TrimSpace(string(out))
	var calls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var request struct{ Input []string }
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
			t.Error(err)
			return
		}
		v := []float32{1, 0}
		if calls.Add(1) == 2 {
			v = []float32{1}
		}
		var rows []map[string]any
		for i := range request.Input {
			rows = append(rows, map[string]any{"index": i, "embedding": v})
		}
		if err := json.NewEncoder(w).Encode(map[string]any{"data": rows}); err != nil {
			t.Error(err)
		}
	}))
	defer server.Close()
	s.embeddings = &materialEncoder{endpoint: server.URL, model: "fixture", client: server.Client()}
	if err := s.indexDocumentBlob(ctx, p.ID, repo, blob, 2); err == nil {
		t.Fatal("accepted inconsistent embedding dimensions")
	}
	var count int
	if err := s.db.QueryRow("SELECT count(*) FROM document_embeddings").Scan(&count); err != nil || count != 0 {
		t.Fatal(count, err)
	}
	if err := s.indexDocumentBlob(ctx, p.ID, repo, blob, 2); err != nil {
		t.Fatal(err)
	}
	before := calls.Load()
	if err := s.indexDocumentBlob(ctx, p.ID, repo, blob, 2); err != nil || before != calls.Load() {
		t.Fatal("cache miss", err)
	}
	var start, end int
	if err := s.db.QueryRow(`SELECT start_byte,end_byte FROM document_embeddings WHERE words @@ plainto_tsquery('simple','checksum') ORDER BY end_byte DESC LIMIT 1`).Scan(&start, &end); err != nil || end != len(body) || !strings.HasSuffix(body[start:end], "verify checksum") {
		t.Fatal(start, end, err)
	}
}
