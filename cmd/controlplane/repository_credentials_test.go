package main

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"encoding/pem"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"forgeos/internal/bao"
	"forgeos/internal/localrepo"
	"forgeos/internal/store"
	"forgeos/internal/testdb"
)

func testServerCA(t *testing.T, server *httptest.Server) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "ca.pem")
	if err := os.WriteFile(path, pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: server.Certificate().Raw}), 0600); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestRepositoryCredentialGitTransport(t *testing.T) {
	ctx := context.Background()
	_, repo, base := issueTestRepo(t)
	empty := t.TempDir()
	if _, err := localrepo.Git(ctx, repo, "-c", "protocol.file.allow=always", "clone", "--bare", repo, filepath.Join(empty, "private.git")); err != nil {
		t.Fatal(err)
	}
	if _, err := localrepo.Git(ctx, filepath.Join(empty, "private.git"), "update-server-info"); err != nil {
		t.Fatal(err)
	}
	header := "Authorization: Basic " + base64.StdEncoding.EncodeToString([]byte("reader:fixture-token"))
	files := http.FileServer(http.Dir(empty))
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != strings.TrimPrefix(header, "Authorization: ") {
			w.Header().Set("WWW-Authenticate", `Basic realm="repository"`)
			http.Error(w, "sign in", 401)
			return
		}
		files.ServeHTTP(w, r)
	}))
	defer server.Close()
	ca := testServerCA(t, server)
	remote := cloneRemote{Host: "127.0.0.1", URL: server.URL + "/private.git", credential: &gitCredential{username: "reader", token: "fixture-token"}}
	dir := t.TempDir()
	t.Setenv("NO_PROXY", "127.0.0.1")
	cmd, err := projectHTTPSGit(ctx, dir, remote, "clone", "--no-recurse-submodules", "--", remote.URL, ".")
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(strings.Join(cmd.Args, " "), "fixture-token") {
		t.Fatal("credential in process arguments")
	}
	cmd.Env = append(cmd.Env, "GIT_SSL_CAINFO="+ca)
	if err = cmd.Run(); err != nil {
		t.Fatal("private HTTPS clone failed", err)
	}
	config, err := os.ReadFile(filepath.Join(dir, ".git", "config"))
	if err != nil || strings.Contains(string(config), "fixture-token") || strings.Contains(string(config), "credential") || !strings.Contains(string(config), remote.URL) {
		t.Fatal("unexpected clone configuration")
	}
	head, err := localrepo.Git(ctx, dir, "rev-parse", "HEAD")
	if err != nil || strings.TrimSpace(string(head)) != base {
		t.Fatal("missing private checkout")
	}
	remote, _ = parseCloneRemote("https://github.com/org/repo", "")
	cmd, err = projectHTTPSGit(ctx, dir, remote, "ls-remote", "--", remote.URL)
	if err != nil || !strings.Contains(strings.Join(cmd.Args, " "), "!gh auth git-credential") || strings.Contains(strings.Join(cmd.Env, "\n"), "FORGEOS_REPO_TOKEN=") {
		t.Fatal("local GitHub identity changed")
	}
}

func TestRepositoryCredentialLifecycle(t *testing.T) {
	ctx := context.Background()
	data := store.New(testdb.Open(t))
	if err := data.Migrate(ctx); err != nil {
		t.Fatal(err)
	}
	tokens := []string{""}
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.HasPrefix(r.URL.Path, "/v1/secret/data/forgeos/repository-credentials/") || r.Header.Get("X-Vault-Token") != "vault-test" {
			http.Error(w, "denied", 403)
			return
		}
		if r.Method == "POST" {
			var doc struct{ Data struct{ Token string } }
			if json.NewDecoder(r.Body).Decode(&doc) != nil || doc.Data.Token == "" {
				w.WriteHeader(400)
				return
			}
			if doc.Data.Token == "simulate-failure" {
				w.WriteHeader(503)
				return
			}
			tokens = append(tokens, doc.Data.Token)
			fmt.Fprintf(w, `{"data":{"version":%d}}`, len(tokens)-1)
			return
		}
		v, _ := strconv.Atoi(r.URL.Query().Get("version"))
		if v < 1 || v >= len(tokens) {
			w.WriteHeader(404)
			return
		}
		fmt.Fprintf(w, `{"data":{"data":{"token":%q},"metadata":{"version":%d}}}`, tokens[v], v)
	}))
	defer srv.Close()
	file := filepath.Join(t.TempDir(), "vault-token")
	if err := os.WriteFile(file, []byte("vault-test"), 0600); err != nil {
		t.Fatal(err)
	}
	client, err := bao.New(srv.URL, file, testServerCA(t, srv))
	if err != nil {
		t.Fatal(err)
	}
	root, repo, _ := issueTestRepo(t)
	service := projectCloneService{data: data, credentials: client, localRoot: root}
	mux := http.NewServeMux()
	service.registerCredentials(mux, "owner")
	service.registerProjectGit(mux, "owner")
	request := func(method, path, body, identity string, status int) string {
		t.Helper()
		r := httptest.NewRequest(method, path, strings.NewReader(body))
		r.Header.Set("Authorization", "Bearer "+identity)
		w := httptest.NewRecorder()
		mux.ServeHTTP(w, r)
		if w.Code != status || strings.Contains(w.Body.String(), "fixture-token") {
			t.Fatalf("%s %s: status %d, wanted %d (or unexpected token disclosure)", method, path, w.Code, status)
		}
		return w.Body.String()
	}
	const path = "/projects/repository-credentials"
	const target = "https://github.com/org/private.git"
	body := `{"name":"reader","username":"test","url":"` + target + `","token":"fixture-token"}`
	request("POST", path, body, "worker", 401)
	request("POST", path, body, "owner", 201)
	request("GET", path, "", "worker", 401)
	if list := request("GET", path, "", "owner", 200); !strings.Contains(list, `"enabled":true`) {
		t.Fatal("credential missing")
	}
	remote, _ := parseCloneRemote(target, "")
	other, _ := parseCloneRemote("https://github.com/org/other", "")
	if service.authenticateRemote(ctx, 1, &other) != errCredentialScope {
		t.Fatal("credential crossed repository scope")
	}
	request("PUT", path, `{"id":1,"action":"replace","token":"simulate-failure"}`, "owner", 503)
	if err = service.authenticateRemote(ctx, 1, &remote); err != nil || remote.credential == nil || *remote.credential != (gitCredential{"test", "fixture-token"}) {
		t.Fatal("prior version lost")
	}
	job, err := data.QueueRepositoryClone(ctx, target, "org/private", t.TempDir(), 1)
	if err != nil {
		t.Fatal(err)
	}
	claimed, err := data.ClaimRepositoryClone(ctx)
	if err != nil || claimed == nil || claimed.CredentialID != 1 {
		t.Fatal("lost queued identity", err)
	}
	if err = data.CompleteRepositoryClone(ctx, job.ID, claimed.Lease, repo); err != nil {
		t.Fatal(err)
	}
	done, err := data.RepositoryClone(ctx, job.ID)
	if err != nil {
		t.Fatal(err)
	}
	identityPath := fmt.Sprintf("/projects/%d/git/identity", done.ProjectID)
	if !strings.Contains(request("GET", identityPath, "", "owner", 200), `"credential_id":1`) {
		t.Fatal("lost project binding")
	}
	// The checkout initially tracks a different repository. A bound token
	// must not follow a changed origin, even on the same host.
	remotePath := fmt.Sprintf("/projects/%d/git/remote", done.ProjectID)
	request("GET", remotePath, "", "owner", 409)
	if _, err = localrepo.Git(ctx, repo, "remote", "set-url", "origin", target); err != nil {
		t.Fatal(err)
	}
	request("PUT", identityPath, `{"credential_id":1}`, "owner", 204)
	request("PUT", path, `{"id":1,"action":"disable"}`, "owner", 204)
	if err = service.authenticateRemote(ctx, 1, &remote); err != errCredentialScope {
		t.Fatal("disabled identity allowed")
	}
	request("GET", remotePath, "", "owner", 409)
	request("GET", fmt.Sprintf("/projects/%d/git/local", done.ProjectID), "", "owner", 200)
	request("PUT", path, `{"id":1,"action":"replace","token":"fixture-token-2"}`, "owner", 204)
	if err = service.authenticateRemote(ctx, 1, &remote); err != nil || remote.credential == nil || *remote.credential != (gitCredential{"test", "fixture-token-2"}) {
		t.Fatal("rotation failed")
	}
	request("PUT", identityPath, `{"credential_id":0}`, "owner", 204)
	if !strings.Contains(request("GET", identityPath, "", "owner", 200), `"credential_id":0`) {
		t.Fatal("local identity not restored")
	}
	account := `{"scope":"account","url":"GitHub.COM","name":"personal","username":"test","token":"fixture-token-account"}`
	request("POST", path, account, "owner", 201)
	for _, location := range []string{target, "git@github.com:another-org/another-repo.git"} {
		r, err := parseCloneRemote(location, "")
		if err != nil || service.authenticateRemote(ctx, 2, &r) != nil || r.credential == nil || r.credential.token != "fixture-token-account" {
			t.Fatal("account credential unavailable for an authorized host")
		}
	}
	for _, host := range []string{"gitlab.com", "github.com.example.org", "git.github.com"} {
		r, err := parseCloneRemote("https://"+host+"/org/private", host)
		if err != nil || service.authenticateRemote(ctx, 2, &r) != errCredentialScope || r.credential != nil {
			t.Fatal("account credential was not restricted to the exact host")
		}
	}
	request("PUT", identityPath, `{"credential_id":2}`, "owner", 204)
	request("PUT", path, `{"id":2,"action":"disable"}`, "owner", 204)
	request("GET", remotePath, "", "owner", 409)
}

func TestCredentialTarget(t *testing.T) {
	for _, input := range []string{"git.example.net", "https://GIT.EXAMPLE.NET:443/"} {
		if got, err := credentialTarget("account", input, "git.example.net"); err != nil || got != "https://git.example.net" {
			t.Fatalf("unexpected account target for %q", input)
		}
	}
	for _, input := range []string{"*", "https://github.com/org/repo", "https://u@github.com", "https://github.com:8443", "http://github.com", "git.example.net"} {
		if _, err := credentialTarget("account", input, ""); err == nil {
			t.Errorf("accepted account target %q", input)
		}
	}
	if _, err := credentialTarget("", "https://github.com", ""); err == nil {
		t.Fatal("account scope must be explicit")
	}
}
