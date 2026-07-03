package smtpchannel

import (
	"bytes"
	"context"
	"strings"
	"testing"

	"github.com/jackiabishop/mileminder/internal/notify"
)

func TestConfigFromEnv(t *testing.T) {
	t.Setenv("MILEMINDER_SMTP_HOST", "smtp.example.com")
	t.Setenv("MILEMINDER_SMTP_PORT", "2525")
	t.Setenv("MILEMINDER_SMTP_USER", "user")
	t.Setenv("MILEMINDER_SMTP_PASS", "pass")
	t.Setenv("MILEMINDER_SMTP_FROM", "MileMinder <alerts@example.com>")

	cfg, ok, err := ConfigFromEnv()
	if err != nil {
		t.Fatalf("ConfigFromEnv: %v", err)
	}
	if !ok {
		t.Fatal("ConfigFromEnv configured=false, want true")
	}
	if cfg.Host != "smtp.example.com" || cfg.Port != 2525 || cfg.Username != "user" || cfg.Password != "pass" {
		t.Fatalf("config mismatch: %+v", cfg)
	}
}

func TestConfigFromEnvUnset(t *testing.T) {
	cfg, ok, err := ConfigFromEnv()
	if err != nil {
		t.Fatalf("ConfigFromEnv: %v", err)
	}
	if ok || cfg != (Config{}) {
		t.Fatalf("ConfigFromEnv = (%+v, %v), want zero false", cfg, ok)
	}
}

func TestBuildMessagePlain(t *testing.T) {
	raw, err := BuildMessage("alerts@example.com", "a@example.com", notify.Message{Subject: "Mileage alert", Body: "Line 1\nLine 2"})
	if err != nil {
		t.Fatalf("BuildMessage: %v", err)
	}
	for _, want := range []string{
		"From: alerts@example.com\r\n",
		"To: a@example.com\r\n",
		"Subject: Mileage alert\r\n",
		"Content-Type: text/plain",
		"Line 1\r\nLine 2",
	} {
		if !bytes.Contains(raw, []byte(want)) {
			t.Fatalf("message missing %q:\n%s", want, raw)
		}
	}
}

func TestBuildMessageHTML(t *testing.T) {
	raw, err := BuildMessage("alerts@example.com", "a@example.com", notify.Message{
		Subject: "Mileage alert",
		Body:    "Plain",
		HTML:    "<p>HTML</p>",
	})
	if err != nil {
		t.Fatalf("BuildMessage: %v", err)
	}
	s := string(raw)
	if !strings.Contains(s, "multipart/alternative") || !strings.Contains(s, "Plain") || !strings.Contains(s, "<p>HTML</p>") {
		t.Fatalf("multipart message missing content:\n%s", s)
	}
}

func TestSendUsesInjectedSender(t *testing.T) {
	ch, err := New(Config{Host: "smtp.example.com", Port: 587, From: "alerts@example.com"})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	var gotTo string
	var gotRaw []byte
	ch.SetSender(func(ctx context.Context, cfg Config, to string, raw []byte) error {
		gotTo = to
		gotRaw = raw
		return nil
	})

	if err := ch.Send(context.Background(), notify.Recipient{Email: "a@example.com"}, notify.Message{Subject: "Hi", Body: "Body"}); err != nil {
		t.Fatalf("Send: %v", err)
	}
	if gotTo != "a@example.com" || !bytes.Contains(gotRaw, []byte("Subject: Hi")) {
		t.Fatalf("sender got to=%q raw=%q", gotTo, gotRaw)
	}
}
