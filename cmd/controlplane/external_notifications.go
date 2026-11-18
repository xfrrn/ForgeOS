package main

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"strconv"
	"time"

	"forgeos/internal/store"
)

type noticeChannel struct {
	Project int64  `json:"project_id"`
	Channel string `json:"channel"`
	Token   string `json:"token"`
	Chat    string `json:"chat_id"`
	Secret  string `json:"signing_secret"`
}

var telegramToken = regexp.MustCompile(`^[0-9]+:[A-Za-z0-9_-]+$`)
var feishuKey = regexp.MustCompile(`^[A-Za-z0-9-]+$`)

func loadNoticeChannels(path string) ([]noticeChannel, error) {
	if path == "" {
		return nil, nil
	}
	info, err := os.Lstat(path)
	if err != nil || !filepath.IsAbs(path) || !info.Mode().IsRegular() || info.Size() > 64<<10 || runtime.GOOS != "windows" && info.Mode().Perm()&0077 != 0 {
		return nil, errors.New("external notifications require a private configuration file at an absolute path")
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, errors.New("cannot read notification configuration")
	}
	var channels []noticeChannel
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.DisallowUnknownFields()
	if dec.Decode(&channels) != nil || dec.Decode(new(any)) != io.EOF || len(channels) == 0 {
		return nil, errors.New("invalid notification configuration")
	}
	seen := map[string]bool{}
	for _, c := range channels {
		key := fmt.Sprintf("%d/%s", c.Project, c.Channel)
		chat, chatErr := strconv.ParseInt(c.Chat, 10, 64)
		validTelegram := c.Channel == "telegram" && telegramToken.MatchString(c.Token) && chatErr == nil && chat != 0 && c.Secret == ""
		validFeishu := c.Channel == "feishu" && feishuKey.MatchString(c.Token) && c.Chat == ""
		if c.Project <= 0 || seen[key] || (!validTelegram && !validFeishu) {
			return nil, errors.New("invalid or duplicate project notification channel")
		}
		seen[key] = true
	}
	return channels, nil
}

// Payloads contain identifiers and a bounded event kind; project text stays in the workbench.
func (c noticeChannel) send(ctx context.Context, client *http.Client, n store.Notification) (sent bool, retry int) {
	text := fmt.Sprintf("ForgeOS 项目 #%d · 任务 #%d · %s · 通知 #%d", n.ProjectID, n.TaskID, n.Kind, n.ID)
	var endpoint string
	var payload map[string]any
	switch c.Channel {
	case "telegram":
		endpoint = "https://api.telegram.org/bot" + c.Token + "/sendMessage"
		payload = map[string]any{"chat_id": c.Chat, "text": text}
	case "feishu":
		endpoint = "https://open.feishu.cn/open-apis/bot/v2/hook/" + c.Token
		payload = map[string]any{"msg_type": "text", "content": map[string]string{"text": text}}
		if c.Secret != "" {
			stamp := strconv.FormatInt(time.Now().Unix(), 10)
			h := hmac.New(sha256.New, []byte(stamp+"\n"+c.Secret))
			payload["timestamp"], payload["sign"] = stamp, base64.StdEncoding.EncodeToString(h.Sum(nil))
		}
	default:
		return false, 0
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return false, 0
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(body))
	if err != nil {
		return false, 0
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := client.Do(req)
	if err != nil {
		return false, 0
	}
	defer resp.Body.Close()
	retry, _ = strconv.Atoi(resp.Header.Get("Retry-After"))
	raw, err := io.ReadAll(io.LimitReader(resp.Body, 65537))
	if err != nil || len(raw) > 65536 {
		return false, retry
	}
	var receipt struct {
		OK     bool
		Code   *int
		Result struct {
			MessageID int64 `json:"message_id"`
		}
		Parameters struct {
			Retry int `json:"retry_after"`
		}
	}
	if json.Unmarshal(raw, &receipt) != nil {
		return false, retry
	}
	retry = max(retry, receipt.Parameters.Retry)
	if resp.StatusCode != http.StatusOK {
		return false, retry
	}
	if c.Channel == "telegram" {
		return receipt.OK && receipt.Result.MessageID > 0, retry
	}
	return receipt.Code != nil && *receipt.Code == 0, retry
}

func (c noticeChannel) dispatch(ctx context.Context, s *store.Store, client *http.Client) error {
	n, attempt, err := s.ClaimExternalNotice(ctx, c.Project, c.Channel)
	if err != nil || attempt == 0 {
		return err
	}
	sent, retry := c.send(ctx, client, n)
	backoff := min(3600, 15<<min(attempt-1, 8))
	return s.FinishExternalNotice(ctx, n.ID, c.Channel, attempt, sent, max(min(retry, 86400), backoff))
}

func (c noticeChannel) run(ctx context.Context, s *store.Store) {
	client := &http.Client{Timeout: 10 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	tick := time.NewTicker(3 * time.Second)
	defer tick.Stop()
	for ctx.Err() == nil {
		if err := c.dispatch(ctx, s, client); err != nil && ctx.Err() == nil {
			log.Printf("notification delivery state unavailable: project %d, channel %s", c.Project, c.Channel)
		}
		select {
		case <-ctx.Done():
			return
		case <-tick.C:
		}
	}
}
