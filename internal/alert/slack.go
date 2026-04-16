package alert

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"time"
)

// Slack sends alerts via a Slack incoming webhook.
type Slack struct {
	webhookURL string
	client     *http.Client
}

func NewSlack(webhookURL string) *Slack {
	return &Slack{
		webhookURL: webhookURL,
		client:     &http.Client{Timeout: 5 * time.Second},
	}
}

func (s *Slack) Send(title, content string) error {
	payload := map[string]string{
		"text": fmt.Sprintf("*%s*\n%s", title, content),
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("slack marshal: %w", err)
	}
	resp, err := s.client.Post(s.webhookURL, "application/json", bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("slack send: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("slack: status %d", resp.StatusCode)
	}
	return nil
}
