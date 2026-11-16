package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"reflect"

	"forgeos/internal/validation"
)

func archivedFile(dir, name string, limit int64) ([]byte, error) {
	path := filepath.Join(dir, name)
	info, err := os.Lstat(path)
	if err != nil {
		return nil, err
	}
	if !info.Mode().IsRegular() || info.Size() > limit {
		return nil, fmt.Errorf("%s is not a regular file within the size limit", name)
	}
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	data, err := io.ReadAll(io.LimitReader(f, limit+1))
	if int64(len(data)) > limit {
		return nil, fmt.Errorf("%s exceeds the size limit", name)
	}
	return data, err
}

func readRun(in io.Reader, run *validation.Run) error {
	decoder := json.NewDecoder(in)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(run); err != nil {
		return err
	}
	if decoder.Decode(new(any)) != io.EOF {
		return errors.New("expected one CI run receipt")
	}
	return nil
}

// Use the independently stored receipt as the authority for both filenames and content.
func verifyCIRun(root string, in io.Reader, out io.Writer) error {
	var run validation.Run
	if err := readRun(in, &run); err != nil || !run.Valid() ||
		!validation.FullCommit(run.Base) || !validation.FullCommit(run.Commit) || run.Runner == "" || !filepath.IsAbs(root) {
		return errors.New("provide the stored CI run receipt and an absolute workspace root")
	}
	dir := filepath.Join(root, run.ID)
	info, err := os.Lstat(dir)
	if err != nil || !info.IsDir() {
		return fmt.Errorf("missing CI run directory %s", run.ID)
	}
	raw, err := archivedFile(dir, "run.json", 16<<20)
	if err != nil {
		return err
	}
	var local validation.Run
	if err := readRun(bytes.NewReader(raw), &local); err != nil || !reflect.DeepEqual(&run, &local) {
		return errors.New("run.json does not match the stored CI receipt")
	}
	verify := func(name string, want validation.FileDigest) error {
		data, err := archivedFile(dir, name, want.Bytes)
		if err != nil {
			return err
		}
		if validation.DigestBytes(data) != want {
			return fmt.Errorf("%s does not match the stored size and SHA-256", name)
		}
		return nil
	}
	var files int
	for _, c := range run.Checks {
		if c.LogDigest == nil {
			return fmt.Errorf("%s has no stored log digest", c.Log)
		}
		if err := verify(c.Log, *c.LogDigest); err != nil {
			return err
		}
		files++
		for _, o := range c.Outputs {
			if err := verify(o.File, validation.FileDigest{Bytes: o.Bytes, SHA256: o.SHA256}); err != nil {
				return err
			}
			files++
		}
	}
	_, err = fmt.Fprintf(out, "%s runner=%s execution=%d commit=%s verified_files=%d\n", run.ID, run.Runner, run.ExecutionID, run.Commit, files)
	return err
}
