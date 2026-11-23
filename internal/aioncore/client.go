// Package aioncore connects a ForgeOS node to its local AionCore instance.
package aioncore

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/cookiejar"
	"net/url"
	"strconv"
	"strings"
	"time"
)

type Config struct {
	Instance, Backend, Model, Username, Password string
	Port                                         int
}
type Client struct {
	base  *url.URL
	http  *http.Client
	token string
}

func Login(ctx context.Context, c Config) (*Client, error) {
	if c.Port < 1 || c.Port > 65535 || c.Username == "" || c.Password == "" {
		return nil, errors.New("invalid local AionCore configuration")
	}
	base := &url.URL{Scheme: "http", Host: "127.0.0.1:" + strconv.Itoa(c.Port)}
	jar, _ := cookiejar.New(nil)
	client := &Client{base: base, http: &http.Client{Jar: jar, Timeout: 45 * time.Second, Transport: &http.Transport{DisableKeepAlives: true}, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}}
	var auth struct{ Token string }
	if err := client.request(ctx, "POST", "/login", map[string]string{"username": c.Username, "password": c.Password}, &auth); err != nil {
		return nil, err
	}
	if auth.Token == "" {
		return nil, errors.New("missing AionCore login receipt")
	}
	client.token = auth.Token
	return client, nil
}

func (c *Client) request(ctx context.Context, method, path string, body, out any) error {
	var reader io.Reader
	if method != "GET" {
		b, err := json.Marshal(body)
		if err != nil {
			return err
		}
		reader = bytes.NewReader(b)
	}
	req, err := http.NewRequestWithContext(ctx, method, c.base.String()+path, reader)
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	if c.token != "" {
		req.Header.Set("Authorization", "Bearer "+c.token)
	}
	for _, cookie := range c.http.Jar.Cookies(c.base) {
		if cookie.Name == "aionui-csrf-token" {
			req.Header.Set("x-csrf-token", cookie.Value)
		}
	}
	res, err := c.http.Do(req)
	if err != nil {
		return err
	}
	defer res.Body.Close()
	if res.StatusCode < 200 || res.StatusCode >= 300 {
		return fmt.Errorf("AionCore HTTP %d", res.StatusCode)
	}
	decoder := json.NewDecoder(io.LimitReader(res.Body, 8<<20))
	if path == "/login" {
		return decoder.Decode(out)
	}
	var envelope struct {
		Success bool
		Data    json.RawMessage
	}
	if err := decoder.Decode(&envelope); err != nil {
		return err
	}
	if !envelope.Success {
		return errors.New("AionCore rejected request")
	}
	if out != nil {
		return json.Unmarshal(envelope.Data, out)
	}
	return nil
}

func (c *Client) Available(ctx context.Context, backend string) bool {
	var agents []struct {
		Backend            string
		Installed, Enabled bool
	}
	if c.request(ctx, "GET", "/api/agents/management", nil, &agents) != nil {
		return false
	}
	for _, a := range agents {
		if a.Backend == backend && a.Installed && a.Enabled {
			return true
		}
	}
	return false
}
func (c *Client) Create(ctx context.Context, backend, workspace, name string) (string, error) {
	var result struct{ ID string }
	err := c.request(ctx, "POST", "/api/conversations", map[string]any{"type": "acp", "name": name, "extra": map[string]string{"backend": backend, "workspace": workspace}}, &result)
	if err == nil && result.ID == "" {
		err = errors.New("missing conversation id")
	}
	return result.ID, err
}

type option struct {
	ID      string
	Current string                   `json:"current_value"`
	Choices []struct{ Value string } `json:"options"`
}
type runtime struct {
	Options []option `json:"config_options"`
}

func (r runtime) get(key string) option {
	for _, opt := range r.Options {
		if opt.ID == key {
			return opt
		}
	}
	return option{}
}

func (c *Client) Configure(ctx context.Context, id, backend, model string) error {
	base := "/api/conversations/" + url.PathEscape(id)
	mode := "read-only"
	if backend == "claude" {
		mode = "plan"
	}
	var state runtime
	if err := c.request(ctx, "POST", base+"/runtime/ensure", nil, &state); err != nil {
		return err
	}
	for _, setting := range []struct{ key, value string }{{"model", model}, {"mode", mode}} {
		opt := state.get(setting.key)
		if opt.Current == setting.value {
			continue
		}
		supported := false
		for _, o := range opt.Choices {
			if o.Value == setting.value {
				supported = true
				break
			}
		}
		if !supported {
			return fmt.Errorf("requested AionCore %s unavailable", setting.key)
		}
		// Model changes may take effect on the next turn. Permissions have to
		// be observed before a turn can be sent.
		if err := c.request(ctx, "PUT", base+"/config-options/"+setting.key, map[string]string{"value": setting.value}, nil); err != nil {
			return err
		}
	}
	for attempt := 0; attempt < 30; attempt++ {
		if err := c.request(ctx, "POST", base+"/runtime/ensure", nil, &state); err != nil {
			return err
		}
		if state.get("mode").Current == mode {
			return nil
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(400 * time.Millisecond):
		}
	}
	return errors.New("AionCore did not confirm discussion permissions")
}

type Receipt struct {
	Message string `json:"msg_id"`
	Turn    string `json:"turn_id"`
}

func (c *Client) Send(ctx context.Context, id, prompt string) (Receipt, error) {
	var r Receipt
	err := c.request(ctx, "POST", "/api/conversations/"+url.PathEscape(id)+"/messages", map[string]string{"content": prompt}, &r)
	if err == nil && (r.Message == "" || r.Turn == "") {
		err = errors.New("missing AionCore send receipt")
	}
	return r, err
}
func (c *Client) Reply(ctx context.Context, id, message, marker string) (string, error) {
	base := "/api/conversations/" + url.PathEscape(id)
	for {
		var state struct{ Status string }
		if err := c.request(ctx, "GET", base, nil, &state); err != nil {
			return "", err
		}
		var page struct {
			Items []struct {
				ID, Position, Status, Type string
				MessageID                  string `json:"msg_id"`
				Content                    json.RawMessage
			}
		}
		if err := c.request(ctx, "GET", base+"/messages", nil, &page); err != nil {
			return "", err
		}
		started := false
		var replies []string
		for _, item := range page.Items {
			var value struct{ Content, Type string }
			if json.Unmarshal(item.Content, &value) != nil {
				continue
			}
			if item.Position == "right" {
				if started {
					break
				}
				started = (message != "" && (item.ID == message || item.MessageID == message)) || (message == "" && strings.HasPrefix(value.Content, marker))
				continue
			}
			if !started || item.Position != "left" {
				continue
			}
			if item.Type == "tips" && value.Type == "error" {
				return "", errors.New("AionCore upstream turn failed")
			}
			if item.Type == "text" && item.Status == "finish" {
				replies = append(replies, value.Content)
			}
		}
		if started && state.Status == "finished" && len(replies) > 0 {
			return strings.Join(replies, "\n"), nil
		}
		select {
		case <-ctx.Done():
			return "", ctx.Err()
		case <-time.After(time.Second):
		}
	}
}
