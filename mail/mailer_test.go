package mail

import (
	"context"
	"log/slog"
	"strings"
	"sync"
	"testing"

	"github.com/goyii2/framework/log"
)

func TestNewDefaultsToLogMailer(t *testing.T) {
	sink := &recordingSink{}
	for _, driver := range []string{"", "log", "LOG"} {
		m, err := New(Config{Driver: driver}, WithLogger(sink))
		if err != nil {
			t.Fatalf("driver %q: %v", driver, err)
		}
		if _, ok := m.(*LogMailer); !ok {
			t.Errorf("driver %q produced %T, want *LogMailer", driver, m)
		}
	}
}

func TestNewSmtpDriver(t *testing.T) {
	m, err := New(Config{
		Driver: "smtp",
		SMTP:   SMTPConfig{Host: "mail.example.com", Port: 25, Encryption: "none"},
	}, nil)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	if _, ok := m.(*SmtpMailer); !ok {
		t.Errorf("smtp driver produced %T, want *SmtpMailer", m)
	}
}

func TestNewRejectsUnusableConfig(t *testing.T) {
	cases := []struct {
		name string
		cfg  Config
		want string
	}{
		{"unknown driver", Config{Driver: "pirate"}, "unknown driver"},
		{"smtp without host", Config{Driver: "smtp"}, "host is required"},
		{"bad encryption", Config{Driver: "smtp", SMTP: SMTPConfig{Host: "m.example.com", Encryption: "x"}}, "encryption"},
		{"invalid default from", Config{Driver: "log", From: "not an address"}, "mail.from"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := New(tc.cfg)
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("New error = %v, want it to contain %q", err, tc.want)
			}
		})
	}
}

func TestNewDefaultFromApplied(t *testing.T) {
	sink := &recordingSink{}
	m, err := New(Config{
		Driver: "log",
		From:   "Robot <noreply@example.com>",
	}, WithLogger(sink))
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	// A message without its own sender gets the configured one...
	msg := NewMessage().SetTo("grace@example.com").SetTextBody("hi")
	if err := m.Send(context.Background(), msg); err != nil {
		t.Fatalf("Send: %v", err)
	}
	if entries := sink.all(); len(entries) != 1 || !strings.Contains(entries[0], "From: Robot <noreply@example.com>") {
		t.Fatalf("configured default sender not applied; logged:\n%s", strings.Join(entries, "\n"))
	}

	// ...and the message's own sender still wins. The original message must
	// not be mutated by the default.
	msg = NewMessage().SetFrom("ada@example.com").SetTextBody("hi")
	if err := m.Send(context.Background(), msg); err != nil {
		t.Fatalf("Send: %v", err)
	}
	if entries := sink.all(); len(entries) != 2 || !strings.Contains(entries[1], "From: ada@example.com") {
		t.Fatalf("message sender must win over the default; logged:\n%s", strings.Join(entries, "\n"))
	}
	if msg.From().Email != "ada@example.com" {
		t.Errorf("the caller's message was mutated: From = %+v", msg.From())
	}
}

// fakeMailer stands in for the Mailer contract in consumer tests; this is
// the injection point acceptance asks for: any code holding a Mailer can be
// tested without a transport.
type fakeMailer struct {
	sent []*Message
}

func (f *fakeMailer) Send(_ context.Context, msg *Message) error {
	f.sent = append(f.sent, msg)
	return nil
}

func TestMailerContractAcceptsFake(t *testing.T) {
	var m Mailer = &fakeMailer{}
	msg := NewMessage().SetFrom("ada@example.com").SetTo("grace@example.com").SetTextBody("hi")
	if err := m.Send(context.Background(), msg); err != nil {
		t.Fatalf("Send through fake: %v", err)
	}
	if got := m.(*fakeMailer).sent; len(got) != 1 || got[0] != msg {
		t.Errorf("fake captured %+v, want the exact message", got)
	}
}

// captureTarget collects the records a framework logger dispatches.
type captureTarget struct {
	mu      sync.Mutex
	records []log.Record
}

func (c *captureTarget) Name() string { return "capture" }

func (c *captureTarget) Write(rec log.Record) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.records = append(c.records, rec)
	return nil
}

func (c *captureTarget) Close() error { return nil }

func (c *captureTarget) all() []log.Record {
	c.mu.Lock()
	defer c.mu.Unlock()
	return append([]log.Record(nil), c.records...)
}

// TestNewWithFrameworkLogger wires the LogMailer through the framework log
// component so messages land in the application's targets.
func TestNewWithFrameworkLogger(t *testing.T) {
	target := &captureTarget{}
	m, err := New(Config{Driver: "log"}, WithLogger(log.NewWithTargets(slog.LevelInfo, target)))
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	sendErr := m.Send(context.Background(), NewMessage().
		SetFrom("Ada <ada@example.com>").SetTo("grace@example.com").
		SetSubject("through the framework log").SetTextBody("body"))
	if sendErr != nil {
		t.Fatalf("Send: %v", sendErr)
	}

	records := target.all()
	if len(records) != 1 {
		t.Fatalf("framework logger received %d records, want 1", len(records))
	}
	if rec := records[0]; !strings.Contains(rec.Message, "Subject: through the framework log") {
		t.Errorf("record message missing the rendered mail; got:\n%s", rec.Message)
	}
}
