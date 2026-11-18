package main

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"strings"
	"testing"

	"forgeos/internal/store"
)

func testFileChangeFixture(app bool) []map[string]any {
	changes := []map[string]any{
		{"path": "docs/guide.md", "kind": "add"},
		{"path": "docs/old.md", "kind": "update"},
		{"path": "docs/deleted.md", "kind": "delete"},
	}
	if app {
		changes[0]["kind"] = map[string]any{"type": "add"}
		changes[0]["diff"] = "+public guide\n+password=private-value\n+private-node-credential\n"
		changes[1]["kind"] = map[string]any{"type": "update", "move_path": "docs/new.md"}
		changes[1]["diff"] = "-old\n+new\n"
		changes[2]["kind"] = map[string]any{"type": "delete"}
		changes[2]["diff"] = "-deleted\n"
	}
	return changes
}

func TestNativeFileChangeResultsReachWindowAndArchive(t *testing.T) {
	bin, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	for _, app := range []bool{false, true} {
		for _, phase := range []string{"completed", "failed", "declined"} {
			name := "cli-" + phase
			if app {
				name = "appserver-" + phase
			}
			t.Run(name, func(t *testing.T) {
				w, _, _ := archiveTestProcess(t)
				prompt := "SIMULATED_FILE_CHANGE"
				if phase == "failed" {
					prompt += "_FAILED"
				}
				expected := phase
				if phase == "declined" {
					prompt += "_DECLINED"
					expected = "failed"
				}
				adapter := codexCLI{bin: bin, sandbox: "read-only", appServer: app}
				job := store.Job{LeaseToken: "private-lease", RecordToolActivity: true, RecordToolOutput: true}
				p := w.processLog(context.Background(), job, "codex", "1.2.3")
				if _, err := adapter.runTurn(context.Background(), p, t.TempDir(), prompt, ""); err != nil {
					t.Fatal(err)
				}
				got, err := scanHarnessProcess(w.journal.QueryRow(readHarnessProcess + " ORDER BY instance DESC LIMIT 1"))
				if err != nil || !got.Valid() || got.ToolOutput == nil || got.ToolOutput.Tool != "file_change" || got.ToolOutput.Seq != 2 || got.Tools[1].Phase != expected {
					t.Fatal(got, err)
				}
				text := got.ToolOutput.Text
				for _, part := range []string{`add "docs/guide.md"`, `update "docs/old.md"`, `delete "docs/deleted.md"`} {
					if !strings.Contains(text, part) {
						t.Fatal("missing change", part, text)
					}
				}
				if app {
					if !strings.Contains(text, `-> "docs/new.md"`) || !strings.Contains(text, "+public guide\n[redacted]\n[redacted]\n") {
						t.Fatal(text)
					}
				} else if strings.Contains(text, "+public") {
					t.Fatal("CLI invented a patch")
				}
				var archive bytes.Buffer
				if err := exportProcessArchive(w.journal, got.Instance, &archive); err != nil {
					t.Fatal(err)
				}
				for _, private := range []string{"private-input", "private-value", "private-node-credential"} {
					if strings.Contains(archive.String(), private) {
						t.Fatal("private content in archive", private)
					}
				}
				if !strings.Contains(archive.String(), "docs/guide.md") {
					t.Fatal("change missing from archive")
				}
			})
		}
	}
}

func TestFileChangeOutputSkipsUnsupportedKindsAndQuotesPaths(t *testing.T) {
	got := fileChangeOutput([]nativeFileChange{
		{Path: "ignored", Kind: json.RawMessage(`"future"`), Diff: "must-not-save"},
		{Path: "ignored", Kind: json.RawMessage(`null`), Diff: "must-not-save"},
		{Path: "", Kind: json.RawMessage(`"add"`)},
		{Path: "line\nbreak.md", Kind: json.RawMessage(`"add"`)},
	})
	if got != `add "line\nbreak.md"`+"\n" {
		t.Fatal(got)
	}
}

func TestFileChangeOutputRequiresCapture(t *testing.T) {
	w, p, _ := archiveTestProcess(t)
	p.toolOutput = nil
	raw := []byte("{\"item\":{\"id\":\"change\",\"type\":\"file_change\",\"status\":\"completed\",\"changes\":[{\"path\":\"not-captured.md\",\"kind\":\"add\"}]}}")
	if err := p.codexTool("item.completed", raw); err != nil {
		t.Fatal(err)
	}
	got, err := scanHarnessProcess(w.journal.QueryRow(readHarnessProcess+" WHERE instance=?", p.id))
	if err != nil || got.ToolOutput != nil {
		t.Fatal(got, err)
	}
	var archive bytes.Buffer
	if err := exportProcessArchive(w.journal, p.id, &archive); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(archive.String(), "not-captured") {
		t.Fatal("uncaptured payload entered archive")
	}
}
