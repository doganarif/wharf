// Package webhook is a wharf.Notifier that POSTs each notification as JSON to a
// URL — wire it to Slack, Discord, or your own endpoint. Standard library only.
//
//	srv.Notify(webhook.New("https://hooks.example.com/...")).NotifyOnConnect(true)
package webhook

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"

	"wharf"
)

var _ wharf.Notifier = (*Notifier)(nil)

type Notifier struct {
	url    string
	client *http.Client
}

func New(url string) *Notifier {
	return &Notifier{url: url, client: http.DefaultClient}
}

func (n *Notifier) Notify(ctx context.Context, m wharf.Notification) error {
	payload, _ := json.Marshal(map[string]string{
		"title":       m.Title,
		"body":        m.Body,
		"user":        m.User.ShortID,
		"fingerprint": m.User.Fingerprint,
	})
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, n.url, bytes.NewReader(payload))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := n.client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode/100 != 2 {
		return fmt.Errorf("webhook: unexpected status %d", resp.StatusCode)
	}
	return nil
}
