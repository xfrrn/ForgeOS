package chatkernel

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"
)

var ErrInvalidObject = errors.New("模型未返回完整的 JSON 对象")

// CompleteObject runs a tool-free turn against the configured model. The role
// supplies its schema and instructions separately from the source evidence.
func CompleteObject(ctx context.Context, p Provider, name, schema, instruction string, input json.RawMessage) (json.RawMessage, error) {
	if !p.Valid() || name == "" || !json.Valid([]byte(schema)) || !json.Valid(input) {
		return nil, errors.New("模型连接或结构化请求无效")
	}
	quoted, err := json.Marshal(string(input))
	if err != nil {
		return nil, err
	}
	in := responseRequest{Model: p.Model, Instructions: instruction, Input: []item{{Role: "user", Content: quoted}}}
	in.Text.Format = obj{"type": "json_schema", "name": name, "strict": true, "schema": json.RawMessage(schema)}
	body, err := requestBody(in, nil, p.OutputFormat)
	if err != nil {
		return nil, errors.New("无法生成结构化请求")
	}
	cfg := Config{provider: p}
	reply, err := cfg.complete(ctx, completionClient(2*time.Minute), body)
	if err != nil {
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		if code := cfg.providerStatus.Load(); code > 0 && code != 200 {
			return nil, fmt.Errorf("模型服务返回 HTTP %d", code)
		}
		if cfg.providerStatus.Load() == 200 {
			return nil, ErrInvalidObject
		}
		return nil, errors.New("模型连接失败或响应格式无效")
	}
	if len(reply.Choices) != 1 {
		return nil, ErrInvalidObject
	}
	choice := reply.Choices[0]
	raw := bytes.TrimSpace([]byte(choice.Message.Content))
	if choice.Finish != "stop" || len(choice.Message.Calls) != 0 || len(raw) == 0 || len(raw) > 64<<10 || raw[0] != '{' || !json.Valid(raw) {
		return nil, ErrInvalidObject
	}
	return raw, nil
}
