// Package smpp sends one-off SMS messages (registration codes) through an
// SMPP gateway. Volume is low — only registration events — so a short-lived
// bind per message is used instead of a persistent connection.
package smpp

import (
	"errors"
	"fmt"
	"time"

	"github.com/linxGnu/gosmpp"
	"github.com/linxGnu/gosmpp/data"
	"github.com/linxGnu/gosmpp/pdu"
)

// Config holds SMPP gateway credentials, loaded from the smpp_settings table.
type Config struct {
	Host     string
	Port     int
	SystemID string
	Password string
	SenderID string
}

// SendSMS binds a transmitter session, submits one message, and closes it.
func SendSMS(cfg Config, toPhone, message string) error {
	if cfg.Host == "" || cfg.SystemID == "" {
		return errors.New("smpp is not configured")
	}

	auth := gosmpp.Auth{
		SMSC:     fmt.Sprintf("%s:%d", cfg.Host, cfg.Port),
		SystemID: cfg.SystemID,
		Password: cfg.Password,
	}

	session, err := gosmpp.NewSession(
		gosmpp.TXConnector(gosmpp.NonTLSDialer, auth),
		gosmpp.Settings{ReadTimeout: 10 * time.Second},
		0, // one-shot send — no auto-rebind
	)
	if err != nil {
		return fmt.Errorf("smpp bind: %w", err)
	}
	defer session.Close()

	srcAddr, err := pdu.NewAddressWithAddr(cfg.SenderID)
	if err != nil {
		return fmt.Errorf("smpp source address: %w", err)
	}
	dstAddr, err := pdu.NewAddressWithAddr(toPhone)
	if err != nil {
		return fmt.Errorf("smpp destination address: %w", err)
	}
	sm, err := pdu.NewShortMessageWithEncoding(message, data.UCS2)
	if err != nil {
		return fmt.Errorf("smpp message: %w", err)
	}

	submit := pdu.NewSubmitSM().(*pdu.SubmitSM)
	submit.SourceAddr = srcAddr
	submit.DestAddr = dstAddr
	submit.Message = sm

	if err := session.Transmitter().Submit(submit); err != nil {
		return fmt.Errorf("smpp submit: %w", err)
	}

	// Give the session's async writer a moment to flush before the deferred
	// Close() tears down the connection.
	time.Sleep(500 * time.Millisecond)
	return nil
}
