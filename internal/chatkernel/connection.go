package chatkernel

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"time"
)

func completionClient(timeout time.Duration) *http.Client {
	return &http.Client{Timeout: timeout, CheckRedirect: func(*http.Request, []*http.Request) error {
		return http.ErrUseLastResponse
	}}
}

// TestConnection exercises the configured Chat Completions format with a fixed
// prompt. Only a locally generated diagnosis leaves this function.
func TestConnection(ctx context.Context, p Provider) error {
	if !p.Valid() {
		return errors.New("请检查模型 ID、接口地址和 API Key。")
	}
	ctx, cancel := context.WithTimeout(ctx, 20*time.Second)
	defer cancel()
	cfg := Config{provider: p}
	in := responseRequest{Model: p.Model, Input: []item{{Role: "user", Content: json.RawMessage(`"Return only the JSON object {\"ok\":true}."`)}}}
	in.Text.Format = map[string]any{"type": "json_schema", "name": "connection_test", "strict": true, "schema": obj{
		"type": "object", "properties": obj{"ok": obj{"type": "boolean"}}, "required": []string{"ok"}, "additionalProperties": false,
	}}
	body, err := requestBody(in, nil, p.OutputFormat)
	if err != nil {
		return errors.New("无法生成测试请求。")
	}
	reply, err := cfg.complete(ctx, completionClient(20*time.Second), body)
	if err != nil {
		if errors.Is(ctx.Err(), context.DeadlineExceeded) {
			return errors.New("模型服务在 20 秒内没有完成回复，请稍后重试。")
		}
		code := cfg.providerStatus.Load()
		switch code {
		case -1:
			return errors.New("无法连接模型服务，请检查 Base URL、网络或代理设置。")
		case 401, 403:
			return fmt.Errorf("模型服务拒绝访问（HTTP %d），请检查 API Key 和模型权限。", code)
		case 402, 429:
			return fmt.Errorf("模型服务限制了请求（HTTP %d），请检查余额、配额或稍后重试。", code)
		case 404:
			return errors.New("模型或接口不存在（HTTP 404），请检查模型 ID 和 Base URL。")
		case 400, 422:
			return fmt.Errorf("模型服务拒绝测试请求（HTTP %d），请检查模型 ID 和回复格式。", code)
		case 200:
			return errors.New("服务已响应，但没有返回可读取的 Chat Completions 回复。")
		default:
			return fmt.Errorf("模型服务返回 HTTP %d，请检查接口地址或稍后重试。", code)
		}
	}
	var check struct{ OK bool }
	choice := reply.Choices[0]
	if choice.Finish != "stop" || json.Unmarshal([]byte(choice.Message.Content), &check) != nil || !check.OK {
		return errors.New("服务已响应，但回复不符合测试要求，请检查模型和回复格式。")
	}
	return nil
}
