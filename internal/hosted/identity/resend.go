package identity

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"time"
)

type Resend struct {
	APIKey, From string
	Client       *http.Client
}

func (s *Resend) Send(ctx context.Context, to, link string) error {
	if s.APIKey == "" || s.From == "" {
		return errors.New("email provider is not configured")
	}
	body, err := json.Marshal(map[string]any{"from": s.From, "to": []string{to}, "subject": "Sign in to Serenity", "text": "Sign in to your private Serenity memory:\n\n" + link + "\n\nThis link expires in 15 minutes and works once. If you did not request it, ignore this email."})
	if err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, "https://api.resend.com/emails", bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+s.APIKey)
	req.Header.Set("Content-Type", "application/json")
	client := s.Client
	if client == nil {
		client = &http.Client{Timeout: 15 * time.Second}
	}
	resp, err := client.Do(req)
	if err != nil {
		return errors.New("email provider request failed")
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("email provider returned status %d", resp.StatusCode)
	}
	return nil
}

type DevSender struct{ Writer io.Writer }

func (s *DevSender) Send(_ context.Context, _ string, link string) error {
	_, err := fmt.Fprintln(s.Writer, "Development login:", link)
	return err
}
