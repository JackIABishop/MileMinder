// Package smtpchannel implements notify.Channel over SMTP.
package smtpchannel

import (
	"bytes"
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"io"
	"mime/multipart"
	"net"
	"net/mail"
	"net/smtp"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/jackiabishop/mileminder/internal/notify"
)

// Config is the SMTP transport configuration.
type Config struct {
	Host     string
	Port     int
	Username string
	Password string
	From     string
}

// ConfigFromEnv reads MILEMINDER_SMTP_* configuration. The bool is false when
// SMTP is not configured and callers should fall back to a log channel.
func ConfigFromEnv() (Config, bool, error) {
	host := strings.TrimSpace(os.Getenv("MILEMINDER_SMTP_HOST"))
	if host == "" {
		return Config{}, false, nil
	}
	port := 587
	if raw := strings.TrimSpace(os.Getenv("MILEMINDER_SMTP_PORT")); raw != "" {
		n, err := strconv.Atoi(raw)
		if err != nil || n <= 0 || n > 65535 {
			return Config{}, true, fmt.Errorf("invalid MILEMINDER_SMTP_PORT %q", raw)
		}
		port = n
	}
	cfg := Config{
		Host:     host,
		Port:     port,
		Username: os.Getenv("MILEMINDER_SMTP_USER"),
		Password: os.Getenv("MILEMINDER_SMTP_PASS"),
		From:     strings.TrimSpace(os.Getenv("MILEMINDER_SMTP_FROM")),
	}
	if cfg.From == "" {
		return Config{}, true, errors.New("MILEMINDER_SMTP_FROM is required when SMTP is configured")
	}
	if _, err := mail.ParseAddress(cfg.From); err != nil {
		return Config{}, true, fmt.Errorf("invalid MILEMINDER_SMTP_FROM: %w", err)
	}
	if (cfg.Username == "") != (cfg.Password == "") {
		return Config{}, true, errors.New("MILEMINDER_SMTP_USER and MILEMINDER_SMTP_PASS must be set together")
	}
	return cfg, true, nil
}

// Channel sends email through SMTP.
type Channel struct {
	cfg  Config
	send func(context.Context, Config, string, []byte) error
}

// New validates cfg and returns an SMTP Channel.
func New(cfg Config) (*Channel, error) {
	if strings.TrimSpace(cfg.Host) == "" {
		return nil, errors.New("SMTP host is required")
	}
	if cfg.Port <= 0 || cfg.Port > 65535 {
		return nil, fmt.Errorf("invalid SMTP port %d", cfg.Port)
	}
	if strings.TrimSpace(cfg.From) == "" {
		return nil, errors.New("SMTP from address is required")
	}
	if _, err := mail.ParseAddress(cfg.From); err != nil {
		return nil, fmt.Errorf("invalid SMTP from address: %w", err)
	}
	return &Channel{cfg: cfg, send: sendSMTP}, nil
}

// SetSender replaces the network sender. It is intended for tests.
func (c *Channel) SetSender(sender func(context.Context, Config, string, []byte) error) {
	c.send = sender
}

// Send delivers msg to an email recipient.
func (c *Channel) Send(ctx context.Context, to notify.Recipient, msg notify.Message) error {
	if strings.TrimSpace(to.Email) == "" {
		return errors.New("recipient email is required")
	}
	if _, err := mail.ParseAddress(to.Email); err != nil {
		return fmt.Errorf("invalid recipient email: %w", err)
	}
	raw, err := BuildMessage(c.cfg.From, to.Email, msg)
	if err != nil {
		return err
	}
	return c.send(ctx, c.cfg, to.Email, raw)
}

// BuildMessage constructs an RFC 5322 message. It is exported for tests.
func BuildMessage(from, to string, msg notify.Message) ([]byte, error) {
	var buf bytes.Buffer
	writeHeader := func(k, v string) {
		fmt.Fprintf(&buf, "%s: %s\r\n", k, v)
	}
	writeHeader("From", from)
	writeHeader("To", to)
	writeHeader("Subject", msg.Subject)
	writeHeader("MIME-Version", "1.0")

	if msg.HTML == "" {
		writeHeader("Content-Type", `text/plain; charset="utf-8"`)
		writeHeader("Content-Transfer-Encoding", "8bit")
		buf.WriteString("\r\n")
		buf.WriteString(normalizeCRLF(msg.Body))
		return buf.Bytes(), nil
	}

	var body bytes.Buffer
	mw := multipart.NewWriter(&body)
	textPart, err := mw.CreatePart(map[string][]string{
		"Content-Type":              {`text/plain; charset="utf-8"`},
		"Content-Transfer-Encoding": {"8bit"},
	})
	if err != nil {
		return nil, err
	}
	if _, err := io.WriteString(textPart, normalizeCRLF(msg.Body)); err != nil {
		return nil, err
	}
	htmlPart, err := mw.CreatePart(map[string][]string{
		"Content-Type":              {`text/html; charset="utf-8"`},
		"Content-Transfer-Encoding": {"8bit"},
	})
	if err != nil {
		return nil, err
	}
	if _, err := io.WriteString(htmlPart, normalizeCRLF(msg.HTML)); err != nil {
		return nil, err
	}
	if err := mw.Close(); err != nil {
		return nil, err
	}

	writeHeader("Content-Type", `multipart/alternative; boundary="`+mw.Boundary()+`"`)
	buf.WriteString("\r\n")
	buf.Write(body.Bytes())
	return buf.Bytes(), nil
}

func normalizeCRLF(s string) string {
	s = strings.ReplaceAll(s, "\r\n", "\n")
	s = strings.ReplaceAll(s, "\r", "\n")
	return strings.ReplaceAll(s, "\n", "\r\n")
}

func sendSMTP(ctx context.Context, cfg Config, to string, raw []byte) error {
	addr := net.JoinHostPort(cfg.Host, strconv.Itoa(cfg.Port))
	dialer := net.Dialer{Timeout: 10 * time.Second}
	conn, err := dialer.DialContext(ctx, "tcp", addr)
	if err != nil {
		return fmt.Errorf("dial SMTP: %w", err)
	}
	defer conn.Close()

	client, err := smtp.NewClient(conn, cfg.Host)
	if err != nil {
		return fmt.Errorf("create SMTP client: %w", err)
	}
	defer client.Close()

	if err := client.Hello("localhost"); err != nil {
		return fmt.Errorf("SMTP hello: %w", err)
	}
	if ok, _ := client.Extension("STARTTLS"); ok {
		tlsCfg := &tls.Config{ServerName: cfg.Host, MinVersion: tls.VersionTLS12}
		if err := client.StartTLS(tlsCfg); err != nil {
			return fmt.Errorf("SMTP STARTTLS: %w", err)
		}
	} else if cfg.Username != "" {
		return errors.New("SMTP server does not support STARTTLS; refusing to authenticate")
	}

	if cfg.Username != "" {
		auth := smtp.PlainAuth("", cfg.Username, cfg.Password, cfg.Host)
		if err := client.Auth(auth); err != nil {
			return fmt.Errorf("SMTP auth: %w", err)
		}
	}
	if err := client.Mail(cfg.From); err != nil {
		return fmt.Errorf("SMTP MAIL FROM: %w", err)
	}
	if err := client.Rcpt(to); err != nil {
		return fmt.Errorf("SMTP RCPT TO: %w", err)
	}
	w, err := client.Data()
	if err != nil {
		return fmt.Errorf("SMTP DATA: %w", err)
	}
	if _, err := w.Write(raw); err != nil {
		w.Close()
		return fmt.Errorf("write SMTP data: %w", err)
	}
	if err := w.Close(); err != nil {
		return fmt.Errorf("close SMTP data: %w", err)
	}
	if err := client.Quit(); err != nil {
		return fmt.Errorf("SMTP quit: %w", err)
	}
	return nil
}

var _ notify.Channel = (*Channel)(nil)
