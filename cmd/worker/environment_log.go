package main

import (
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"

	"forgeos/internal/store"
	"forgeos/internal/validation"
)

type environmentLog struct {
	archive    bool
	observe    func(ciLogFile)
	run        string
	secrets    []string
	credential func() (string, error)
	writer     *environmentOutputWriter
}

func (l *environmentLog) open(dir, environment string) (*os.File, error) {
	f, err := os.OpenFile(filepath.Join(dir, "startup.log"), os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
	if err == nil {
		credential := l.credential
		if credential == nil {
			credential = func() (string, error) { return "", nil }
		}
		l.writer = &environmentOutputWriter{dest: &cappedLog{dest: f, remaining: validation.LogLimit}, output: modelOutput{secrets: l.secrets, credential: credential}}
	}
	if err == nil && l.observe != nil {
		l.observe(ciLogFile{dir: dir, file: "startup.log", run: l.run, check: environment})
	}
	return f, err
}

func (l *environmentLog) finish(dir string, f *os.File) (*store.EnvironmentOutput, error) {
	if l.writer.pending != "" || l.writer.dropping {
		_, _ = l.writer.Write([]byte("\n"))
	}
	closeErr := f.Close()
	data, err := archivedFile(dir, "startup.log", int64(validation.MaxLogBytes))
	if err != nil {
		return nil, errors.Join(err, closeErr, l.writer.err)
	}
	out := &store.EnvironmentOutput{Bytes: len(data), Text: validation.DisplayLogTail(data, l.secrets)}
	if l.archive && closeErr == nil && l.writer.err == nil {
		out.SHA256 = validation.DigestBytes(data).SHA256
	}
	return out, errors.Join(closeErr, l.writer.err)
}

// Sanitize complete lines before writing even the node-local log.
type environmentOutputWriter struct {
	dest     io.Writer
	output   modelOutput
	pending  string
	dropping bool
	err      error
}

func (w *environmentOutputWriter) Write(data []byte) (int, error) {
	if w.err != nil {
		return 0, w.err
	}
	text := string(data)
	for text != "" {
		part, rest, newline := strings.Cut(text, "\n")
		text = rest
		if !w.dropping {
			if len(w.pending)+len(part) > 64<<10 {
				w.pending, w.dropping, w.output.privateKey = "", true, true
			} else {
				w.pending += part
			}
		}
		if newline {
			line := strings.TrimSuffix(w.pending, "\r")
			if w.dropping {
				line = "[redacted]"
			}
			safe, err := w.output.redact(line)
			if err == nil {
				_, err = io.WriteString(w.dest, safe)
			}
			if err != nil {
				w.err = err
				return 0, err
			}
			w.pending, w.dropping = "", false
		}
	}
	return len(data), nil
}
