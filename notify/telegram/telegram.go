// Package telegram is a wharf.Notifier that sends pings to a Telegram chat via
// the Bot API. Standard library only.
//
//	srv.Notify(telegram.New(botToken, chatID)).NotifyOnConnect(true)
package telegram

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
	token  string
	chatID string
	client *http.Client
}

func New(botToken, chatID string) *Notifier {
	return &Notifier{token: botToken, chatID: chatID, client: http.DefaultClient}
}

func (n *Notifier) Notify(ctx context.Context, m wharf.Notification) error {
	text := m.Title
	if m.Body != "" {
		if text != "" {
			text += "\n"
		}
		text += m.Body
	}
	payload, _ := json.Marshal(map[string]string{"chat_id": n.chatID, "text": text})
	url := fmt.Sprintf("https://api.telegram.org/bot%s/sendMessage", n.token)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(payload))
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
		return fmt.Errorf("telegram: unexpected status %d", resp.StatusCode)
	}
	return nil
}
