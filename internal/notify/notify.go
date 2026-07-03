// Package notify defines channel-neutral outbound notifications. Alerting,
// password resets and future push delivery render their own messages and call a
// Channel; channel implementations only handle transport.
package notify

import (
	"context"
	"log"
	"sync"
)

// Message is channel-neutral. Subject doubles as a push title, Body as plain
// text, and HTML is optional email-only content.
type Message struct {
	Subject string
	Body    string
	HTML    string
}

// Recipient carries per-channel addressing.
type Recipient struct {
	Email string
}

// Channel sends a rendered notification to a recipient.
type Channel interface {
	Send(ctx context.Context, to Recipient, msg Message) error
}

// Delivery records one fake send.
type Delivery struct {
	To      Recipient
	Message Message
}

// Fake is an in-memory Channel for tests.
type Fake struct {
	mu         sync.Mutex
	deliveries []Delivery
	err        error
}

// NewFake returns a fake Channel.
func NewFake() *Fake {
	return &Fake{}
}

// SetError makes Send return err without recording a delivery.
func (f *Fake) SetError(err error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.err = err
}

// Send records a delivery unless a test error is configured.
func (f *Fake) Send(ctx context.Context, to Recipient, msg Message) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.err != nil {
		return f.err
	}
	f.deliveries = append(f.deliveries, Delivery{To: to, Message: msg})
	return nil
}

// Deliveries returns a snapshot of recorded sends.
func (f *Fake) Deliveries() []Delivery {
	f.mu.Lock()
	defer f.mu.Unlock()
	out := make([]Delivery, len(f.deliveries))
	copy(out, f.deliveries)
	return out
}

// LogChannel logs notifications instead of delivering them. It is the local
// default when SMTP is not configured.
type LogChannel struct {
	Logger *log.Logger
}

// Send logs a notification summary.
func (c LogChannel) Send(ctx context.Context, to Recipient, msg Message) error {
	logger := c.Logger
	if logger == nil {
		logger = log.Default()
	}
	logger.Printf("notification to=%q subject=%q body=%q", to.Email, msg.Subject, msg.Body)
	return nil
}

var (
	_ Channel = (*Fake)(nil)
	_ Channel = LogChannel{}
)
