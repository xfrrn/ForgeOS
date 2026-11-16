// Package bao reads KV v2 metadata and requests single-use, response-wrapped values.
package bao

import (
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
	req, err := http.NewRequestWithContext(ctx, "GET", c.address+"/v1/"+path, nil)
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
	req.Header.Set("X-Vault-Token", token)
	if wrapSeconds > 0 {
		req.Header.Set("X-Vault-Wrap-TTL", strconv.Itoa(wrapSeconds)+"s")
	}
	return c.http.Do(req)
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
