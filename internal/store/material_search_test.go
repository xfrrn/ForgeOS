package store

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestMaterialEncoderKeepsInputOrder(t *testing.T) {
	for _, tt := range []struct {
		name, data string
		valid      bool
	}{
		{"reordered", `[{"index":1,"embedding":[0,3]},{"index":0,"embedding":[2,0]}]`, true},
		{"duplicate", `[{"index":0,"embedding":[1,2]},{"index":0,"embedding":[2,1]}]`, false},
		{"wrong dimensions", `[{"index":0,"embedding":[1,2]},{"index":1,"embedding":[1]}]`, false},
		{"missing index", `[{"embedding":[1,2]},{"index":1,"embedding":[2,1]}]`, false},
		{"zero", `[{"index":0,"embedding":[0,0]},{"index":1,"embedding":[2,1]}]`, false},
	} {
		t.Run(tt.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				_, _ = w.Write([]byte(`{"data":` + tt.data + `}`))
			}))
			defer server.Close()
			e := materialEncoder{endpoint: server.URL, model: "test", client: server.Client()}
			v, err := e.encode(context.Background(), []string{"one", "two"})
			if (err == nil) != tt.valid || tt.valid && (v[0][0] != 1 || v[1][1] != 1) {
				t.Fatal(v, err)
			}
		})
	}
}

func TestMaterialIndexRetriesWholeUnicodeRevision(t *testing.T) {
	ctx := context.Background()
	s := New(isolatedDatabase(t))
	if err := s.Migrate(ctx); err != nil {
		t.Fatal(err)
	}
	p, err := s.CreateProject(ctx, t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	text := strings.Repeat("断电后请恢复备份。🔋 ", 180)
	_, err = s.SaveMaterial(ctx, p.ID, "runbook", MaterialInput{Kind: "material", Title: "手册", Content: text, Source: "备份说明"})
	if err != nil {
		t.Fatal(err)
	}
	calls := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		if calls == 2 {
			w.WriteHeader(http.StatusServiceUnavailable)
			return
		}
		var req struct{ Input []string }
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Error(err)
			return
		}
		var data []map[string]any
		for i := range req.Input {
			data = append(data, map[string]any{"index": i, "embedding": []float32{0, 1}})
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"data": data})
	}))
	defer server.Close()
	s.embeddings = &materialEncoder{endpoint: server.URL, model: "local-v1", client: server.Client()}
	if err := s.indexMaterial(ctx, p.ID, "runbook", 1, 2); err == nil {
		t.Fatal("accepted incomplete encoding")
	}
	var count int
	if err := s.db.QueryRowContext(ctx, `SELECT count(*) FROM material_embeddings`).Scan(&count); err != nil || count != 0 {
		t.Fatal(count, err)
	}
	if err := s.indexMaterial(ctx, p.ID, "runbook", 1, 2); err != nil {
		t.Fatal(err)
	}
	rows, err := s.db.QueryContext(ctx, `SELECT e.start_char,e.end_char,substring(m.content FROM e.start_char+1 FOR e.end_char-e.start_char)
		FROM material_embeddings e JOIN project_materials m USING(project_id,key,revision) ORDER BY start_char`)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	runes := []rune(text)
	end := 0
	for rows.Next() {
		var start, next int
		var excerpt string
		if err := rows.Scan(&start, &next, &excerpt); err != nil {
			t.Fatal(err)
		}
		if start > end || next <= end || next > len(runes) || excerpt != string(runes[start:next]) {
			t.Fatalf("invalid Unicode passage [%d,%d)", start, next)
		}
		end = next
	}
	if err := rows.Err(); err != nil || end != len(runes) {
		t.Fatal(end, err)
	}
}
