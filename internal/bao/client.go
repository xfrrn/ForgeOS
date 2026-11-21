// Package bao accesses OpenBao with separate identities for project metadata,
// wrapped task secrets, and control-plane repository credentials.
package bao

import (
	"bytes"
	"context"
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"forgeos/internal/store"

	"github.com/google/uuid"
)

type Client struct {
	address, tokenFile string
	http               *http.Client
}

var ErrUnavailable = errors.New("OpenBao metadata is unavailable")

func New(address, tokenFile, caFile string) (*Client, error) {
	if address == "" && tokenFile == "" && caFile == "" {
		return nil, nil
	}
	u, err := url.Parse(address)
	if err != nil || u.Scheme != "https" || u.Hostname() == "" || u.User != nil || u.RawQuery != "" || u.ForceQuery || u.Fragment != "" || (u.Path != "" && u.Path != "/") || !filepath.IsAbs(tokenFile) {
		return nil, errors.New("set an HTTPS OpenBao origin and an absolute token file path")
	}
	cfg := &tls.Config{MinVersion: tls.VersionTLS12}
	if caFile != "" {
		pem, err := os.ReadFile(caFile)
		if err != nil {
			return nil, errors.New("cannot read OpenBao CA file")
		}
		cfg.RootCAs = x509.NewCertPool()
		if !cfg.RootCAs.AppendCertsFromPEM(pem) {
			return nil, errors.New("invalid OpenBao CA certificate")
		}
	}
	client := &http.Client{Timeout: 5 * time.Second, Transport: &http.Transport{TLSClientConfig: cfg, IdleConnTimeout: 30 * time.Second},
		CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	return &Client{u.Scheme + "://" + u.Host, tokenFile, client}, nil
}

type Status struct {
	State   string `json:"state"`
	Version int    `json:"version"`
}

func (c *Client) Status(ctx context.Context, m store.SecretMeta) (Status, error) {
	if c == nil || m.ProjectID < 1 || !store.ValidEnvironment(m.Environment) || !store.ValidSecretName(m.Name) {
		return Status{}, ErrUnavailable
	}
	resp, err := c.get(ctx, fmt.Sprintf("secret/metadata/projects/%d/%s/%s", m.ProjectID, m.Environment, m.Name), 0)
	if err != nil {
		return Status{}, ErrUnavailable
	}
	defer resp.Body.Close()
	if resp.StatusCode == 404 {
		return Status{State: "MISSING"}, nil
	}
	if resp.StatusCode != 200 {
		return Status{}, ErrUnavailable
	}
	var doc struct {
		Data *struct {
			Version  *int `json:"current_version"`
			Versions map[string]struct {
				Deleted   string `json:"deletion_time"`
				Destroyed bool   `json:"destroyed"`
			} `json:"versions"`
		} `json:"data"`
	}
	raw, err := io.ReadAll(io.LimitReader(resp.Body, (2<<20)+1))
	if err != nil || len(raw) > 2<<20 || json.Unmarshal(raw, &doc) != nil || doc.Data == nil || doc.Data.Version == nil || *doc.Data.Version < 0 {
		return Status{}, ErrUnavailable
	}
	version := *doc.Data.Version
	if version == 0 {
		return Status{State: "MISSING"}, nil
	}
	entry, ok := doc.Data.Versions[strconv.Itoa(version)]
	if !ok {
		return Status{}, ErrUnavailable
	}
	status := Status{State: "AVAILABLE", Version: version}
	if entry.Destroyed {
		status.State = "DESTROYED"
	} else if entry.Deleted != "" {
		at, err := time.Parse(time.RFC3339Nano, entry.Deleted)
		if err != nil {
			return Status{}, ErrUnavailable
		}
		if !at.After(time.Now()) {
			status.State = "DELETED"
		}
	}
	return status, nil
}

func (c *Client) get(ctx context.Context, path string, wrapSeconds int) (*http.Response, error) {
	return c.request(ctx, "GET", path, nil, wrapSeconds)
}

func (c *Client) request(ctx context.Context, method, path string, body io.Reader, wrapSeconds int) (*http.Response, error) {
	req, err := http.NewRequestWithContext(ctx, method, c.address+"/v1/"+path, body)
	if err != nil {
		return nil, ErrUnavailable
	}
	raw, err := os.ReadFile(c.tokenFile)
	if err != nil || len(raw) > 4096 {
		return nil, ErrUnavailable
	}
	token := strings.TrimSpace(string(raw))
	if token == "" || strings.ContainsAny(token, " \t\r\n") {
		return nil, ErrUnavailable
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	req.Header.Set("X-Vault-Token", token)
	if wrapSeconds > 0 {
		req.Header.Set("X-Vault-Wrap-TTL", strconv.Itoa(wrapSeconds)+"s")
	}
	return c.http.Do(req)
}

// Repository tokens have a separate control-plane identity and KV namespace.
func (c *Client) repositoryRequest(ctx context.Context, key, method string, version int, body io.Reader, dest any) error {
	id, err := uuid.Parse(key)
	if c == nil || err != nil {
		return ErrUnavailable
	}
	path := "secret/data/forgeos/repository-credentials/" + id.String()
	if version > 0 {
		path += "?version=" + strconv.Itoa(version)
	}
	resp, err := c.request(ctx, method, path, body, 0)
	if err != nil {
		return ErrUnavailable
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return ErrUnavailable
	}
	raw, err := io.ReadAll(io.LimitReader(resp.Body, 16385))
	if err != nil || len(raw) > 16384 || json.Unmarshal(raw, dest) != nil {
		return ErrUnavailable
	}
	return nil
}

func (c *Client) SetRepositoryToken(ctx context.Context, key, token string) (int, error) {
	body, err := json.Marshal(map[string]any{"data": map[string]string{"token": token}})
	if err != nil {
		return 0, ErrUnavailable
	}
	var out struct {
		Data struct {
			Version int `json:"version"`
		} `json:"data"`
	}
	if err = c.repositoryRequest(ctx, key, "POST", 0, bytes.NewReader(body), &out); err != nil || out.Data.Version < 1 {
		return 0, ErrUnavailable
	}
	return out.Data.Version, nil
}

func (c *Client) RepositoryToken(ctx context.Context, key string, version int) (string, error) {
	var out struct {
		Data struct {
			Data struct {
				Token string `json:"token"`
			} `json:"data"`
			Metadata struct {
				Version int `json:"version"`
			} `json:"metadata"`
		} `json:"data"`
	}
	if version < 1 || c.repositoryRequest(ctx, key, "GET", version, nil, &out) != nil || out.Data.Metadata.Version != version || out.Data.Data.Token == "" {
		return "", ErrUnavailable
	}
	return out.Data.Data.Token, nil
}

type Wrapped struct {
	Token     string    `json:"wrapping_token"`
	ExpiresAt time.Time `json:"expires_at"`
}

func (c *Client) Wrap(ctx context.Context, m store.SecretMeta, version int, until time.Time) (Wrapped, error) {
	if c == nil || m.ProjectID <= 0 || !store.ValidEnvironment(m.Environment) || m.Environment == "production" || !store.ValidSecretName(m.Name) ||
		!store.ValidRisk(m.Risk) || m.Risk == "S3" || version <= 0 {
		return Wrapped{}, ErrUnavailable
	}
	seconds := min(60, int(time.Until(until)/time.Second)-1)
	if seconds < 1 {
		return Wrapped{}, store.ErrConflict
	}
	path := fmt.Sprintf("secret/data/projects/%d/%s/%s", m.ProjectID, m.Environment, m.Name)
	resp, err := c.get(ctx, path+"?version="+strconv.Itoa(version), seconds)
	if err != nil {
		return Wrapped{}, ErrUnavailable
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		return Wrapped{}, ErrUnavailable
	}
	var doc struct {
		Data json.RawMessage `json:"data"`
		Wrap struct {
			Token string    `json:"token"`
			TTL   int       `json:"ttl"`
			Path  string    `json:"creation_path"`
			At    time.Time `json:"creation_time"`
		} `json:"wrap_info"`
	}
	raw, err := io.ReadAll(io.LimitReader(resp.Body, 16385))
	if err != nil || len(raw) > 16384 || json.Unmarshal(raw, &doc) != nil || (doc.Data != nil && string(doc.Data) != "null") ||
		doc.Wrap.Token == "" || doc.Wrap.TTL < 1 || doc.Wrap.TTL > seconds || doc.Wrap.Path != path || doc.Wrap.At.IsZero() {
		return Wrapped{}, ErrUnavailable
	}
	expires := doc.Wrap.At.Add(time.Duration(doc.Wrap.TTL) * time.Second)
	if !expires.After(time.Now()) || expires.After(until) {
		return Wrapped{}, store.ErrConflict
	}
	return Wrapped{doc.Wrap.Token, expires}, nil
}
