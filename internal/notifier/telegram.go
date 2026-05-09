package notifier

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
)

type Telegram struct {
	Token   string
	ChatID  string
	BaseURL string // e.g. https://api.telegram.org; used for testability
	HTTP    *http.Client
}

func NewTelegram(token, chatID string, http *http.Client) *Telegram {
	return &Telegram{
		Token:   token,
		ChatID:  chatID,
		BaseURL: "https://api.telegram.org",
		HTTP:    http,
	}
}

// InlineButton is one button in a single-row inline keyboard.
type InlineButton struct {
	Text         string `json:"text"`
	CallbackData string `json:"callback_data"`
}

type sendMessageBody struct {
	ChatID      string          `json:"chat_id"`
	Text        string          `json:"text"`
	ParseMode   string          `json:"parse_mode,omitempty"`
	ReplyMarkup *inlineKeyboard `json:"reply_markup,omitempty"`
}

type inlineKeyboard struct {
	InlineKeyboard [][]InlineButton `json:"inline_keyboard"`
}

type sendMessageResp struct {
	OK          bool   `json:"ok"`
	Description string `json:"description"`
}

// Send sends a plain text message (no inline keyboard).
func (t *Telegram) Send(text string) error {
	return t.SendWithButtons(text, nil)
}

// SendWithButtons sends a message with an optional single-row inline keyboard.
// Pass nil or empty buttons to send without buttons (equivalent to Send).
func (t *Telegram) SendWithButtons(text string, buttons []InlineButton) error {
	if t.Token == "" || t.ChatID == "" {
		return fmt.Errorf("telegram: token or chat_id not configured")
	}
	msg := sendMessageBody{ChatID: t.ChatID, Text: text}
	if len(buttons) > 0 {
		msg.ReplyMarkup = &inlineKeyboard{
			InlineKeyboard: [][]InlineButton{buttons},
		}
	}
	body, _ := json.Marshal(msg)
	url := fmt.Sprintf("%s/bot%s/sendMessage", t.BaseURL, t.Token)
	req, _ := http.NewRequest("POST", url, bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	resp, err := t.HTTP.Do(req)
	if err != nil {
		return fmt.Errorf("telegram http: %w", err)
	}
	defer resp.Body.Close()
	respBody, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != 200 {
		return fmt.Errorf("telegram HTTP %d: %s", resp.StatusCode, string(respBody))
	}
	var r sendMessageResp
	if err := json.Unmarshal(respBody, &r); err != nil {
		return fmt.Errorf("telegram parse: %w", err)
	}
	if !r.OK {
		return fmt.Errorf("telegram api: %s", r.Description)
	}
	return nil
}
