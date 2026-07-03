package notify

import (
	"bytes"
	"context"
	"errors"
	"log"
	"strings"
	"testing"
)

func TestFakeRecordsDeliveries(t *testing.T) {
	ch := NewFake()
	if err := ch.Send(context.Background(), Recipient{Email: "a@example.com"}, Message{Subject: "Hi", Body: "Body"}); err != nil {
		t.Fatalf("Send: %v", err)
	}
	got := ch.Deliveries()
	if len(got) != 1 {
		t.Fatalf("deliveries = %d, want 1", len(got))
	}
	if got[0].To.Email != "a@example.com" || got[0].Message.Subject != "Hi" {
		t.Fatalf("delivery mismatch: %+v", got[0])
	}
}

func TestFakeErrorDoesNotRecord(t *testing.T) {
	ch := NewFake()
	want := errors.New("send failed")
	ch.SetError(want)
	if err := ch.Send(context.Background(), Recipient{Email: "a@example.com"}, Message{}); !errors.Is(err, want) {
		t.Fatalf("Send error = %v, want %v", err, want)
	}
	if got := ch.Deliveries(); len(got) != 0 {
		t.Fatalf("deliveries = %d, want 0", len(got))
	}
}

func TestLogChannel(t *testing.T) {
	var buf bytes.Buffer
	ch := LogChannel{Logger: log.New(&buf, "", 0)}
	if err := ch.Send(context.Background(), Recipient{Email: "a@example.com"}, Message{Subject: "Alert", Body: "Body"}); err != nil {
		t.Fatalf("Send: %v", err)
	}
	out := buf.String()
	if !strings.Contains(out, "a@example.com") || !strings.Contains(out, "Alert") {
		t.Fatalf("log output missing delivery fields: %q", out)
	}
}
