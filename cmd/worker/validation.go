package main

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/csv"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"forgeos/internal/localrepo"
	"forgeos/internal/store"
	"forgeos/internal/validation"
)

// Validate the committed Git objects. A file left outside the commit cannot make a check pass.
func validateCandidate(ctx context.Context, workspace, base, candidate, profile string) (store.ValidationResult, error) {
	result := store.ValidationResult{Profile: profile, Commit: candidate, ExitCode: 1}
	head, err := localrepo.Git(ctx, workspace, "rev-parse", "--verify", "HEAD^{commit}")
	if err != nil || strings.TrimSpace(string(head)) != candidate || candidate == base {
		result.Output = "candidate does not match the checked-out revision"
		return result, errors.New(result.Output)
	}
	if _, err = localrepo.Git(ctx, workspace, "-c", "core.whitespace=trailing-space,space-before-tab,cr-at-eol", "diff", "--no-ext-diff", "--no-textconv", "--check", base, candidate, "--"); err != nil {
		result.Output = "git diff --check failed on the candidate commit"
		return result, errors.New(result.Output)
	}
	switch profile {
	case "docs":
		err = checkDocs(ctx, workspace, base, candidate)
		result.Output = "git diff --check passed; every changed path is a regular documentation file"
	case "go-test":
		err = checkGo(ctx, workspace, candidate)
		result.Output = "git diff --check and go test -count=1 ./... passed"
	default:
		err = errors.New("unsupported validation profile")
	}
	if err != nil {
		result.Output = err.Error()
		var exit *exec.ExitError
		if errors.As(err, &exit) && exit.ExitCode() > 0 {
			result.ExitCode = exit.ExitCode()
		}
		return result, err
	}
	result.ExitCode = 0
	return result, nil
}

func checkDocs(ctx context.Context, repo, base, candidate string) error {
	// With renames disabled, each NUL-separated metadata record has exactly one path.
	out, err := localrepo.Git(ctx, repo, "diff", "--raw", "--no-renames", "--no-ext-diff", "-z", base, candidate, "--")
	if err != nil || len(out) == 0 {
		return errors.New("docs check requires a non-empty committed change")
	}
	parts := bytes.Split(bytes.TrimSuffix(out, []byte{0}), []byte{0})
	if len(parts)%2 != 0 {
		return errors.New("cannot read candidate path list")
	}
	for i := 0; i < len(parts); i += 2 {
		modes := strings.Fields(strings.TrimPrefix(string(parts[i]), ":"))
		if len(modes) != 5 || (modes[0] != "100644" && modes[0] != "000000") || (modes[1] != "100644" && modes[1] != "000000") {
			return errors.New("docs check accepts only regular, non-executable files")
		}
		switch strings.ToLower(filepath.Ext(string(parts[i+1]))) {
		case ".md", ".rst", ".adoc", ".txt":
		default:
			return errors.New("docs check found a change outside .md, .rst, .adoc or .txt files")
		}
	}
	return nil
}

type localDocker struct{ bin, host string }

func openDocker(ctx context.Context) (localDocker, error) {
	docker, err := exec.LookPath("docker")
	if err != nil {
		return localDocker{}, errors.New("local CI requires a local Docker engine")
	}
	host := os.Getenv("DOCKER_HOST")
	if host == "" || os.Getenv("DOCKER_CONTEXT") != "" {
		endpoint, err := exec.CommandContext(ctx, docker, "context", "inspect", "--format", "{{.Endpoints.docker.Host}}").Output()
		if err != nil {
			return localDocker{}, errors.New("cannot resolve the local Docker context")
		}
		host = strings.TrimSpace(string(endpoint))
	}
	if !strings.HasPrefix(host, "unix:///") && !strings.HasPrefix(host, "npipe:////./pipe/") {
		return localDocker{}, errors.New("local CI requires a local Docker socket or Windows named pipe")
	}
	return localDocker{docker, host}, nil
}

func (d localDocker) command(ctx context.Context, args ...string) *exec.Cmd {
	return exec.CommandContext(ctx, d.bin, append([]string{"--host", d.host}, args...)...)
}

func ciSnapshot(ctx context.Context, repo, candidate string) (string, func(), error) {
	tmp, err := os.MkdirTemp("", "forgeos-validation-")
	if err != nil {
		return "", nil, errors.New("cannot prepare validation workspace")
	}
	snapshot := filepath.Join(tmp, "source")
	if err := checkGitFilters(ctx, repo); err != nil {
		os.Remove(tmp)
		return "", nil, err
	}
	if _, err = localrepo.Git(ctx, repo, "worktree", "add", "--quiet", "--detach", snapshot, candidate); err != nil {
		os.Remove(tmp)
		return "", nil, errors.New("cannot check out the candidate for local CI")
	}
	return snapshot, func() {
		cleanup, cancel := context.WithTimeout(context.Background(), 15*time.Second)
		defer cancel()
		_, _ = localrepo.Git(cleanup, repo, "worktree", "remove", "--force", snapshot)
		os.Remove(tmp)
	}, nil
}

func sandboxContainerOptions(snapshot, network string) ([]string, string, error) {
	var nonce [16]byte
	if _, err := rand.Read(nonce[:]); err != nil {
		return nil, "", errors.New("cannot allocate validation run")
	}
	name := "forgeos-ci-" + hex.EncodeToString(nonce[:])
	var mount strings.Builder
	fields := csv.NewWriter(&mount)
	if err := fields.Write([]string{"type=bind", "source=" + snapshot, "target=/src", "readonly"}); err != nil {
		return nil, "", errors.New("could not prepare CI workspace mount")
	}
	fields.Flush()
	return []string{"--name", name, "--pull=never", "--network=" + network, "--log-driver=none",
		"--read-only", "--cap-drop=ALL", "--security-opt=no-new-privileges", "--user=65534:65534",
		"--pids-limit=256", "--memory=2g", "--cpus=2", "--tmpfs=/tmp:rw,exec,mode=1777",
		"--mount", strings.TrimSuffix(mount.String(), "\n"), "--workdir=/src", "--env=HOME=/tmp", "--env=GOCACHE=/tmp/go-build",
		"--env=GOPATH=/tmp/go", "--env=GOPROXY=off", "--env=GOTOOLCHAIN=local", "--env=GOSUMDB=off"}, name, nil
}

func checkGo(ctx context.Context, repo, candidate string) error {
	docker, err := openDocker(ctx)
	if err != nil {
		return err
	}
	image := os.Getenv("FORGEOS_GO_IMAGE")
	if image == "" {
		image = "golang:1.25-alpine"
	}
	if strings.HasPrefix(image, "-") || strings.ContainsAny(image, " \t\r\n") {
		return errors.New("invalid FORGEOS_GO_IMAGE")
	}
	snapshot, release, err := ciSnapshot(ctx, repo, candidate)
	if err != nil {
		return err
	}
	defer release()
	options, name, err := sandboxContainerOptions(snapshot, "none")
	if err != nil {
		return err
	}
	args := append([]string{"run", "--rm"}, options...)
	cmd := docker.command(ctx, append(args, image, "go", "test", "-buildvcs=false", "-count=1", "./...")...)
	// Keep captured test output local and outside the candidate checkout.
	logFile, err := os.OpenFile(repo+".go-test.log", os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
	if err != nil {
		return errors.New("cannot create local CI log")
	}
	defer logFile.Close()
	log := &cappedLog{dest: logFile, remaining: validation.LogLimit}
	cmd.Stdout, cmd.Stderr = log, log
	err = cmd.Run()
	if err != nil {
		cleanup, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		_ = docker.command(cleanup, "rm", "-f", name).Run()
		if ctx.Err() != nil {
			return errors.New("go-test interrupted; inspect the adjacent .go-test.log")
		}
		return fmt.Errorf("containerized go test did not pass (%w); inspect the adjacent .go-test.log", err)
	}
	return nil
}

type cappedLog struct {
	dest      io.Writer
	remaining int
	truncated bool
}

func (l *cappedLog) Write(p []byte) (int, error) {
	take := min(len(p), l.remaining)
	if take > 0 {
		n, err := l.dest.Write(p[:take])
		l.remaining -= n
		if err != nil {
			return n, err
		}
	}
	if take < len(p) && !l.truncated {
		l.truncated = true
		if _, err := io.WriteString(l.dest, validation.LogTruncation); err != nil {
			return take, err
		}
	}
	return len(p), nil
}
