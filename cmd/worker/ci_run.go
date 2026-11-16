package main

import (
	"archive/tar"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"

	"forgeos/internal/localrepo"
	"forgeos/internal/store"
	"forgeos/internal/validation"
)

type ciRequest struct {
	ExecutionID int64              `json:"-"`
	Repo        string             `json:"repo"`
	Base        string             `json:"base_commit"`
	Commit      string             `json:"commit"`
	Level       string             `json:"level"`
	Policy      *validation.Policy `json:"policy,omitempty"`
}

var imageID = regexp.MustCompile("^sha256:[a-f0-9]{64}$")

func (w *worker) validateIssueCandidate(ctx context.Context, execID int64, repo, workspace, candidate string, spec *store.IssueSpec) (store.ValidationResult, error) {
	base, profile := spec.BaseCommit, spec.Validation
	if profile != "auto" && !validation.ValidLevel(profile) {
		return validateCandidate(ctx, workspace, base, candidate, profile)
	}
	report := store.ValidationResult{Profile: profile, Commit: candidate, ExitCode: 1}
	run, err := runLocalCI(ctx, ciRequest{ExecutionID: execID, Repo: repo, Base: base, Commit: candidate, Level: profile, Policy: spec.Policy}, w.repoRoot, w.workspaceRoot, w.id)
	if err != nil {
		report.Output = err.Error()
		return report, err
	}
	report.Run = &run
	if run.Failure != "" {
		report.Output = run.Failure + ": " + run.Detail
		return report, errors.New(report.Output)
	}
	report.ExitCode = 0
	report.Output = fmt.Sprintf("%d declared checks completed; all required checks passed", len(run.Checks))
	return report, nil
}

func runLocalCI(ctx context.Context, req ciRequest, root, work, runner string) (validation.Run, error) {
	run := validation.Run{ExecutionID: req.ExecutionID, Base: req.Base, Commit: req.Commit, Level: req.Level, Runner: runner, Started: time.Now().UTC(), Checks: []validation.CheckResult{}}
	if os.Geteuid() == 0 || runner == "" || !filepath.IsAbs(work) {
		return run, errors.New("configure a non-root runner with an absolute workspace root and identity")
	}
	repo, err := localrepo.ResolveRepo(ctx, root, req.Repo)
	if err != nil {
		return run, err
	}
	if (req.Level != "auto" && !validation.ValidLevel(req.Level)) || (req.Policy != nil && req.Level != "auto") || !validation.FullCommit(req.Base) || !validation.FullCommit(req.Commit) || req.Base == req.Commit {
		return run, errors.New("provide a level and two distinct full commit hashes")
	}
	if out, err := localrepo.Git(ctx, repo, "cat-file", "-t", req.Commit); err != nil || string(out) != "commit\n" {
		return run, errors.New("candidate must be a commit")
	}
	if _, err = localrepo.Git(ctx, repo, "merge-base", "--is-ancestor", req.Base, req.Commit); err != nil {
		return run, errors.New("candidate must descend from the configuration base")
	}
	if err = os.MkdirAll(work, 0700); err != nil {
		return run, err
	}
	work, err = filepath.EvalSymlinks(work)
	if err != nil {
		return run, err
	}
	if rel, err := filepath.Rel(repo, work); err == nil && (rel == "." || filepath.IsLocal(rel)) {
		return run, errors.New("CI output must be outside the project checkout")
	}
	dir, err := os.MkdirTemp(work, "ci-run-")
	if err != nil {
		return run, err
	}
	run.ID = filepath.Base(dir)
	finish := func(category, detail string) (validation.Run, error) {
		run.Finished = time.Now().UTC()
		run.Failure = category
		run.Detail = detail
		body, err := json.MarshalIndent(run, "", "  ")
		if err == nil {
			err = os.WriteFile(filepath.Join(dir, "run.json"), body, 0600)
		}
		return run, err
	}
	if req.Level == "auto" {
		var policy validation.Policy
		if req.Policy != nil {
			policy = *req.Policy
		}
		decision, err := validation.Assess(ctx, repo, req.Base, req.Commit, policy)
		if err != nil {
			return finish("CONFIG_FAILURE", "cannot assess candidate using the pinned acceptance rules")
		}
		run.Decision, run.Level = &decision, decision.Level
	}
	plan, err := validation.ReadPlan(ctx, repo, req.Base, run.Level)
	if err != nil {
		return finish("CONFIG_FAILURE", "cannot load validation.yaml at the pinned base")
	}
	run.Profile, run.ConfigVersion, run.ConfigBlob = plan.Profile, plan.Version, plan.ConfigBlob
	if !plan.Ready {
		return finish("CONFIG_FAILURE", "validation plan lacks "+strings.Join(plan.Missing, ", "))
	}
	docker, err := openDocker(ctx)
	if err != nil {
		return finish("INFRA_FAILURE", err.Error())
	}
	image := os.Getenv("FORGEOS_CI_IMAGE")
	if image == "" {
		image = "golang:1.25-alpine"
	}
	if strings.HasPrefix(image, "-") || strings.ContainsAny(image, " \t\r\n") {
		return finish("CONFIG_FAILURE", "invalid local CI image")
	}
	pinned, err := docker.command(ctx, "image", "inspect", "--format", "{{.Id}}", image).Output()
	if err != nil || !imageID.MatchString(strings.TrimSpace(string(pinned))) {
		return finish("TEST_ENV_FAILURE", "configured CI image is unavailable locally")
	}
	run.Image = strings.TrimSpace(string(pinned))
	snapshot, release, err := ciSnapshot(ctx, repo, req.Commit)
	if err != nil {
		return finish("INFRA_FAILURE", err.Error())
	}
	defer release()
	var failure, detail string
	remaining := int64(64 << 20)
	for _, c := range plan.Checks {
		if ctx.Err() != nil {
			return finish("TIMEOUT", "validation interrupted before all checks completed")
		}
		result := runCheck(ctx, docker, run.Image, snapshot, dir, c, &remaining)
		run.Checks = append(run.Checks, result)
		if result.Required && result.Failure != "" && failure == "" {
			failure, detail = result.Failure, "required check "+c.Name+" did not pass"
		}
	}
	return finish(failure, detail)
}

func runCheck(parent context.Context, docker localDocker, image, snapshot, dir string, c validation.PlannedCheck, remaining *int64) (result validation.CheckResult) {
	result = validation.CheckResult{Name: c.Name, Required: c.Required == nil || *c.Required, Started: time.Now().UTC(), Status: "FAILED", Failure: "INFRA_FAILURE", Log: c.Name + ".log"}
	defer func() { result.Finished = time.Now().UTC() }()
	logFile, err := os.OpenFile(filepath.Join(dir, result.Log), os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
	if err != nil {
		return
	}
	logger := &cappedLog{dest: logFile, remaining: validation.LogLimit}
	defer func() {
		closeErr := logFile.Close()
		data, err := archivedFile(dir, result.Log, int64(validation.MaxLogBytes))
		if err != nil || closeErr != nil {
			result.Status, result.Failure = "FAILED", "INFRA_FAILURE"
			return
		}
		digest := validation.DigestBytes(data)
		result.LogDigest = &digest
	}()
	options, name, err := ciContainerOptions(snapshot)
	if err != nil {
		return
	}
	defer func() {
		ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
		defer cancel()
		if err := docker.command(ctx, "rm", "-fv", name).Run(); err != nil && result.Status == "PASSED" {
			result.Status, result.Failure = "FAILED", "INFRA_FAILURE"
		}
	}()
	args := append([]string{"run", "-d", "--rm"}, options...)
	// Bound the container lifetime even if the worker process is killed before its cleanup.
	start := docker.command(parent, append(args, "--entrypoint", "sleep", image, strconv.Itoa(c.TimeoutSeconds+120))...)
	start.Stderr = logger
	if err = start.Run(); err != nil {
		return
	}
	prep := docker.command(parent, "exec", name, "/bin/sh", "-c", "mkdir /tmp/project && cp -R /src/. /tmp/project && rm -f /tmp/project/.git")
	prep.Stdout, prep.Stderr = logger, logger
	if err = prep.Run(); err != nil {
		result.Failure = "TEST_ENV_FAILURE"
		return
	}
	cwd := path.Join("/tmp/project", c.WorkingDirectory)
	ctx, cancel := context.WithTimeout(parent, time.Duration(c.TimeoutSeconds)*time.Second)
	defer cancel()
	check := docker.command(ctx, append([]string{"exec", "--workdir", cwd, name}, c.Command...)...)
	check.Stdout, check.Stderr = logger, logger
	err = check.Run()
	if ctx.Err() != nil {
		result.Failure = "TIMEOUT"
		return
	}
	if err != nil {
		var status *exec.ExitError
		if !errors.As(err, &status) {
			return
		}
		alive, err := docker.command(parent, "inspect", "--format", "{{.State.Running}}", name).Output()
		if err != nil || strings.TrimSpace(string(alive)) != "true" {
			return
		}
		code := status.ExitCode()
		result.ExitCode = &code
		switch {
		case code == 126 || code == 127:
			result.Failure = "TEST_ENV_FAILURE"
		case c.Type == "build":
			result.Failure = "BUILD_FAILURE"
		case c.Type == "security":
			result.Failure = "SECURITY_FAILURE"
		default:
			result.Failure = "CODE_FAILURE"
		}
	} else {
		code := 0
		result.ExitCode = &code
		result.Status, result.Failure = "PASSED", ""
	}
	for _, group := range []struct {
		kind  string
		paths []string
	}{{"report", c.Reports}, {"artifact", c.Artifacts}} {
		for _, source := range group.paths {
			file := fmt.Sprintf("%s-%d.bin", c.Name, len(result.Outputs)+1)
			out, err := collectOutput(parent, docker, name, cwd, source, filepath.Join(dir, file), *remaining)
			if err != nil {
				fmt.Fprintf(logger, "\nCannot collect declared %s %s\n", group.kind, source)
				if result.Status == "PASSED" {
					result.Status, result.Failure = "FAILED", "TEST_ENV_FAILURE"
				}
				continue
			}
			out.Kind, out.Source, out.File = group.kind, source, file
			result.Outputs = append(result.Outputs, out)
			*remaining -= out.Bytes
		}
	}
	return
}

func collectOutput(parent context.Context, docker localDocker, name, cwd, source, destination string, remaining int64) (validation.Output, error) {
	var output validation.Output
	ctx, cancel := context.WithTimeout(parent, 20*time.Second)
	defer cancel()
	// Read inside the running mount namespace, where tmpfs outputs are visible.
	cmd := docker.command(ctx, "exec", name, "tar", "-C", cwd, "-cf", "-", "./"+source)
	reader, err := cmd.StdoutPipe()
	if err != nil {
		return output, err
	}
	if err = cmd.Start(); err != nil {
		return output, err
	}
	waited := false
	defer func() {
		reader.Close()
		cancel()
		if !waited {
			_ = cmd.Wait()
		}
	}()
	archive := tar.NewReader(reader)
	h, err := archive.Next()
	if err != nil || strings.TrimPrefix(h.Name, "./") != source || h.Typeflag != tar.TypeReg || h.Size < 0 || h.Size > min(remaining, 8<<20) {
		return output, errors.New("output must be a bounded regular file")
	}
	f, err := os.OpenFile(destination, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
	if err != nil {
		return output, err
	}
	hash := sha256.New()
	size, err := io.CopyN(io.MultiWriter(f, hash), archive, h.Size)
	closeErr := f.Close()
	_, end := archive.Next()
	if err != nil || closeErr != nil || end != io.EOF {
		os.Remove(destination)
		return output, errors.New("incomplete output")
	}
	// Drain tar padding before waiting for Docker to acknowledge a complete copy.
	_, drainErr := io.Copy(io.Discard, reader)
	waitErr := cmd.Wait()
	waited = true
	if drainErr != nil || waitErr != nil {
		os.Remove(destination)
		return output, errors.New("Docker did not confirm output transfer")
	}
	output.Bytes, output.SHA256 = size, hex.EncodeToString(hash.Sum(nil))
	return output, nil
}

func runCICandidate(ctx context.Context, in io.Reader, out io.Writer) error {
	raw, err := io.ReadAll(io.LimitReader(in, (16<<10)+1))
	if err != nil || len(raw) > 16<<10 {
		return errors.New("invalid CI request size")
	}
	var req ciRequest
	if err = json.Unmarshal(raw, &req); err != nil {
		return errors.New("invalid CI request")
	}
	run, err := runLocalCI(ctx, req, os.Getenv("FORGEOS_REPO_ROOT"), os.Getenv("FORGEOS_WORKSPACE_ROOT"), os.Getenv("FORGEOS_WORKER_ID"))
	if err != nil {
		return err
	}
	if err = json.NewEncoder(out).Encode(run); err != nil {
		return err
	}
	if run.Failure != "" {
		return fmt.Errorf("validation ended with %s", run.Failure)
	}
	return nil
}
