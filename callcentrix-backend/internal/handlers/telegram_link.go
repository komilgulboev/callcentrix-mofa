package handlers

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/base32"
	"fmt"
	"log"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"callcentrix/internal/telegram"
)

const telegramLinkCodeTTL = 15 * time.Minute

// checkTelegramChatIDAvailable rejects assigning a chat id to a user when
// it's already saved against a different one. Before this, an admin typing a
// numeric chat id straight into the user's create/edit form (see Users.jsx)
// had no way to verify it actually belonged to that person — pasting the
// same (often a shared group's) chat id onto multiple users meant every one
// of them received every other's Telegram notifications. Empty ids are
// exempt: most users legitimately have none yet.
func checkTelegramChatIDAvailable(ctx context.Context, db *sql.DB, chatID string, excludeID int) bool {
	if chatID == "" {
		return true
	}
	var cnt int
	if err := db.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM users WHERE telegram_chat_id = $1 AND id != $2`, chatID, excludeID,
	).Scan(&cnt); err != nil {
		return true // best-effort; don't block saving on a lookup failure
	}
	return cnt == 0
}

// GenerateTelegramLinkCode issues a short-lived one-time code for linking a
// user's own Telegram account — the self-service half of the fix for chat id
// mixups (see checkTelegramChatIDAvailable): rather than an admin typing a
// number they can't verify, the user proves ownership of their own Telegram
// chat by sending this code to the bot themselves (see HandleTelegramMessage).
// SuperAdmin/TenantAdmin only, same gate as the rest of Users management.
func (h *UsersHandler) GenerateTelegramLinkCode(w http.ResponseWriter, r *http.Request) {
	id, _ := strconv.Atoi(chi.URLParam(r, "id"))

	var exists int
	if err := h.DB.QueryRowContext(r.Context(), `SELECT COUNT(*) FROM users WHERE id=$1`, id).Scan(&exists); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if exists == 0 {
		writeError(w, http.StatusNotFound, "not found")
		return
	}

	code, err := randomLinkCode()
	if err != nil {
		writeError(w, http.StatusInternalServerError, "code generation failed")
		return
	}
	expiresAt := time.Now().Add(telegramLinkCodeTTL)

	// One live code per user — a fresh request supersedes whatever code (if
	// any) was issued before, rather than piling up rows nothing ever cleans.
	if _, err := h.DB.ExecContext(r.Context(), `DELETE FROM telegram_link_codes WHERE user_id=$1`, id); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if _, err := h.DB.ExecContext(r.Context(),
		`INSERT INTO telegram_link_codes (code, user_id, expires_at) VALUES ($1,$2,$3)`,
		code, id, expiresAt); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	var botToken string
	_ = h.DB.QueryRowContext(r.Context(), `SELECT bot_token FROM telegram_settings WHERE id=1`).Scan(&botToken)
	botUsername := ""
	if botToken != "" {
		botUsername, _ = telegram.GetMe(botToken)
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"code":        code,
		"botUsername": botUsername,
		"expiresAt":   expiresAt.Format(time.RFC3339),
	})
}

// randomLinkCode returns an 8-character uppercase base32 code — short enough
// to type by hand, but also safe to use verbatim as a Telegram deep-link
// `start` parameter (which only allows [A-Za-z0-9_-]).
func randomLinkCode() (string, error) {
	buf := make([]byte, 5)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}
	return strings.ToUpper(base32.StdEncoding.WithPadding(base32.NoPadding).EncodeToString(buf)), nil
}

// HandleTelegramMessage processes a plain (non-button) message arriving at
// the bot — today just "/start <code>", the second half of the linking flow
// GenerateTelegramLinkCode starts (a t.me/<bot>?start=<code> deep link sends
// exactly this automatically the moment the user opens it). Called from
// TasksHandler.RunTelegramBot's poll loop, which owns the single long-lived
// getUpdates connection this backend keeps open with Telegram — a second
// independent poller would just steal updates from the first, so linking
// can't run its own.
func HandleTelegramMessage(db *sql.DB, botToken string, msg *telegram.TgMsg) {
	if msg == nil {
		return
	}
	chatID := strconv.FormatInt(msg.Chat.ID, 10)
	text := strings.TrimSpace(msg.Text)
	if !strings.HasPrefix(text, "/start") {
		return
	}

	code := strings.ToUpper(strings.TrimSpace(strings.TrimPrefix(text, "/start")))
	if code == "" {
		_ = telegram.SendMessage(botToken, chatID,
			"Чтобы привязать аккаунт, откройте ссылку из приложения (или отправьте: /start <код>).", nil)
		return
	}

	var userID int
	var expiresAt time.Time
	err := db.QueryRow(`SELECT user_id, expires_at FROM telegram_link_codes WHERE code=$1`, code).
		Scan(&userID, &expiresAt)
	if err == sql.ErrNoRows || (err == nil && time.Now().After(expiresAt)) {
		_ = telegram.SendMessage(botToken, chatID,
			"Код недействителен или устарел. Запросите новый код в приложении.", nil)
		return
	}
	if err != nil {
		log.Printf("[TelegramLink] load code %q: %v", code, err)
		return
	}

	// This chat id might already sit on a stale/different user's row —
	// exactly the mixup this flow exists to fix — so clear it there first;
	// otherwise the unique link below would just create a second duplicate.
	if _, err := db.Exec(`UPDATE users SET telegram_chat_id='' WHERE telegram_chat_id=$1 AND id != $2`, chatID, userID); err != nil {
		log.Printf("[TelegramLink] clear stale chat id: %v", err)
	}

	if _, err := db.Exec(`UPDATE users SET telegram_chat_id=$1, updated_at=NOW() WHERE id=$2`, chatID, userID); err != nil {
		log.Printf("[TelegramLink] set chat id for user %d: %v", userID, err)
		_ = telegram.SendMessage(botToken, chatID, "Не удалось привязать аккаунт. Попробуйте ещё раз.", nil)
		return
	}
	_, _ = db.Exec(`DELETE FROM telegram_link_codes WHERE code=$1`, code)

	var username string
	_ = db.QueryRow(`SELECT username FROM users WHERE id=$1`, userID).Scan(&username)
	_ = telegram.SendMessage(botToken, chatID,
		fmt.Sprintf("✅ Telegram привязан к аккаунту %s. Теперь уведомления будут приходить сюда.", username), nil)
}
