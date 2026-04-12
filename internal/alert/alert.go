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
	WebhookURL string
	client     *http.Client
}

func NewDingTalk(webhookURL string) *DingTalk {
	return &DingTalk{
		WebhookURL: webhookURL,
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
	body, _ := json.Marshal(payload)
	resp, err := d.client.Post(d.WebhookURL, "application/json", bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("dingtalk send: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("dingtalk: status %d", resp.StatusCode)
	}
	return nil
}

// Feishu sends alerts via Feishu (Lark) webhook.
type Feishu struct {
	WebhookURL string
	client     *http.Client
}

func NewFeishu(webhookURL string) *Feishu {
	return &Feishu{
		WebhookURL: webhookURL,
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
	body, _ := json.Marshal(payload)
	resp, err := f.client.Post(f.WebhookURL, "application/json", bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("feishu send: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("feishu: status %d", resp.StatusCode)
	}
	return nil
}

// Noop discards alerts silently. Used when no webhook is configured.
type Noop struct{}

func (Noop) Send(string, string) error { return nil }
