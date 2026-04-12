package alert

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"time"
)

// Sender sends alert notifications.
type Sender interface {
	Send(title, content string) error
}

// DingTalk sends alerts via DingTalk webhook.
type DingTalk struct {
	webhookURL string
	client     *http.Client
}

func NewDingTalk(webhookURL string) *DingTalk {
	return &DingTalk{
		webhookURL: webhookURL,
		client:     &http.Client{Timeout: 10 * time.Second},
	}
}

func (d *DingTalk) Send(title, content string) error {
	payload := map[string]any{
		"msgtype": "markdown",
		"markdown": map[string]string{
			"title": title,
			"text":  fmt.Sprintf("## %s\n\n%s", title, content),
		},
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("dingtalk marshal: %w", err)
	}
	resp, err := d.client.Post(d.webhookURL, "application/json", bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("dingtalk send: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("dingtalk: status %d", resp.StatusCode)
	}
	var result struct {
		ErrCode int    `json:"errcode"`
		ErrMsg  string `json:"errmsg"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err == nil && result.ErrCode != 0 {
		return fmt.Errorf("dingtalk API error %d: %s", result.ErrCode, result.ErrMsg)
	}
	return nil
}

// Feishu sends alerts via Feishu (Lark) webhook.
type Feishu struct {
	webhookURL string
	client     *http.Client
}

func NewFeishu(webhookURL string) *Feishu {
	return &Feishu{
		webhookURL: webhookURL,
		client:     &http.Client{Timeout: 10 * time.Second},
	}
}

func (f *Feishu) Send(title, content string) error {
	payload := map[string]any{
		"msg_type": "interactive",
		"card": map[string]any{
			"header": map[string]any{
				"title": map[string]string{"tag": "plain_text", "content": title},
			},
			"elements": []map[string]any{
				{"tag": "markdown", "content": content},
			},
		},
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("feishu marshal: %w", err)
	}
	resp, err := f.client.Post(f.webhookURL, "application/json", bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("feishu send: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("feishu: status %d", resp.StatusCode)
	}
	var result struct {
		Code int    `json:"code"`
		Msg  string `json:"msg"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err == nil && result.Code != 0 {
		return fmt.Errorf("feishu API error %d: %s", result.Code, result.Msg)
	}
	return nil
}

// Noop discards alerts silently. Used when no webhook is configured.
type Noop struct{}

func (Noop) Send(string, string) error { return nil }
