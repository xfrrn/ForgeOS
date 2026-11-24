package chatkernel

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"forgeos/internal/skilllib"
	"forgeos/schemas"
)

func TestMain(m *testing.M) {
	if len(os.Args) == 4 && os.Args[1] == "project-test-mcp" {
		if skilllib.Serve(os.Stdin, os.Stdout, os.Args[2], []string{os.Args[3]}) != nil {
			os.Exit(1)
		}
		os.Exit(0)
	}
	os.Exit(m.Run())
}

func TestProjectChatKernel(t *testing.T) {
	bin := os.Getenv("FORGEOS_TEST_CHAT_KERNEL")
	if bin == "" {
		t.Skip("set FORGEOS_TEST_CHAT_KERNEL to the built ForgeOS chat kernel")
	}
	root := t.TempDir()
	t.Setenv("CODEX_HOME", t.TempDir())
	key := strings.Repeat("a", 64)
	folder := filepath.Join(root, key)
	if err := os.Mkdir(folder, 0700); err != nil {
		t.Fatal(err)
	}
	note := filepath.Join(folder, "SKILL.md")
	if err := os.WriteFile(note, []byte("project fixture memory"), 0600); err != nil {
		t.Fatal(err)
	}
	credential := "FORGEOS_CHAT_PROVIDER_KEY_TEST"
	t.Setenv(credential, "fixture-token")
	calls := 0
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/chat/completions" || r.Header.Get("Authorization") != "Bearer fixture-token" {
			t.Error("incorrect upstream route or credentials")
			http.Error(w, "invalid", 400)
			return
		}
		var request struct {
			Model    string
			Messages []struct{ Role, Content string }
			Tools    []struct{ Function struct{ Name string } }
			Format   struct{ Type string } `json:"response_format"`
		}
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
			t.Error(err)
			http.Error(w, "invalid", 400)
			return
		}
		if request.Model != "project-model" || request.Format.Type != "json_schema" || len(request.Tools) != 1 {
			t.Errorf("model=%s format=%s tools=%+v", request.Model, request.Format.Type, request.Tools)
			http.Error(w, "invalid", 400)
			return
		}
		calls++
		choice := obj{"finish_reason": "stop", "message": obj{"role": "assistant", "content": `{"answer":"verified"}`}}
		if calls == 1 {
			args, _ := json.Marshal(obj{"path": note})
			choice = obj{"finish_reason": "tool_calls", "message": obj{"role": "assistant", "tool_calls": []obj{{"id": "call_fixture", "type": "function", "function": obj{"name": request.Tools[0].Function.Name, "arguments": string(args)}}}}}
		} else if last := request.Messages[len(request.Messages)-1]; calls != 2 || last.Role != "tool" || !strings.Contains(last.Content, "project fixture memory") {
			t.Errorf("expected scoped tool result, calls=%d role=%s", calls, last.Role)
			http.Error(w, "invalid", 400)
			return
		}
		json.NewEncoder(w).Encode(obj{"choices": []obj{choice}})
	}))
	defer upstream.Close()
	exe, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	c := Config{ProjectID: 1, provider: Provider{Model: "project-model", BaseURL: upstream.URL + "/v1", APIKey: "fixture-token"}, Servers: map[string]ToolServer{"project_notes": {Command: exe, Args: []string{"project-test-mcp", root, key}, Tools: []string{"read"}}}}
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	base, closeBridge, err := c.Start(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer closeBridge()
	schema := filepath.Join(root, "schema.json")
	output := filepath.Join(root, "out.json")
	if err := os.WriteFile(schema, []byte(`{"type":"object","properties":{"answer":{"type":"string"}},"required":["answer"],"additionalProperties":false}`), 0600); err != nil {
		t.Fatal(err)
	}
	args := []string{"-a", "never", "exec", "--ignore-user-config", "--ignore-rules", "--sandbox", "read-only", "--json", "--skip-git-repo-check", "-C", root, "--output-schema", schema, "-o", output, "-c", "mcp_servers={}"}
	args = append(args, "-c", "projects."+strconv.Quote(root)+`.trust_level="untrusted"`)
	for _, f := range schemas.StructuredCodexProfile().DisabledFeatures {
		args = append(args, "--disable", f)
	}
	args = append(args, c.Arguments(base)...)
	args = append(args, "Read the mounted project note and return the answer.")
	cmd := exec.CommandContext(ctx, bin, args...)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("Codex: %v: %.4000s", err, out)
	}
	out, err := os.ReadFile(output)
	if err != nil || !strings.Contains(string(out), "verified") || calls != 2 {
		t.Fatalf("output=%q calls=%d err=%v", out, calls, err)
	}
}

func TestRejectDifferentProject(t *testing.T) {
	path := filepath.Join(t.TempDir(), "project.json")
	if err := os.WriteFile(path, []byte(`{"project_id":2}`), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := ForProject(path, 1, Provider{Model: "local", BaseURL: "http://localhost:8000/v1", APIKey: "test"}); err == nil {
		t.Fatal("accepted another project's configuration")
	}
}

func TestJSONObjectAndToolScope(t *testing.T) {
	var in responseRequest
	raw := `{"model":"local","text":{"format":{"type":"json_schema","name":"answer","schema":{"type":"object"}}},"tools":[{"type":"function","name":"shell"},{"type":"namespace","name":"mcp__other","tools":[{"type":"function","name":"read"}]}]}`
	if err := json.Unmarshal([]byte(raw), &in); err != nil {
		t.Fatal(err)
	}
	cfg := Config{Servers: map[string]ToolServer{"current": {Tools: []string{"read"}}}}
	tools := cfg.tools(in.Tools)
	if len(tools) != 0 {
		t.Fatal("offered unmounted project tools")
	}
	payload, err := requestBody(in, tools, "json_object")
	if err != nil {
		t.Fatal(err)
	}
	format := payload["response_format"].(obj)
	messages := payload["messages"].([]obj)
	if format["type"] != "json_object" || len(messages) != 1 || !strings.Contains(messages[0]["content"].(string), `"type":"object"`) {
		t.Fatal("JSON object fallback lost the output contract")
	}
}

func TestAttachImagesMultimodalUserMessage(t *testing.T) {
	body := obj{
		"messages": []obj{
			{"role": "system", "content": "sys"},
			{"role": "user", "content": "check screenshot"},
		},
	}
	attachImages(body, []ImageAttachment{{MIME: "image/png", Data: []byte("abc")}})
	msgs := body["messages"].([]obj)
	parts, ok := msgs[1]["content"].([]obj)
	if !ok || len(parts) != 2 {
		t.Fatalf("expected 2 multimodal parts, got %#v", msgs[1]["content"])
	}
	if parts[0]["type"] != "text" || parts[0]["text"] != "check screenshot" {
		t.Fatalf("unexpected text part: %#v", parts[0])
	}
	imgURL := parts[1]["image_url"].(obj)["url"].(string)
	if parts[1]["type"] != "image_url" || imgURL != "data:image/png;base64,YWJj" {
		t.Fatalf("unexpected image part: %#v", parts[1])
	}
}
