package bao

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"

	"forgeos/internal/chatkernel"
	"github.com/google/uuid"
)

// Project model settings are a single versioned KV record. Git credential reads accept
// UUIDs only and cannot address this separate subtree of the control-plane KV policy.
func (c *Client) chatProvider(ctx context.Context, path string, update *chatkernel.Provider) (chatkernel.Provider, error) {
	var p chatkernel.Provider
	if c == nil {
		return p, ErrUnavailable
	}
	method := http.MethodGet
	var body io.Reader
	if update != nil {
		method = http.MethodPost
		raw, err := json.Marshal(map[string]any{"data": update})
		if err != nil {
			return p, ErrUnavailable
		}
		body = bytes.NewReader(raw)
	}
	path = "secret/data/forgeos/repository-credentials/project-chat/" + path
	resp, err := c.request(ctx, method, path, body, 0)
	if err != nil {
		return p, ErrUnavailable
	}
	defer resp.Body.Close()
	if update == nil && resp.StatusCode == 404 {
		return p, nil
	}
	if resp.StatusCode != 200 {
		return p, ErrUnavailable
	}
	if update != nil {
		return *update, nil
	}
	var result struct {
		Data struct{ Data chatkernel.Provider }
	}
	if json.NewDecoder(io.LimitReader(resp.Body, 32<<10)).Decode(&result) != nil {
		return p, ErrUnavailable
	}
	p = result.Data.Data
	if p != (chatkernel.Provider{}) && !p.Valid() {
		return chatkernel.Provider{}, ErrUnavailable
	}
	return p, nil
}
func (c *Client) ChatProvider(ctx context.Context, project int64) (chatkernel.Provider, error) {
	return c.chatProvider(ctx, fmt.Sprint(project), nil)
}

func (c *Client) ModelConnection(ctx context.Context, revision uuid.UUID) (chatkernel.Provider, error) {
	return c.chatProvider(ctx, "models/"+revision.String(), nil)
}

func (c *Client) SetModelConnection(ctx context.Context, revision uuid.UUID, p chatkernel.Provider) error {
	_, err := c.chatProvider(ctx, "models/"+revision.String(), &p)
	return err
}
