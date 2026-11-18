package main

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"forgeos/internal/store"
)

const nativePatchMethod = "item/fileChange/requestApproval"

type nativePatchProposal struct {
	changes []nativeFileChange
	files   []store.NativePatchFile
}

func (c *turnInputClient) inspectPatch(workspace string, changes []nativeFileChange, allowed []string) ([]store.NativePatchFile, error) {
	token, err := c.worker.credential()
	if err != nil {
		return nil, err
	}
	return prepareNativePatch(workspace, changes, allowed, append(outputSecrets(c.lease), token)), nil
}

// Native paths are resolved by the tool process. Accept only their canonical spelling in this worktree.
func patchTarget(workspace, target string, allowed []string, existing bool) (string, string, bool) {
	if !filepath.IsAbs(target) {
		target = filepath.FromSlash(target)
		if !filepath.IsLocal(target) || filepath.Clean(target) != target {
			return "", "", false
		}
		target = filepath.Join(workspace, target)
	}
	if filepath.Clean(target) != filepath.FromSlash(target) {
		return "", "", false
	}
	rel, err := filepath.Rel(workspace, target)
	path := filepath.ToSlash(rel)
	if err != nil || !filepath.IsLocal(rel) || !store.ValidPatchPath(path) || !slices.Contains(allowed, path) {
		return "", "", false
	}
	root, err := filepath.EvalSymlinks(workspace)
	if err != nil {
		return "", "", false
	}
	if same, err := filepath.Rel(root, workspace); err != nil || same != "." {
		return "", "", false
	}
	current := workspace
	finalPath := filepath.Join(workspace, rel)
	for _, segment := range strings.Split(path, "/") {
		current = filepath.Join(current, segment)
		info, err := os.Lstat(current)
		if os.IsNotExist(err) && !existing {
			return path, "", true
		}
		if err != nil {
			return "", "", false
		}
		if current != finalPath {
			if !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
				return "", "", false
			}
			resolved, err := filepath.EvalSymlinks(current)
			if err != nil {
				return "", "", false
			}
			if same, err := filepath.Rel(resolved, current); err != nil || same != "." {
				return "", "", false
			}
			continue
		}
		if !existing || !info.Mode().IsRegular() {
			return "", "", false
		}
		f, err := os.Open(current)
		if err != nil {
			return "", "", false
		}
		defer f.Close()
		stat, err := f.Stat()
		if err != nil || !os.SameFile(info, stat) || !singlePatchLink(f, stat) {
			return "", "", false
		}
		h := sha256.New()
		if _, err = io.Copy(h, f); err != nil {
			return "", "", false
		}
		return path, hex.EncodeToString(h.Sum(nil)), true
	}
	return "", "", false
}

func prepareNativePatch(workspace string, changes []nativeFileChange, allowed, secrets []string) []store.NativePatchFile {
	if len(changes) == 0 || len(changes) > 32 {
		return nil
	}
	files := make([]store.NativePatchFile, 0, len(changes))
	for _, c := range changes {
		var kind struct {
			Type string `json:"type"`
			Move string `json:"move_path"`
		}
		decoder := json.NewDecoder(bytes.NewReader(c.Kind))
		decoder.DisallowUnknownFields()
		if decoder.Decode(&kind) != nil {
			return nil
		}
		path, before, ok := patchTarget(workspace, c.Path, allowed, kind.Type != "add")
		if !ok {
			return nil
		}
		file := store.NativePatchFile{Path: path, Op: kind.Type, Diff: c.Diff, Before: before}
		if kind.Move != "" {
			file.MoveTo, _, ok = patchTarget(workspace, kind.Move, allowed, false)
			if !ok {
				return nil
			}
		}
		files = append(files, file)
	}
	if !store.ValidNativePatch(files, secrets) {
		return nil
	}
	return files
}
