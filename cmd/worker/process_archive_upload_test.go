package main

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"time"

	"forgeos/internal/store"
)

func TestProcessArchiveUploadResumesInsideRecord(t *testing.T) {
	w, p, path := archiveTestProcess(t)
	if err := p.modelMessage(strings.Repeat("archive line\n", 34000)); err != nil {
		t.Fatal(err)
	}
	var exported bytes.Buffer
	if err := processLogsCommand([]string{"export", "--journal", path, "--instance", fmt.Sprint(p.id)}, &exported); err != nil {
		t.Fatal(err)
	}
	lines := bytes.SplitAfter(exported.Bytes(), []byte("\n"))
	raw := bytes.Join(lines[1:len(lines)-2], nil)
	if len(raw) <= store.ProcessArchiveChunkSize {
		t.Fatal("expected more than one block")
	}
	offset := store.ProcessArchiveChunkSize
	server := httptest.NewServer(http.HandlerFunc(func(rw http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer "+w.token || r.URL.Path != fmt.Sprintf("/internal/tasks/2/executions/3/processes/%d/archive", p.id) {
			t.Error("incorrect upload identity")
		}
		if r.Method == http.MethodHead {
			rw.Header().Set("ForgeOS-Archive-Offset", strconv.Itoa(offset))
			rw.WriteHeader(204)
			return
		}
		n, err := strconv.Atoi(r.URL.Query().Get("offset"))
		if err != nil || n != offset {
			t.Error("incorrect offset", n, offset)
			rw.WriteHeader(409)
			return
		}
		part, err := io.ReadAll(r.Body)
		if err != nil || !bytes.Equal(part, raw[offset:min(offset+store.ProcessArchiveChunkSize, len(raw))]) {
			t.Error("incorrect retained bytes")
		}
		offset += len(part)
		rw.WriteHeader(204)
	}))
	defer server.Close()
	w.api, w.client = server.URL, server.Client()
	endpoint := fmt.Sprintf("%s/internal/tasks/2/executions/3/processes/%d/archive", w.api, p.id)
	if err := w.uploadProcessArchive(context.Background(), endpoint, p.id, int64(len(raw))); err != nil || offset != len(raw) {
		t.Fatal("archive transfer failed", err, offset)
	}
	if err := w.uploadProcessArchive(context.Background(), endpoint, p.id, int64(len(raw))); err != nil {
		t.Fatal(err)
	}
	_, err := w.journal.Exec("INSERT INTO attempts(lease_hash,execution_id,task_id,workspace,outcome,settled_at) VALUES(?,3,2,'test','confirmed','2000-01-01')", p.lease[:])
	if err != nil {
		t.Fatal(err)
	}
	if _, err = w.journal.Exec("UPDATE harness_processes SET exited_at='2000-01-01',exit_code=0 WHERE instance=?", p.id); err != nil {
		t.Fatal(err)
	}
	if _, err = w.journal.Exec("INSERT INTO process_log_uploads(instance) VALUES(?)", p.id); err != nil {
		t.Fatal(err)
	}
	if n, err := pruneProcessArchives(w.journal, time.Now()); err != nil || n != 0 {
		t.Fatal("pending transfer pruned", n, err)
	}
	if _, err = w.journal.Exec("DELETE FROM process_log_uploads WHERE instance=?", p.id); err != nil {
		t.Fatal(err)
	}
	if n, err := pruneProcessArchives(w.journal, time.Now()); err != nil || n != 1 {
		t.Fatal("confirmed transfer was not eligible for pruning", n, err)
	}
}
