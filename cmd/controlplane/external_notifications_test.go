package main

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"forgeos/internal/store"
)

type localNoticeTransport struct{ target *url.URL }

func (t localNoticeTransport) RoundTrip(r *http.Request) (*http.Response, error) {
	copy := r.Clone(r.Context())
	copy.URL.Host, copy.URL.Scheme = t.target.Host, t.target.Scheme
	return http.DefaultTransport.RoundTrip(copy)
}

func TestExternalNoticeRequiresProviderReceipt(t *testing.T) {
	for _, tc := range []struct {
		channel, reply string
		status         int
		sent           bool
		retry          int
	}{
		{"telegram", `{"ok":true,"result":{"message_id":7}}`, 200, true, 0},
		{"telegram", `{"ok":true}`, 200, false, 0},
		{"telegram", `{"ok":false,"parameters":{"retry_after":90}}`, 429, false, 90},
		{"feishu", `{"code":0}`, 200, true, 0},
		{"feishu", `{"code":1}`, 200, false, 0},
		{"feishu", `{}`, 200, false, 0},
		{"feishu", `{"code":0}`, 503, false, 0},
	} {
		t.Run(tc.channel, func(t *testing.T) {
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Method != http.MethodPost {
					t.Error(r.Method)
				}
				w.WriteHeader(tc.status)
				fmt.Fprint(w, tc.reply)
			}))
			defer srv.Close()
			target, err := url.Parse(srv.URL)
			if err != nil {
				t.Fatal(err)
			}
			client := &http.Client{Transport: localNoticeTransport{target}}
			sent, retry := (noticeChannel{Channel: tc.channel, Token: "qa", Chat: "1"}).send(context.Background(), client, store.Notification{ID: 1, ProjectID: 2, TaskID: 3, Kind: "failure"})
			if sent != tc.sent || retry != tc.retry {
				t.Fatalf("sent=%t retry=%d", sent, retry)
			}
		})
	}
}

func TestNoticeConfigRejectsMalformedDestinations(t *testing.T) {
	path := filepath.Join(t.TempDir(), "channels.json")
	for _, value := range []string{
		`[{"project_id":1,"channel":"telegram","token":"123:qa/path","chat_id":"10"}]`,
		`[{"project_id":1,"channel":"telegram","token":"123:qa","chat_id":"0"}]`,
		`[{"project_id":1,"channel":"feishu","token":"https://example.invalid/qa"}]`,
		`[{"project_id":0,"channel":"feishu","token":"qa"}]`,
		`[{"project_id":1,"channel":"feishu","token":"qa"},{"project_id":1,"channel":"feishu","token":"other-qa"}]`,
		`[{"project_id":1,"channel":"feishu","token":"qa","endpoint":"qa"}]`,
	} {
		if err := os.WriteFile(path, []byte(value), 0600); err != nil {
			t.Fatal(err)
		}
		if _, err := loadNoticeChannels(path); err == nil || strings.Contains(err.Error(), "qa") {
			t.Fatal("invalid destination accepted or exposed")
		}
	}
}
