package notifier

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strconv"
	"strings"
	"time"
)

// CommandHandler is called when a /command is received from the authorized chat.
// It returns reply text (empty string = no reply).
type CommandHandler func(ctx context.Context, args string) string

// CallbackHandler is called when a callback_query is received from the authorized chat.
// It returns (replyText, editOriginal). When editOriginal is true, the inline
// keyboard on the originating message is cleared to prevent double-tapping.
type CallbackHandler func(ctx context.Context, data string) (reply string, editOriginal bool)

// Bot is a long-polling Telegram bot that dispatches incoming /commands and
// callback_query events to registered handlers.
type Bot struct {
	Telegram   *Telegram
	AuthChatID string // only act on messages/callbacks from this chat ID
	Log        *slog.Logger
	Commands   map[string]CommandHandler  // key: "/status" etc.
	Callbacks  map[string]CallbackHandler // key: callback_data string

	offset int64
}

// Run starts the long-poll loop. It blocks until ctx is cancelled.
func (b *Bot) Run(ctx context.Context) {
	for {
		if ctx.Err() != nil {
			return
		}
		if err := b.pollOnce(ctx); err != nil {
			if b.Log != nil {
				b.Log.Error("bot poll", "err", err)
			}
			select {
			case <-ctx.Done():
				return
			case <-time.After(5 * time.Second):
			}
		}
	}
}

// pollOnce calls getUpdates once and dispatches any received updates.
func (b *Bot) pollOnce(ctx context.Context) error {
	urlStr := fmt.Sprintf("%s/bot%s/getUpdates?timeout=30&offset=%d",
		b.Telegram.BaseURL, b.Telegram.Token, b.offset+1)

	// Use a slightly longer timeout than the server-side long poll (35s > 30s).
	client := &http.Client{Timeout: 35 * time.Second}

	req, err := http.NewRequestWithContext(ctx, "GET", urlStr, nil)
	if err != nil {
		return fmt.Errorf("getUpdates build req: %w", err)
	}

	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("getUpdates http: %w", err)
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("getUpdates HTTP %d: %s", resp.StatusCode, string(body))
	}

	var result getUpdatesResp
	if err := json.Unmarshal(body, &result); err != nil {
		return fmt.Errorf("getUpdates parse: %w", err)
	}
	if !result.OK {
		return fmt.Errorf("getUpdates api error")
	}

	for i := range result.Result {
		upd := &result.Result[i]
		if upd.UpdateID > b.offset {
			b.offset = upd.UpdateID
		}
		if upd.Message != nil {
			b.handleMessage(ctx, upd.Message)
		}
		if upd.CallbackQuery != nil {
			b.handleCallback(ctx, upd.CallbackQuery)
		}
	}
	return nil
}

func (b *Bot) handleMessage(ctx context.Context, msg *tgMessage) {
	if strconv.FormatInt(msg.Chat.ID, 10) != b.AuthChatID {
		return // silent drop — don't reveal bot existence to unknown chats
	}
	if !strings.HasPrefix(msg.Text, "/") {
		return // ignore non-commands
	}
	cmd, args := splitCommand(msg.Text)
	handler, ok := b.Commands["/"+cmd]
	if !ok {
		_ = b.Telegram.Send(fmt.Sprintf("unknown command: /%s. try /help", cmd))
		return
	}
	reply := handler(ctx, args)
	if reply != "" {
		_ = b.Telegram.Send(reply)
	}
}

func (b *Bot) handleCallback(ctx context.Context, cq *tgCallbackQuery) {
	if strconv.FormatInt(cq.From.ID, 10) != b.AuthChatID {
		// answer with empty text to dismiss the loading spinner
		b.answerCallbackQuery(cq.ID, "")
		return
	}
	handler, ok := b.Callbacks[cq.Data]
	if !ok {
		b.answerCallbackQuery(cq.ID, "unknown action")
		return
	}
	reply, edit := handler(ctx, cq.Data)
	b.answerCallbackQuery(cq.ID, reply)
	if edit && cq.Message != nil {
		b.editMessageReplyMarkup(cq.Message.Chat.ID, cq.Message.MessageID)
	}
}

// answerCallbackQuery sends answerCallbackQuery to dismiss the loading spinner.
func (b *Bot) answerCallbackQuery(callbackQueryID, text string) {
	payload := map[string]string{
		"callback_query_id": callbackQueryID,
		"text":              text,
	}
	body, _ := json.Marshal(payload)
	urlStr := fmt.Sprintf("%s/bot%s/answerCallbackQuery", b.Telegram.BaseURL, b.Telegram.Token)
	req, err := http.NewRequest("POST", urlStr, bytes.NewReader(body))
	if err != nil {
		return
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := b.Telegram.HTTP.Do(req)
	if err != nil {
		return
	}
	defer resp.Body.Close()
	_, _ = io.ReadAll(resp.Body)
}

// editMessageReplyMarkup clears inline buttons from a message to prevent double-tapping.
func (b *Bot) editMessageReplyMarkup(chatID, msgID int64) {
	payload := map[string]interface{}{
		"chat_id":    chatID,
		"message_id": msgID,
		"reply_markup": map[string]interface{}{
			"inline_keyboard": [][]interface{}{},
		},
	}
	body, _ := json.Marshal(payload)
	urlStr := fmt.Sprintf("%s/bot%s/editMessageReplyMarkup", b.Telegram.BaseURL, b.Telegram.Token)
	req, err := http.NewRequest("POST", urlStr, bytes.NewReader(body))
	if err != nil {
		return
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := b.Telegram.HTTP.Do(req)
	if err != nil {
		return
	}
	defer resp.Body.Close()
	_, _ = io.ReadAll(resp.Body)
}

// splitCommand splits "/cmd args" into ("cmd", "args").
// The leading slash is stripped from cmd. If no args, args is "".
func splitCommand(text string) (cmd, args string) {
	text = strings.TrimSpace(text)
	// strip leading slash
	if strings.HasPrefix(text, "/") {
		text = text[1:]
	}
	// strip @botname suffix if present (e.g. /start@mybot)
	parts := strings.SplitN(text, " ", 2)
	cmd = strings.SplitN(parts[0], "@", 2)[0]
	if len(parts) == 2 {
		args = strings.TrimSpace(parts[1])
	}
	return
}

// Telegram API types

type tgUpdate struct {
	UpdateID      int64            `json:"update_id"`
	Message       *tgMessage       `json:"message,omitempty"`
	CallbackQuery *tgCallbackQuery `json:"callback_query,omitempty"`
}

type tgMessage struct {
	MessageID int64  `json:"message_id"`
	Chat      tgChat `json:"chat"`
	From      tgUser `json:"from"`
	Text      string `json:"text"`
}

type tgChat struct {
	ID int64 `json:"id"`
}
type tgUser struct {
	ID int64 `json:"id"`
}

type tgCallbackQuery struct {
	ID      string     `json:"id"`
	From    tgUser     `json:"from"`
	Message *tgMessage `json:"message,omitempty"`
	Data    string     `json:"data"`
}

type getUpdatesResp struct {
	OK     bool       `json:"ok"`
	Result []tgUpdate `json:"result"`
}
