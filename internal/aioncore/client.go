// Package aioncore connects a ForgeOS node to its local AionCore instance.
package aioncore

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime/multipart"
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
		return HTTPError(res.StatusCode)
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

type HTTPError int

func (e HTTPError) Error() string { return fmt.Sprintf("AionCore HTTP %d", int(e)) }
func ProbeFailure(err error) string {
	var status HTTPError
	if errors.As(err, &status) && (status == 401 || status == 403) {
		return "auth"
	}
	if errors.Is(err, context.DeadlineExceeded) {
		return "timeout"
	}
	return "unreachable"
}
func (c *Client) Probe(ctx context.Context, backend string) string {
	var agents []struct {
		Backend            string
		Installed, Enabled bool
	}
	if err := c.request(ctx, "GET", "/api/agents/management", nil, &agents); err != nil {
		return ProbeFailure(err)
	}
	for _, a := range agents {
		if a.Backend == backend && a.Installed {
			if a.Enabled {
				return "ready"
			}
			return "disabled"
		}
	}
	return "not_installed"
}
func (c *Client) Available(ctx context.Context, backend string) bool {
	return c.Probe(ctx, backend) == "ready"
}

var ErrSessionMissing = errors.New("saved discussion session is missing")

func (c *Client) CheckSession(ctx context.Context, id string) error {
	err := c.request(ctx, "GET", "/api/conversations/"+url.PathEscape(id), nil, nil)
	if errors.Is(err, HTTPError(404)) {
		return ErrSessionMissing
	}
	return err
}
func (c *Client) Cancel(ctx context.Context, id, turn string) error {
	if turn == "" {
		return errors.New("missing native turn receipt")
	}
	base := "/api/conversations/" + url.PathEscape(id)
	var result struct {
		Runtime *struct {
			State      string
			Processing bool   `json:"is_processing"`
			Turn       string `json:"turn_id"`
		} `json:"runtime"`
	}
	if err := c.request(ctx, "POST", base+"/cancel", map[string]string{"turn_id": turn}, &result); err != nil {
		return err
	}
	for {
		if result.Runtime != nil && !result.Runtime.Processing && result.Runtime.State == "idle" {
			return nil
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(250 * time.Millisecond):
		}
		result.Runtime = nil
		if err := c.request(ctx, "GET", base, nil, &result); err != nil {
			return err
		}
	}
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
	Current string                         `json:"current_value"`
	Choices []struct{ Value, Name string } `json:"options"`
}
type runtime struct {
	Options []option `json:"config_options"`
}

type ModelChoice struct {
	ID    string `json:"id"`
	Label string `json:"label,omitempty"`
}

func (c *Client) ProbeModels(ctx context.Context, id string) ([]ModelChoice, error) {
	base := "/api/conversations/" + url.PathEscape(id)
	var state runtime
	if err := c.request(ctx, "POST", base+"/runtime/ensure", nil, &state); err != nil {
		return nil, err
	}
	opt := state.get("model")
	seen := make(map[string]bool, len(opt.Choices)+1)
	out := make([]ModelChoice, 0, len(opt.Choices)+1)
	for _, ch := range opt.Choices {
		if ch.Value == "" || seen[ch.Value] {
			continue
		}
		seen[ch.Value] = true
		label := strings.TrimSpace(ch.Name)
		if label == "" {
			label = ch.Value
		}
		out = append(out, ModelChoice{ID: ch.Value, Label: label})
	}
	if opt.Current != "" && !seen[opt.Current] {
		out = append([]ModelChoice{{ID: opt.Current, Label: opt.Current}}, out...)
	}
	return out, nil
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

type ChatFileRef struct {
	Kind string `json:"kind"`
	Path string `json:"path"`
}

func (c *Client) UploadFile(ctx context.Context, conversationID, filename string, data []byte) (string, error) {
	if filename == "" {
		filename = "image.png"
	}
	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	if err := writer.WriteField("file_name", filename); err != nil {
		return "", err
	}
	if conversationID != "" {
		if err := writer.WriteField("conversation_id", conversationID); err != nil {
			return "", err
		}
	}
	part, err := writer.CreateFormFile("file", filename)
	if err != nil {
		return "", err
	}
	if _, err := part.Write(data); err != nil {
		return "", err
	}
	if err := writer.Close(); err != nil {
		return "", err
	}
	req, err := http.NewRequestWithContext(ctx, "POST", c.base.String()+"/api/fs/upload", &body)
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", writer.FormDataContentType())
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
		return "", err
	}
	defer res.Body.Close()
	if res.StatusCode < 200 || res.StatusCode >= 300 {
		return "", HTTPError(res.StatusCode)
	}
	var envelope struct {
		Success bool
		Data    string
	}
	if err := json.NewDecoder(io.LimitReader(res.Body, 1<<20)).Decode(&envelope); err != nil {
		return "", err
	}
	if !envelope.Success || envelope.Data == "" {
		return "", errors.New("AionCore rejected file upload")
	}
	return envelope.Data, nil
}

func (c *Client) Send(ctx context.Context, id, prompt string) (Receipt, error) {
	return c.SendWithFiles(ctx, id, prompt, nil)
}

func (c *Client) SendWithFiles(ctx context.Context, id, prompt string, files []ChatFileRef) (Receipt, error) {
	var r Receipt
	payload := struct {
		Content string        `json:"content"`
		Files   []ChatFileRef `json:"files,omitempty"`
	}{Content: prompt, Files: files}
	err := c.request(ctx, "POST", "/api/conversations/"+url.PathEscape(id)+"/messages", payload, &r)
	if err == nil && (r.Message == "" || r.Turn == "") {
		err = errors.New("missing AionCore send receipt")
	}
	return r, err
}
func (c *Client) Reply(ctx context.Context, id, message, marker string) (string, error) {
	return c.DocumentReply(ctx, id, message, marker, "", nil)
}

// Claude may deliver its document as an ExitPlanMode submission. Persist only the
// plan for this exact message and turn, then stop the isolated conversation.
func (c *Client) DocumentReply(ctx context.Context, id, message, marker, turn string, save func(string) error) (string, error) {
	base := "/api/conversations/" + url.PathEscape(id)
	for {
		var state struct {
			Status  string
			Runtime struct {
				State string
				Turn  string `json:"turn_id"`
			}
		}
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
			if save != nil && turn != "" && state.Runtime.Turn == turn && state.Runtime.State == "waiting_confirmation" && item.Type == "tool_call" && item.Status == "work" {
				var submission struct {
					Name  string
					Input struct{ Plan string }
				}
				if json.Unmarshal(item.Content, &submission) == nil && submission.Name == "ExitPlanMode" && strings.TrimSpace(submission.Input.Plan) != "" {
					plan := strings.TrimSpace(submission.Input.Plan)
					if len(plan) > 16<<10 {
						return "", errors.New("submitted design exceeds reply limit")
					}
					if err := save(plan); err != nil {
						return "", err
					}
					return plan, c.StopDocumentTurn(ctx, id, turn)
				}
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

// On receipt replay, the prior process may already have stopped the native turn.
func (c *Client) StopDocumentTurn(ctx context.Context, id, turn string) error {
	var state struct {
		Runtime struct {
			State      string
			Processing bool   `json:"is_processing"`
			Turn       string `json:"turn_id"`
		}
	}
	if err := c.request(ctx, "GET", "/api/conversations/"+url.PathEscape(id), nil, &state); err != nil {
		return err
	}
	if !state.Runtime.Processing && state.Runtime.State == "idle" {
		return nil
	}
	if turn == "" || state.Runtime.Turn != turn {
		return errors.New("document turn identity changed")
	}
	stop, done := context.WithTimeout(ctx, 15*time.Second)
	defer done()
	return c.Cancel(stop, id, turn)
}
