package aioncore

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"net/url"
	"testing"
	"time"
)

func TestConfigureModelAppliesOnNextTurn(t *testing.T) {
	mode := "write"
	modelSet := false
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/conversations/c/runtime/ensure":
			fmt.Fprintf(w, `{"success":true,"data":{"config_options":[{"id":"model","current_value":"old","options":[{"value":"next"}]},{"id":"mode","current_value":%q,"options":[{"value":"read-only"}]}]}}`, mode)
		case "/api/conversations/c/config-options/model":
			modelSet = true
			fmt.Fprint(w, `{"success":true,"data":{"confirmation":"pending_next_turn"}}`)
		case "/api/conversations/c/config-options/mode":
			if !modelSet {
				t.Error("permissions set before model selection")
			}
			mode = "read-only"
			fmt.Fprint(w, `{"success":true,"data":{}}`)
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()
	base, _ := url.Parse(server.URL)
	httpClient := server.Client()
	httpClient.Jar, _ = cookiejar.New(nil)
	client := Client{base: base, http: httpClient}
	if err := client.Configure(context.Background(), "c", "codex", "next"); err != nil {
		t.Fatal(err)
	}
	if mode != "read-only" || !modelSet {
		t.Fatal("incomplete runtime configuration")
	}
	if err := client.Configure(context.Background(), "c", "codex", "missing"); err == nil {
		t.Fatal("unavailable model accepted")
	}
}

func TestCancelWaitsForIdleAndPinsTurn(t *testing.T) {
	checked := false
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/conversations/old/cancel":
			var body map[string]string
			if r.Method != "POST" || json.NewDecoder(r.Body).Decode(&body) != nil || body["turn_id"] != "original" {
				t.Error("cancel did not target the submitted turn")
			}
			fmt.Fprint(w, `{"success":true,"data":{"runtime":{"state":"cancelling","is_processing":true}}}`)
		case "/api/conversations/old":
			checked = true
			fmt.Fprint(w, `{"success":true,"data":{"runtime":{"state":"idle","is_processing":false}}}`)
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()
	base, _ := url.Parse(server.URL)
	httpClient := server.Client()
	httpClient.Jar, _ = cookiejar.New(nil)
	client := Client{base: base, http: httpClient}
	ctx, done := context.WithTimeout(context.Background(), 2*time.Second)
	defer done()
	if err := client.Cancel(ctx, "old", "original"); err != nil || !checked {
		t.Fatal("no cancellation acknowledgement", err)
	}
	if err := client.CheckSession(ctx, "missing"); err != ErrSessionMissing {
		t.Fatal(err)
	}
	if ProbeFailure(HTTPError(http.StatusForbidden)) != "auth" || ProbeFailure(context.DeadlineExceeded) != "timeout" {
		t.Fatal("incorrect probe status")
	}
}

func TestDocumentSubmissionStaysInPlanMode(t *testing.T) {
	var saved string
	stopped := false
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/conversations/doc":
			if stopped {
				fmt.Fprint(w, `{"success":true,"data":{"runtime":{"state":"idle","is_processing":false}}}`)
				return
			}
			fmt.Fprint(w, `{"success":true,"data":{"status":"pending","runtime":{"state":"waiting_confirmation","turn_id":"original","is_processing":true}}}`)
		case "/api/conversations/doc/messages":
			fmt.Fprint(w, `{"success":true,"data":{"items":[{"position":"left","type":"tool_call","status":"work","content":{"name":"ExitPlanMode","input":{"plan":"earlier turn"}}},{"position":"right","id":"q","type":"text","content":{"content":"owner request"}},{"position":"left","type":"tool_call","status":"work","content":{"name":"ExitPlanMode","input":{"plan":"submitted design"}}}]}}`)
		case "/api/conversations/doc/cancel":
			var body map[string]string
			if saved != "submitted design" || json.NewDecoder(r.Body).Decode(&body) != nil || body["turn_id"] != "original" {
				t.Error("unrecorded or wrong turn stopped")
			}
			stopped = true
			fmt.Fprint(w, `{"success":true,"data":{"runtime":{"state":"idle","is_processing":false}}}`)
		default:
			t.Errorf("unexpected operation %s", r.URL.Path)
			http.NotFound(w, r)
		}
	}))
	defer server.Close()
	base, _ := url.Parse(server.URL)
	httpClient := server.Client()
	httpClient.Jar, _ = cookiejar.New(nil)
	client := Client{base: base, http: httpClient}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	doc, err := client.DocumentReply(ctx, "doc", "q", "marker", "original", func(value string) error { saved = value; return nil })
	if err != nil || doc != saved || !stopped {
		t.Fatal(doc, err)
	}
	if err = client.StopDocumentTurn(ctx, "doc", "original"); err != nil {
		t.Fatal("could not recover stopped turn", err)
	}
}

func TestUploadFileAndSendWithFiles(t *testing.T) {
	var uploadedName, uploadedConv string
	var uploadedBytes []byte
	var gotFiles []ChatFileRef
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/fs/upload":
			if err := r.ParseMultipartForm(1 << 20); err != nil {
				t.Error(err)
				http.Error(w, "bad multipart", 400)
				return
			}
			uploadedName = r.FormValue("file_name")
			uploadedConv = r.FormValue("conversation_id")
			f, _, err := r.FormFile("file")
			if err != nil {
				t.Error(err)
				http.Error(w, "missing file", 400)
				return
			}
			defer f.Close()
			buf := make([]byte, 64)
			n, _ := f.Read(buf)
			uploadedBytes = buf[:n]
			fmt.Fprint(w, `{"success":true,"data":"C:\\temp\\aionui\\shot.png"}`)
		case "/api/conversations/c1/messages":
			var body struct {
				Content string        `json:"content"`
				Files   []ChatFileRef `json:"files"`
			}
			if err := json.NewDecoder(r.Body).Decode(&body); err != nil || body.Content != "check image" {
				t.Errorf("unexpected send body: %+v err=%v", body, err)
				http.Error(w, "bad send", 400)
				return
			}
			gotFiles = body.Files
			fmt.Fprint(w, `{"success":true,"data":{"msg_id":"m1","turn_id":"t1"}}`)
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()
	base, _ := url.Parse(server.URL)
	httpClient := server.Client()
	httpClient.Jar, _ = cookiejar.New(nil)
	client := Client{base: base, http: httpClient}
	path, err := client.UploadFile(context.Background(), "c1", "shot.png", []byte("png-bytes"))
	if err != nil || path != `C:\temp\aionui\shot.png` || uploadedName != "shot.png" || uploadedConv != "c1" || string(uploadedBytes) != "png-bytes" {
		t.Fatalf("upload failed: path=%q name=%q conv=%q bytes=%q err=%v", path, uploadedName, uploadedConv, uploadedBytes, err)
	}
	receipt, err := client.SendWithFiles(context.Background(), "c1", "check image", []ChatFileRef{{Kind: "upload", Path: path}})
	if err != nil || receipt.Message != "m1" || receipt.Turn != "t1" || len(gotFiles) != 1 || gotFiles[0].Kind != "upload" || gotFiles[0].Path != path {
		t.Fatalf("send with files failed: receipt=%+v files=%+v err=%v", receipt, gotFiles, err)
	}
}

func TestProbeModels(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/conversations/p1/runtime/ensure" {
			http.NotFound(w, r)
			return
		}
		fmt.Fprint(w, `{"success":true,"data":{"config_options":[{"id":"model","current_value":"gpt-5.6-sol","options":[{"value":"gpt-5.6-sol","name":"GPT-5.6 Sol"},{"value":"gpt-5.4","name":"GPT-5.4"}]}]}}`)
	}))
	defer server.Close()
	base, _ := url.Parse(server.URL)
	httpClient := server.Client()
	httpClient.Jar, _ = cookiejar.New(nil)
	client := Client{base: base, http: httpClient}
	models, err := client.ProbeModels(context.Background(), "p1")
	if err != nil || len(models) != 2 || models[0].ID != "gpt-5.6-sol" || models[1].ID != "gpt-5.4" || models[1].Label != "GPT-5.4" {
		t.Fatalf("unexpected probed models: %+v err=%v", models, err)
	}
}
