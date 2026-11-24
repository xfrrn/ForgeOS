package chatkernel

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"strings"
	"time"
)

// Each turn gets its own unguessable loopback endpoint. The upstream provider key
// stays in the worker and is never added to the Codex or tool process environment.
func (c *Config) Start(ctx context.Context) (string, func(), error) {
	var secret [32]byte
	if _, err := rand.Read(secret[:]); err != nil {
		return "", nil, err
	}
	listener, err := net.Listen("tcp4", "127.0.0.1:0")
	if err != nil {
		return "", nil, err
	}
	prefix := "/" + hex.EncodeToString(secret[:])
	client := completionClient(4 * time.Minute)
	mux := http.NewServeMux()
	mux.HandleFunc("POST "+prefix+"/responses", func(w http.ResponseWriter, r *http.Request) {
		var input responseRequest
		if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 4<<20)).Decode(&input); err != nil {
			http.Error(w, "invalid Responses request", 400)
			return
		}
		if err := c.exchange(ctx, r.Context(), client, w, input); err != nil {
			http.Error(w, err.Error(), 502)
		}
	})
	if c.TaskTool != nil {
		mux.HandleFunc("POST "+prefix+"/tasks", c.taskTools)
	}
	server := &http.Server{Handler: mux, ReadHeaderTimeout: 5 * time.Second, IdleTimeout: 10 * time.Second, MaxHeaderBytes: 16 << 10}
	go server.Serve(listener)
	return "http://" + listener.Addr().String() + prefix, func() { server.Close() }, nil
}

type contentPart struct {
	Type string
	Text string
}
type item struct {
	Type      string
	Role      string
	Content   json.RawMessage
	Name      string
	Namespace string
	CallID    string `json:"call_id"`
	Arguments string
	Output    json.RawMessage
}
type spec struct {
	Type        string
	Name        string
	Description string
	Parameters  json.RawMessage
	Tools       []spec
}
type responseRequest struct {
	Model        string
	Instructions string
	Input        []item
	Tools        []spec
	Text         struct{ Format map[string]any }
}
type obj = map[string]any

func plain(raw json.RawMessage) (string, error) {
	var s string
	if json.Unmarshal(raw, &s) == nil {
		return s, nil
	}
	var parts []contentPart
	if err := json.Unmarshal(raw, &parts); err != nil {
		return "", errors.New("unsupported chat content")
	}
	var text []string
	for _, p := range parts {
		switch p.Type {
		case "input_text", "output_text", "text":
			text = append(text, p.Text)
		default:
			return "", errors.New("project chat supports text content")
		}
	}
	return strings.Join(text, "\n"), nil
}

type mountedTool struct {
	name, namespace, alias string
	definition             obj
}

func (c *Config) tools(specs []spec) []mountedTool {
	var mounted []mountedTool
	var collect func([]spec, string)
	collect = func(list []spec, namespace string) {
		for _, s := range list {
			if s.Type == "namespace" {
				collect(s.Tools, s.Name)
				continue
			}
			if s.Type != "function" {
				continue
			}
			qualified := s.Name
			if namespace != "" {
				qualified = namespace + "__" + s.Name
			}
			allowed := qualified == "mcp__forgeos_skills__read" || (c.TaskTool != nil && (qualified == "mcp__forgeos_tasks__"+ResolveTask || qualified == "mcp__forgeos_tasks__"+TaskProgress))
			for server, conf := range c.Servers {
				for _, tool := range conf.Tools {
					if qualified == "mcp__"+server+"__"+tool {
						allowed = true
					}
				}
			}
			if !allowed {
				continue
			}
			alias := qualified
			if !wireToolName.MatchString(alias) {
				alias = fmt.Sprintf("forgeos_tool_%d", len(mounted))
			}
			mounted = append(mounted, mountedTool{s.Name, namespace, alias, obj{"type": "function", "function": obj{"name": alias, "description": s.Description, "parameters": s.Parameters}}})
		}
	}
	collect(specs, "")
	return mounted
}
func requestBody(in responseRequest, tools []mountedTool, format string) (obj, error) {
	messages := []obj{}
	if in.Instructions != "" {
		messages = append(messages, obj{"role": "system", "content": in.Instructions})
	}
	for _, i := range in.Input {
		switch i.Type {
		case "", "message":
			role := i.Role
			if role == "developer" {
				role = "system"
			}
			if role != "user" && role != "assistant" && role != "system" {
				return nil, errors.New("invalid message role")
			}
			text, err := plain(i.Content)
			if err != nil {
				return nil, err
			}
			messages = append(messages, obj{"role": role, "content": text})
		case "function_call":
			name := ""
			for _, t := range tools {
				if i.Name == t.name && i.Namespace == t.namespace {
					name = t.alias
					break
				}
			}
			if name == "" {
				return nil, errors.New("unknown tool in project chat history")
			}
			call := obj{"id": i.CallID, "type": "function", "function": obj{"name": name, "arguments": i.Arguments}}
			if n := len(messages); n > 0 && messages[n-1]["tool_calls"] != nil {
				prev := messages[n-1]
				prev["tool_calls"] = append(prev["tool_calls"].([]obj), call)
			} else {
				messages = append(messages, obj{"role": "assistant", "tool_calls": []obj{call}})
			}
		case "function_call_output":
			output, err := plain(i.Output)
			if err != nil {
				return nil, err
			}
			messages = append(messages, obj{"role": "tool", "tool_call_id": i.CallID, "content": output})
		case "reasoning": // Provider-specific encrypted reasoning cannot be replayed on this wire format.
		default:
			return nil, errors.New("unsupported Codex input item: " + i.Type)
		}
	}
	body := obj{"model": in.Model, "messages": messages, "stream": true}
	if len(tools) > 0 {
		defs := make([]obj, 0, len(tools))
		for _, t := range tools {
			defs = append(defs, t.definition)
		}
		body["tools"] = defs
	}
	if f := in.Text.Format; f["type"] == "json_schema" {
		if format == "json_object" {
			schema, err := json.Marshal(f["schema"])
			if err != nil {
				return nil, err
			}
			body["messages"] = append([]obj{{"role": "system", "content": "Return a JSON object following this schema: " + string(schema)}}, messages...)
			body["response_format"] = obj{"type": "json_object"}
		} else {
			body["response_format"] = obj{"type": "json_schema", "json_schema": obj{"name": f["name"], "strict": f["strict"], "schema": f["schema"]}}
		}
	}
	return body, nil
}

type functionCall struct {
	ID       string
	Function struct{ Name, Arguments string }
}
type completionChoice struct {
	Finish  string `json:"finish_reason"`
	Message struct {
		Content string
		Calls   []functionCall `json:"tool_calls"`
	}
}
type tokenUsage struct {
	Prompt    int `json:"prompt_tokens"`
	Generated int `json:"completion_tokens"`
}
type completion struct {
	Choices []completionChoice
	Usage   tokenUsage
}

func (c *Config) requestCompletion(ctx context.Context, client *http.Client, body obj) (*http.Response, error) {
	raw, err := json.Marshal(body)
	if err != nil {
		return nil, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, strings.TrimRight(c.provider.BaseURL, "/")+"/chat/completions", bytes.NewReader(raw))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "text/event-stream, application/json")
	req.Header.Set("Authorization", "Bearer "+c.provider.APIKey)
	res, err := client.Do(req)
	if err != nil {
		c.providerStatus.Store(-1)
		return nil, errors.New("project model endpoint unreachable")
	}
	c.providerStatus.Store(int32(res.StatusCode))
	return res, nil
}

// Retry once without streaming only when a provider explicitly rejects that option,
// before any generated tokens have been received.
func rejectsStreaming(res *http.Response) bool {
	if res.StatusCode != 400 && res.StatusCode != 422 {
		return false
	}
	var payload struct {
		Error struct{ Param, Message string }
	}
	if json.NewDecoder(io.LimitReader(res.Body, 8192)).Decode(&payload) != nil {
		return false
	}
	msg := strings.ToLower(payload.Error.Message)
	return payload.Error.Param == "stream" || (strings.Contains(msg, "stream") && (strings.Contains(msg, "not support") || strings.Contains(msg, "unsupported")))
}

func (c *Config) complete(ctx context.Context, client *http.Client, body obj) (completion, error) {
	// These official endpoints need non-thinking mode for JSON output or tool
	// replay without provider-private reasoning fields.
	switch strings.TrimRight(c.provider.BaseURL, "/") {
	case "https://api.deepseek.com":
		if c.provider.Model == "deepseek-flash" || c.provider.Model == "deepseek-v4-pro" {
			body["thinking"] = obj{"type": "disabled"}
		}
	case "https://dashscope.aliyuncs.com/compatible-mode/v1":
		if c.provider.Model == "qwen-plus" {
			body["enable_thinking"] = false
		}
	}
	res, err := c.requestCompletion(ctx, client, body)
	if err != nil {
		return completion{}, err
	}
	if rejectsStreaming(res) {
		res.Body.Close()
		body["stream"] = false
		res, err = c.requestCompletion(ctx, client, body)
		if err != nil {
			return completion{}, err
		}
	}
	defer res.Body.Close()
	if res.StatusCode != 200 {
		return completion{}, fmt.Errorf("project model returned HTTP %d", res.StatusCode)
	}
	var reply completion
	if strings.HasPrefix(strings.ToLower(res.Header.Get("Content-Type")), "text/event-stream") {
		if c.Preview != nil {
			c.Preview("")
		}
		reply, err = readCompletionStream(res.Body, c.Preview)
		if err != nil {
			return completion{}, err
		}
	} else if err := json.NewDecoder(io.LimitReader(res.Body, 2<<20)).Decode(&reply); err != nil || len(reply.Choices) != 1 {
		return completion{}, errors.New("invalid project model response")
	}
	return reply, nil
}

func (c *Config) exchange(turn, request context.Context, client *http.Client, w http.ResponseWriter, in responseRequest) error {
	ctx, cancel := context.WithCancel(request)
	defer cancel()
	stop := context.AfterFunc(turn, cancel)
	defer stop()
	in.Model = c.provider.Model
	tools := c.tools(in.Tools)
	body, err := requestBody(in, tools, c.provider.OutputFormat)
	if err != nil {
		return err
	}
	attachImages(body, c.Images)
	reply, err := c.complete(ctx, client, body)
	if err != nil {
		return err
	}
	choice := reply.Choices[0]
	if choice.Finish != "stop" && choice.Finish != "tool_calls" {
		return errors.New("incomplete project model response")
	}
	output := []obj{}
	if content := choice.Message.Content; content != "" {
		output = append(output, obj{"type": "message", "id": "msg_" + randomID(), "role": "assistant", "status": "completed", "content": []obj{{"type": "output_text", "text": content, "annotations": []any{}}}})
	}
	for _, call := range choice.Message.Calls {
		var target *mountedTool
		for i := range tools {
			if tools[i].alias == call.Function.Name {
				target = &tools[i]
				break
			}
		}
		if target == nil || call.ID == "" || !json.Valid([]byte(call.Function.Arguments)) {
			return errors.New("invalid or unmounted tool requested")
		}
		item := obj{"type": "function_call", "id": "fc_" + randomID(), "call_id": call.ID, "name": target.name, "arguments": call.Function.Arguments, "status": "completed"}
		if target.namespace != "" {
			item["namespace"] = target.namespace
		}
		output = append(output, item)
	}
	if len(output) == 0 {
		return errors.New("project model returned no answer or tool call")
	}
	id := "resp_" + randomID()
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-store")
	event := func(kind string, fields obj) {
		fields["type"] = kind
		b, _ := json.Marshal(fields)
		fmt.Fprintf(w, "event: %s\ndata: %s\n\n", kind, b)
	}
	event("response.created", obj{"response": obj{"id": id, "object": "response", "status": "in_progress", "output": []any{}}})
	for i, item := range output {
		event("response.output_item.done", obj{"output_index": i, "item": item})
	}
	event("response.completed", obj{"response": obj{"id": id, "object": "response", "status": "completed", "output": output, "usage": obj{"input_tokens": reply.Usage.Prompt, "output_tokens": reply.Usage.Generated, "total_tokens": reply.Usage.Prompt + reply.Usage.Generated}}})
	return nil
}
func randomID() string { b := make([]byte, 16); rand.Read(b); return hex.EncodeToString(b) }

// FailureCode exposes only a category. Provider response bodies may contain sensitive data.
func (c *Config) FailureCode() string {
	switch c.providerStatus.Load() {
	case -1:
		return "provider_unreachable"
	case 401, 403:
		return "provider_auth"
	case 404:
		return "provider_not_found"
	case 400, 422:
		return "provider_request"
	case 429:
		return "provider_limit"
	}
	return ""
}

func attachImages(body obj, images []ImageAttachment) {
	if len(images) == 0 {
		return
	}
	messages, ok := body["messages"].([]obj)
	if !ok {
		return
	}
	for i := len(messages) - 1; i >= 0; i-- {
		if messages[i]["role"] != "user" {
			continue
		}
		text, _ := messages[i]["content"].(string)
		parts := make([]obj, 0, 1+len(images))
		if text != "" {
			parts = append(parts, obj{"type": "text", "text": text})
		}
		for _, img := range images {
			if img.MIME == "" || len(img.Data) == 0 {
				continue
			}
			parts = append(parts, obj{
				"type":      "image_url",
				"image_url": obj{"url": "data:" + img.MIME + ";base64," + base64.StdEncoding.EncodeToString(img.Data)},
			})
		}
		messages[i]["content"] = parts
		return
	}
}

// ProposalAnswer uses one frozen input and exposes no tool servers or prior conversation.
func (c *Config) ProposalAnswer(ctx context.Context, prompt string) (string, error) {
	reply, err := c.complete(ctx, completionClient(4*time.Minute), obj{"model": c.provider.Model, "stream": false, "messages": []obj{{"role": "user", "content": prompt}}})
	if err != nil {
		return "", err
	}
	if len(reply.Choices) != 1 || reply.Choices[0].Finish != "stop" || len(reply.Choices[0].Message.Calls) > 0 {
		return "", errors.New("incomplete proposal reply")
	}
	return reply.Choices[0].Message.Content, nil
}
