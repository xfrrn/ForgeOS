package localrepo

import (
	"bufio"
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"io"
	"os/exec"
	"unicode/utf8"
)

var ErrDocumentBlobMissing = errors.New("document blob unavailable")

type DocumentParts struct {
	Count   int
	Bytes   int64
	Digest  string
	Problem string
}

// ReadDocumentParts streams one immutable blob. The caller publishes parts only after Git exits successfully.
func ReadDocumentParts(ctx context.Context, repo, blob string, save func(int, int64, string) error) (DocumentParts, error) {
	if !objectID.MatchString(blob) {
		return DocumentParts{}, errors.New("invalid document blob")
	}
	run, stop := context.WithCancel(ctx)
	defer stop()
	cmd := GitCommand(run, repo, "cat-file", "blob", blob)
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return DocumentParts{}, err
	}
	if err = cmd.Start(); err != nil {
		return DocumentParts{}, err
	}
	result, err := splitDocument(stdout, save)
	if err != nil || result.Problem == "non_text" {
		stop()
	}
	exited := cmd.Wait()
	if err == nil && result.Problem != "non_text" {
		err = exited
		if exited != nil && ctx.Err() == nil {
			_, verify := Git(ctx, repo, "rev-parse", "--verify", "--quiet", blob+"^{blob}")
			var exit *exec.ExitError
			if errors.As(verify, &exit) && exit.ExitCode() == 1 {
				err = ErrDocumentBlobMissing
			}
		}
	}
	return result, err
}

func splitDocument(src io.Reader, save func(int, int64, string) error) (DocumentParts, error) {
	const limit = 64 << 10
	reader := bufio.NewReaderSize(src, limit+1)
	digest := sha256.New()
	var result DocumentParts
	hasText := false
	for {
		window, err := reader.Peek(limit + 1)
		if err != nil && err != io.EOF {
			return result, err
		}
		if len(window) == 0 {
			break
		}
		size := len(window)
		if size > limit {
			size = limit
			if newline := bytes.LastIndexByte(window[:limit], '\n'); newline >= 0 {
				size = newline + 1
			} else {
				for size > 0 && !utf8.RuneStart(window[size]) {
					size--
				}
			}
		}
		part := window[:size]
		if size == 0 || !utf8.Valid(part) || bytes.IndexByte(part, 0) >= 0 {
			result.Problem = "non_text"
			return result, nil
		}
		hasText = hasText || len(bytes.TrimSpace(part)) > 0
		result.Count++
		if err := save(result.Count, result.Bytes, string(part)); err != nil {
			return result, err
		}
		digest.Write(part)
		result.Bytes += int64(size)
		if _, err := reader.Discard(size); err != nil {
			return result, err
		}
	}
	if !hasText {
		result.Problem = "empty"
	}
	result.Digest = hex.EncodeToString(digest.Sum(nil))
	return result, nil
}
