package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"forgeos/internal/localrepo"
	"forgeos/internal/store"
	"forgeos/internal/validation"
	"forgeos/schemas"
)

type environmentRun store.EnvironmentRun

func verifyEnvironment(ctx context.Context, in io.Reader, out io.Writer) error {
	raw, err := io.ReadAll(io.LimitReader(in, (16<<10)+1))
	if err != nil || len(raw) > 16<<10 {
		return errors.New("invalid environment request size")
	}
	var req store.EnvironmentRequest
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.DisallowUnknownFields()
	if dec.Decode(&req) != nil || dec.Decode(new(any)) != io.EOF || !validation.FullCommit(req.Commit) || (req.Environment != "local-mock" && req.Environment != "local-integrated") {
		return errors.New("provide a repository, a full commit and a local environment")
	}
	root, work := os.Getenv("FORGEOS_REPO_ROOT"), os.Getenv("FORGEOS_WORKSPACE_ROOT")
	run := environmentRun{EnvironmentRequest: req, Worker: os.Getenv("FORGEOS_WORKER_ID"), Started: time.Now().UTC()}
	if os.Geteuid() == 0 || run.Worker == "" || !filepath.IsAbs(work) {
		return errors.New("configure a non-root worker, identity and absolute workspace root")
	}
	repo, err := localrepo.ResolveRepo(ctx, root, req.Repo)
	if err != nil {
		return err
	}
	run.Repo = localrepo.RelativePath(root, repo)
	if err = os.MkdirAll(work, 0700); err != nil {
		return err
	}
	work, err = filepath.EvalSymlinks(work)
	if err != nil {
		return err
	}
	if rel, err := filepath.Rel(repo, work); err == nil && (rel == "." || filepath.IsLocal(rel)) {
		return errors.New("startup evidence must be outside the repository")
	}
	dir, err := os.MkdirTemp(work, "environment-run-")
	if err != nil {
		return err
	}
	probeErr := run.record(ctx, repo, dir)
	if err = json.NewEncoder(out).Encode(run); err != nil {
		return err
	}
	return probeErr
}

func (run *environmentRun) record(ctx context.Context, repo, dir string) error {
	run.ID = filepath.Base(dir)
	check, cancel := context.WithTimeout(ctx, 7*time.Minute)
	defer cancel()
	probeErr := run.verify(check, repo)
	if probeErr != nil {
		run.Failure = probeErr.Error()
	}
	run.Finished = time.Now().UTC()
	doc, err := json.MarshalIndent(run, "", "  ")
	if err != nil {
		return err
	}
	if err = os.WriteFile(filepath.Join(dir, "run.json"), doc, 0600); err != nil {
		return err
	}
	return probeErr
}

func (r *environmentRun) verify(ctx context.Context, repo string) error {
	blob, raw, err := validation.ReadConfigFile(ctx, repo, r.Commit, "project.yaml")
	if err != nil || blob == "" {
		return errors.New("project.yaml unavailable at requested commit")
	}
	r.ConfigBlob = blob
	project, err := schemas.ParseProject(raw)
	if err != nil {
		return errors.New("invalid project.yaml")
	}
	config, ok := project.LocalEnvironments[r.Environment]
	if !ok {
		return errors.New("local environment not declared")
	}
	r.Config = &config
	entry, err := localrepo.Git(ctx, repo, "ls-tree", "-z", r.Commit, "--", ":(literal)"+config.StartScript)
	header, file, ok := strings.Cut(strings.TrimSuffix(string(entry), "\x00"), "\t")
	fields := strings.Fields(header)
	if err != nil || !ok || file != config.StartScript || len(fields) != 3 || (fields[0] != "100644" && fields[0] != "100755") || fields[1] != "blob" || !validation.FullCommit(fields[2]) {
		return errors.New("start script must be a committed regular file")
	}
	r.ScriptBlob = fields[2]
	d, err := openDocker(ctx)
	if err != nil {
		return err
	}
	image := os.Getenv("FORGEOS_STARTUP_IMAGE")
	if image == "" || strings.HasPrefix(image, "-") || strings.ContainsAny(image, " \r\n\t") {
		return errors.New("configure a local FORGEOS_STARTUP_IMAGE")
	}
	pinned, err := d.command(ctx, "image", "inspect", "--format", "{{.Id}}", image).Output()
	if err != nil || !imageID.MatchString(strings.TrimSpace(string(pinned))) {
		return errors.New("startup image unavailable")
	}
	r.Image = strings.TrimSpace(string(pinned))
	r.Network = "none"
	if r.Environment == "local-integrated" {
		r.Network, err = integrationNetwork(ctx, d, r.Repo)
		if err != nil {
			return err
		}
	}
	snapshot, release, err := ciSnapshot(ctx, repo, r.Commit)
	if err != nil {
		return err
	}
	defer release()
	return r.probe(ctx, d, snapshot)
}

func integrationNetwork(ctx context.Context, d localDocker, project string) (string, error) {
	name := os.Getenv("FORGEOS_STARTUP_NETWORK")
	if name == "" || strings.HasPrefix(name, "-") {
		return "", errors.New("configure a project-specific internal startup network")
	}
	raw, err := d.command(ctx, "network", "inspect", name).Output()
	var info []struct {
		ID, Driver string
		Internal   bool
		Labels     map[string]string
	}
	if err != nil || json.Unmarshal(raw, &info) != nil || len(info) != 1 {
		return "", errors.New("startup network unavailable")
	}
	n := info[0]
	if len(n.ID) != 64 || n.Driver != "bridge" || !n.Internal || project == "" || n.Labels["forgeos.project"] != project || n.Labels["forgeos.environment"] != "local-integrated" {
		return "", errors.New("startup network does not match the project's isolated local-integrated environment")
	}
	return n.ID, nil
}

func (r *environmentRun) probe(ctx context.Context, d localDocker, snapshot string) (err error) {
	options, name, err := sandboxContainerOptions(snapshot, r.Network)
	if err != nil {
		return err
	}
	// The engine also bounds the container lifetime when the worker disconnects.
	args := append([]string{"run", "-d", "--rm"}, options...)
	args = append(args, "--env=FORGEOS_LOCAL_ENVIRONMENT="+r.Environment, "--entrypoint", "/usr/bin/timeout", r.Image, "-s", "KILL", strconv.Itoa(r.Config.TimeoutSeconds+30),
		"/bin/sh", "-c", `mkdir /tmp/project && cp -R /src/. /tmp/project && rm -f /tmp/project/.git && cd /tmp/project && exec /bin/sh "./$1"`, "forgeos-start", r.Config.StartScript)
	defer func() {
		cleanup, cancel := context.WithTimeout(context.Background(), 15*time.Second)
		defer cancel()
		_ = d.command(cleanup, "rm", "-fv", name).Run()
		remaining, e := d.command(cleanup, "ps", "-aq", "--filter", "name=^/"+name+"$").Output()
		r.Removed = e == nil && len(bytes.TrimSpace(remaining)) == 0
		if !r.Removed {
			err = errors.Join(err, errors.New("cannot confirm startup container removal"))
		}
	}()
	if err = d.command(ctx, args...).Run(); err != nil {
		return errors.New("could not start the environment container")
	}
	probe, cancel := context.WithTimeout(ctx, time.Duration(r.Config.TimeoutSeconds)*time.Second)
	defer cancel()
	url := fmt.Sprintf("http://127.0.0.1:%d%s", r.Config.Port, r.Config.HealthPath)
	for probe.Err() == nil {
		r.Probes++
		code, e := d.command(probe, "exec", name, "/usr/bin/curl", "--disable", "--noproxy", "*", "--silent", "--output", "/dev/null", "--write-out", "%{http_code}", "--max-time", "2", url).Output()
		var exit *exec.ExitError
		if errors.As(e, &exit) && (exit.ExitCode() == 126 || exit.ExitCode() == 127) {
			return errors.New("startup image requires an executable /usr/bin/curl")
		}
		if n, e2 := strconv.Atoi(string(code)); e == nil && e2 == nil {
			r.StatusCode = n
			if n >= 200 && n < 300 {
				return nil
			}
		}
		if probe.Err() != nil {
			break
		}
		alive, e := d.command(probe, "ps", "-q", "--filter", "name=^/"+name+"$").Output()
		if probe.Err() != nil {
			break
		}
		if e != nil {
			return errors.New("startup container status unavailable")
		}
		if len(bytes.TrimSpace(alive)) == 0 {
			return errors.New("service exited before readiness")
		}
		select {
		case <-probe.Done():
		case <-time.After(500 * time.Millisecond):
		}
	}
	if ctx.Err() != nil {
		return ctx.Err()
	}
	return errors.New("readiness deadline exceeded")
}
