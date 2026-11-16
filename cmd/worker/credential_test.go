package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestWorkerReadsRotatedCredentialFile(t *testing.T) {
	file := filepath.Join(t.TempDir(), "worker-token")
	w := worker{tokenFile: file}
	for _, raw := range []string{"old\r\n", "new\n", "", "two\nlines", " space", strings.Repeat("a", 16385)} {
		if err := os.WriteFile(file, []byte(raw), 0600); err != nil {
			t.Fatal(err)
		}
		got, err := w.credential()
		switch raw {
		case "old\r\n", "new\n":
			if err != nil || got != strings.TrimSpace(raw) {
				t.Fatal("credential not reread")
			}
		default:
			if err == nil || got != "" {
				t.Fatal("malformed credential accepted")
			}
		}
	}
}
