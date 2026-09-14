// Package email sends best-effort transactional notifications via the Resend
// API (https://resend.com). Nothing in this package blocks or fails the
// caller's own operation — a missing config or a failed request just returns
// an error for the caller to log.
package email

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/tobiasmarkusbraun/rgblitz/internal/db"
)

const resendEmailsURL = "https://api.resend.com/emails"

type ResendClient struct {
	apiKey string
	from   string
	to     string
	http   *http.Client
}

func NewResendClient(apiKey, from, to string) *ResendClient {
	return &ResendClient{
		apiKey: apiKey,
		from:   from,
		to:     to,
		http:   &http.Client{Timeout: 10 * time.Second},
	}
}

type resendEmailRequest struct {
	From    string   `json:"from"`
	To      []string `json:"to"`
	Subject string   `json:"subject"`
	Text    string   `json:"text"`
}

// SendFeedbackNotification emails fb to the configured recipient. It no-ops
// (returns nil) if the client isn't configured with an API key and
// recipient, so the feature works without Resend set up in dev/test.
func (c *ResendClient) SendFeedbackNotification(fb *db.Feedback) error {
	if c.apiKey == "" || c.to == "" {
		return nil
	}

	body := resendEmailRequest{
		From:    c.from,
		To:      []string{c.to},
		Subject: fmt.Sprintf("Neues Feedback (#%d, %d★)", fb.ID, fb.Rating),
		Text: fmt.Sprintf(
			"Feedback #%d\nZeit: %s\nSterne: %d/5\n\n%s",
			fb.ID, fb.CreatedAt.Format(time.RFC3339), fb.Rating, fb.Text,
		),
	}
	payload, err := json.Marshal(body)
	if err != nil {
		return fmt.Errorf("marshal resend request: %w", err)
	}

	req, err := http.NewRequest(http.MethodPost, resendEmailsURL, bytes.NewReader(payload))
	if err != nil {
		return fmt.Errorf("build resend request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+c.apiKey)
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.http.Do(req)
	if err != nil {
		return fmt.Errorf("send resend request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		respBody, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		return fmt.Errorf("resend request failed: status %d: %s", resp.StatusCode, respBody)
	}
	return nil
}
