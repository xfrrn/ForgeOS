package main

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"image"
	"image/draw"
	"image/png"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strconv"

	"forgeos/internal/store"
)

const screenshotLimit = 16 << 20

func cleanScreenshot(src []byte) ([]byte, error) {
	cfg, err := png.DecodeConfig(bytes.NewReader(src))
	if err != nil || cfg.Width < 1 || cfg.Height < 1 || cfg.Width > screenshotLimit/cfg.Height {
		return nil, errors.New("provide a PNG screenshot of at most 16 megapixels")
	}
	im, err := png.Decode(bytes.NewReader(src))
	if err != nil {
		return nil, errors.New("invalid PNG screenshot")
	}
	// Flatten hidden pixels and discard source metadata before archiving the visible evidence.
	flat := image.NewRGBA(im.Bounds())
	draw.Draw(flat, flat.Bounds(), image.White, image.Point{}, draw.Src)
	draw.Draw(flat, flat.Bounds(), im, im.Bounds().Min, draw.Over)
	var buf bytes.Buffer
	encoder := png.Encoder{CompressionLevel: png.BestSpeed}
	if err = encoder.Encode(&buf, flat); err != nil {
		return nil, err
	}
	if buf.Len() > screenshotLimit {
		return nil, errors.New("archived screenshot exceeds 16 MiB")
	}
	return buf.Bytes(), nil
}

func artifactPath(key string) (string, error) {
	root := os.Getenv("FORGEOS_ARTIFACT_ROOT")
	if root == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return "", err
		}
		root = filepath.Join(home, ".forgeos", "artifacts")
	}
	if !filepath.IsAbs(root) || !filepath.IsLocal(filepath.FromSlash(key)) {
		return "", errors.New("invalid artifact storage path")
	}
	return filepath.Join(root, filepath.FromSlash(key)), nil
}

func saveArtifact(key string, content []byte) error {
	name, err := artifactPath(key)
	if err != nil {
		return err
	}
	dir := filepath.Dir(name)
	if err = os.MkdirAll(dir, 0700); err != nil {
		return err
	}
	f, err := os.CreateTemp(dir, ".upload-*")
	if err != nil {
		return err
	}
	defer os.Remove(f.Name())
	if _, err = f.Write(content); err == nil {
		err = f.Sync()
	}
	closeErr := f.Close()
	if err != nil {
		return err
	}
	if closeErr != nil {
		return closeErr
	}
	return os.Rename(f.Name(), name)
}

func removeArtifact(key string) error {
	name, err := artifactPath(key)
	if err != nil {
		return err
	}
	root, err := artifactPath(".")
	if err != nil {
		return err
	}
	if name == root {
		return errors.New("expected a file beneath artifact storage")
	}
	for dir := filepath.Dir(name); dir != root; dir = filepath.Dir(dir) {
		info, err := os.Lstat(dir)
		if errors.Is(err, os.ErrNotExist) {
			return nil
		}
		if err != nil {
			return err
		}
		if !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
			return errors.New("unexpected directory in artifact storage")
		}
	}
	info, err := os.Lstat(name)
	if err == nil {
		if !info.Mode().IsRegular() {
			return errors.New("expected a regular archived file")
		}
		err = os.Remove(name)
	}
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	return err
}

func registerFeedbackArtifacts(mux *http.ServeMux, data *store.Store, owner, base string) {
	policy := authorized(owner, func(w http.ResponseWriter, r *http.Request) {
		id, ok := pathID(w, r)
		if !ok {
			return
		}
		var input struct {
			Seconds *int32 `json:"seconds"`
		}
		if r.Method == http.MethodPut {
			if !decode(w, r, &input) {
				return
			}
			if input.Seconds == nil || *input.Seconds < 0 {
				problem(w, 400, "provide nonnegative screenshot retention seconds")
				return
			}
		}
		seconds, err := data.ScreenshotRetention(r.Context(), id, input.Seconds)
		respond(w, 200, map[string]int32{"seconds": seconds}, err)
	})
	mux.HandleFunc("GET /projects/{id}/screenshot-retention", policy)
	mux.HandleFunc("PUT /projects/{id}/screenshot-retention", policy)
	path := base + "/{feedback}/artifacts"
	mux.HandleFunc("POST "+path, authorized(owner, func(w http.ResponseWriter, r *http.Request) {
		task, ok := pathID(w, r)
		if !ok {
			return
		}
		fid, err := strconv.ParseInt(r.PathValue("feedback"), 10, 64)
		if err != nil || fid <= 0 {
			problem(w, 400, "invalid feedback ID")
			return
		}
		if r.Header.Get("Content-Type") != "image/png" {
			problem(w, 415, "provide an annotated PNG screenshot")
			return
		}
		src, err := io.ReadAll(http.MaxBytesReader(w, r.Body, screenshotLimit))
		if err != nil {
			problem(w, 400, "cannot read screenshot within 16 MiB")
			return
		}
		content, err := cleanScreenshot(src)
		if err != nil {
			problem(w, 400, err.Error())
			return
		}
		sum := sha256.Sum256(content)
		a, err := data.SaveFeedbackScreenshot(r.Context(), task, r.PathValue("preview"), fid, hex.EncodeToString(sum[:]), int64(len(content)), func(key string) error { return saveArtifact(key, content) })
		respond(w, 200, a, err)
	}))
	mux.HandleFunc("GET "+path+"/{artifact}", authorized(owner, func(w http.ResponseWriter, r *http.Request) {
		task, ok := pathID(w, r)
		if !ok {
			return
		}
		fid, err := strconv.ParseInt(r.PathValue("feedback"), 10, 64)
		id, parseErr := strconv.ParseInt(r.PathValue("artifact"), 10, 64)
		if err != nil || parseErr != nil || fid <= 0 || id <= 0 {
			problem(w, 400, "invalid artifact or feedback ID")
			return
		}
		a, err := data.FeedbackArtifact(r.Context(), task, r.PathValue("preview"), fid, id)
		if err != nil {
			respond(w, 0, nil, err)
			return
		}
		name, err := artifactPath(a.Key)
		if err != nil {
			problem(w, 503, "artifact storage unavailable")
			return
		}
		f, err := os.Open(name)
		if err != nil {
			if errors.Is(err, os.ErrNotExist) {
				_, stateErr := data.FeedbackArtifact(r.Context(), task, r.PathValue("preview"), fid, id)
				if stateErr != nil {
					respond(w, 0, nil, stateErr)
					return
				}
			}
			problem(w, 503, "archived file unavailable")
			return
		}
		defer f.Close()
		content, err := io.ReadAll(io.LimitReader(f, screenshotLimit+1))
		sum := sha256.Sum256(content)
		if err != nil || int64(len(content)) != a.Bytes || hex.EncodeToString(sum[:]) != a.SHA256 {
			problem(w, 503, "artifact integrity check failed")
			return
		}
		w.Header().Set("Content-Type", a.MIME)
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("Cache-Control", "no-store")
		w.Header().Set("Content-Length", strconv.FormatInt(a.Bytes, 10))
		w.Write(content)
	}))
}
