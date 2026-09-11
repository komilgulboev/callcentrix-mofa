// Package email sends outbound notifications over SMTP — currently just the
// "you've been assigned a ticket" notice (see
// handlers.TicketsHandler.notifyTicketAssigned) — through a single-row relay
// config (see smtp_settings) that SuperAdmin sets once for the whole
// platform, the same pattern as smpp_settings for SMS.
package email

import (
	"crypto/tls"
	"errors"
	"fmt"
	"mime"
	"net/smtp"
	"strings"
)

// Config is the outbound SMTP relay this backend sends through.
type Config struct {
	Host     string
	Port     int
	Username string
	Password string
	From     string // envelope/"From" address; falls back to Username if blank
}

// sanitizeHeader strips CR/LF from a value bound for a raw message header —
// To/Subject can come from user-editable data (a profile email, a ticket
// subject), and an embedded newline there would let it inject extra headers
// or body content into the message.
func sanitizeHeader(s string) string {
	s = strings.ReplaceAll(s, "\r", "")
	return strings.ReplaceAll(s, "\n", "")
}

// Send delivers a plain-text email through cfg's relay. Best-effort by
// design at the call site — a misconfigured or unreachable mail server
// should never block the action (a ticket assignment) that triggered it.
func Send(cfg Config, to, subject, body string) error {
	if cfg.Host == "" {
		return errors.New("smtp is not configured")
	}
	from := cfg.From
	if from == "" {
		from = cfg.Username
	}
	if from == "" {
		return errors.New("smtp sender address is not configured")
	}
	to = sanitizeHeader(to)
	if to == "" {
		return errors.New("recipient address is empty")
	}

	addr := fmt.Sprintf("%s:%d", cfg.Host, cfg.Port)
	msg := buildMessage(from, to, subject, body)

	var auth smtp.Auth
	if cfg.Username != "" {
		auth = smtp.PlainAuth("", cfg.Username, cfg.Password, cfg.Host)
	}

	// Port 465 is implicit TLS (the connection is TLS from the first byte, no
	// STARTTLS negotiation) — smtp.SendMail only speaks STARTTLS (587/25), so
	// that port needs its own TLS-first dial instead.
	if cfg.Port == 465 {
		return sendImplicitTLS(addr, cfg.Host, auth, from, to, msg)
	}
	return smtp.SendMail(addr, auth, from, []string{to}, msg)
}

func sendImplicitTLS(addr, host string, auth smtp.Auth, from, to string, msg []byte) error {
	conn, err := tls.Dial("tcp", addr, &tls.Config{ServerName: host})
	if err != nil {
		return fmt.Errorf("smtp tls dial: %w", err)
	}
	defer conn.Close()

	client, err := smtp.NewClient(conn, host)
	if err != nil {
		return fmt.Errorf("smtp client: %w", err)
	}
	defer client.Close()

	if auth != nil {
		if err := client.Auth(auth); err != nil {
			return fmt.Errorf("smtp auth: %w", err)
		}
	}
	if err := client.Mail(from); err != nil {
		return fmt.Errorf("smtp mail from: %w", err)
	}
	if err := client.Rcpt(to); err != nil {
		return fmt.Errorf("smtp rcpt to: %w", err)
	}
	w, err := client.Data()
	if err != nil {
		return fmt.Errorf("smtp data: %w", err)
	}
	if _, err := w.Write(msg); err != nil {
		return fmt.Errorf("smtp write: %w", err)
	}
	return w.Close()
}

// buildMessage assembles a minimal plain-text RFC 5322 message. Subject is
// MIME (RFC 2047) encoded since ticket subjects routinely carry Cyrillic —
// a raw UTF-8 header renders as mojibake in some mail clients.
func buildMessage(from, to, subject, body string) []byte {
	var b strings.Builder
	b.WriteString("From: " + sanitizeHeader(from) + "\r\n")
	b.WriteString("To: " + sanitizeHeader(to) + "\r\n")
	b.WriteString("Subject: " + mime.QEncoding.Encode("UTF-8", sanitizeHeader(subject)) + "\r\n")
	b.WriteString("MIME-Version: 1.0\r\n")
	b.WriteString("Content-Type: text/plain; charset=UTF-8\r\n")
	b.WriteString("\r\n")
	b.WriteString(body)
	return []byte(b.String())
}
