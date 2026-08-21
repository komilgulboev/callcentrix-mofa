// Package telegram talks to the Telegram Bot API: outbound notifications
// (sendMessage/editMessageText) plus long-polling (getUpdates) to receive
// inline-button presses — no webhook, since this backend isn't guaranteed a
// public HTTPS endpoint (see internal/handlers/tasks.go RunTelegramBot).
package telegram

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"time"
)

// httpClient is for regular, fast calls (send/edit/answer). pollClient is
// for GetUpdates, whose long-poll `timeout` param can hold the connection
// open for up to that many seconds — it needs a longer client timeout so
// Go doesn't cut the request off before Telegram responds.
var httpClient = &http.Client{Timeout: 10 * time.Second}
var pollClient = &http.Client{Timeout: 40 * time.Second}

type InlineButton struct {
	Text         string `json:"text"`
	CallbackData string `json:"callback_data"`
}

// InlineKeyboard is Telegram's reply_markup shape for inline buttons —
// one slice per row.
type InlineKeyboard struct {
	InlineKeyboard [][]InlineButton `json:"inline_keyboard"`
}

// SendMessage posts text to chatID via the bot identified by botToken.
// keyboard may be nil for a plain message with no buttons.
func SendMessage(botToken, chatID, text string, keyboard *InlineKeyboard) error {
	if botToken == "" {
		return errors.New("telegram bot token is not configured")
	}
	if chatID == "" {
		return errors.New("recipient has no telegram chat id")
	}

	payload := map[string]any{"chat_id": chatID, "text": text}
	if keyboard != nil {
		payload["reply_markup"] = keyboard
	}
	return post(botToken, "sendMessage", payload)
}

// EditMessageText replaces an already-sent message's text (and, if given, its
// inline keyboard) — used to reflect a status change back onto the original
// task-assignment message after its buttons are used.
func EditMessageText(botToken, chatID string, messageID int, text string, keyboard *InlineKeyboard) error {
	payload := map[string]any{"chat_id": chatID, "message_id": messageID, "text": text}
	if keyboard != nil {
		payload["reply_markup"] = keyboard
	}
	return post(botToken, "editMessageText", payload)
}

// AnswerCallbackQuery acknowledges an inline-button press — required so
// Telegram stops showing the button's loading spinner — and optionally shows
// the caller a small popup with text.
func AnswerCallbackQuery(botToken, callbackQueryID, text string) error {
	payload := map[string]any{"callback_query_id": callbackQueryID}
	if text != "" {
		payload["text"] = text
	}
	return post(botToken, "answerCallbackQuery", payload)
}

func post(botToken, method string, payload any) error {
	body, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("telegram marshal: %w", err)
	}

	apiURL := fmt.Sprintf("https://api.telegram.org/bot%s/%s", botToken, method)
	resp, err := httpClient.Post(apiURL, "application/json", bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("telegram %s request: %w", method, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		// Surface Telegram's own explanation (e.g. "Bad Request: chat not
		// found", "Forbidden: bot was blocked by the user") — a bare status
		// code isn't enough to tell those apart in the logs.
		respBody, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("telegram %s returned status %d: %s", method, resp.StatusCode, respBody)
	}
	return nil
}

// ── Incoming updates (long polling) ─────────────────────────────────────

type Update struct {
	UpdateID      int            `json:"update_id"`
	CallbackQuery *CallbackQuery `json:"callback_query"`
}

type CallbackQuery struct {
	ID      string `json:"id"`
	From    TgUser `json:"from"`
	Data    string `json:"data"`
	Message *TgMsg `json:"message"`
}

type TgUser struct {
	ID int64 `json:"id"`
}

type TgMsg struct {
	MessageID int    `json:"message_id"`
	Chat      TgChat `json:"chat"`
	Text      string `json:"text"`
}

type TgChat struct {
	ID int64 `json:"id"`
}

// GetUpdates long-polls for new updates since offset, waiting up to
// timeoutSec for one to arrive before returning an empty result.
func GetUpdates(botToken string, offset, timeoutSec int) ([]Update, error) {
	if botToken == "" {
		return nil, errors.New("telegram bot token is not configured")
	}

	q := url.Values{}
	q.Set("offset", fmt.Sprintf("%d", offset))
	q.Set("timeout", fmt.Sprintf("%d", timeoutSec))
	q.Set("allowed_updates", `["callback_query"]`)

	apiURL := fmt.Sprintf("https://api.telegram.org/bot%s/getUpdates?%s", botToken, q.Encode())
	resp, err := pollClient.Get(apiURL)
	if err != nil {
		return nil, fmt.Errorf("telegram getUpdates request: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("telegram getUpdates read: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("telegram getUpdates returned status %d: %s", resp.StatusCode, body)
	}

	var parsed struct {
		OK     bool     `json:"ok"`
		Result []Update `json:"result"`
	}
	if err := json.Unmarshal(body, &parsed); err != nil {
		return nil, fmt.Errorf("telegram getUpdates decode: %w", err)
	}
	if !parsed.OK {
		return nil, fmt.Errorf("telegram getUpdates not ok: %s", body)
	}
	return parsed.Result, nil
}
