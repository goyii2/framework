package base

import (
	"context"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/goyii2/framework/log"
	"github.com/goyii2/framework/mail"
)

// TestApplicationMailerDefaultsToLog verifies the fallback mailer records
// through the log component under the mail category — following a logger
// installed after construction.
func TestApplicationMailerDefaultsToLog(t *testing.T) {
	logPath := filepath.Join(t.TempDir(), "mail.log")
	logger, err := log.New(log.Config{
		Targets: []log.TargetConfig{{Type: "file", Path: logPath}},
	})
	if err != nil {
		t.Fatalf("log.New: %v", err)
	}
	app := New()
	app.SetLog(logger)
	t.Cleanup(func() { _ = app.Logger().Close() })

	first := app.Mailer()
	if _, ok := first.(*mail.LogMailer); !ok {
		t.Fatalf("fallback Mailer() = %T, want *mail.LogMailer", first)
	}
	if app.Mailer() != first {
		t.Error("repeated Mailer() returned a different instance")
	}

	// The log mailer records through the application's log component under
	// the mail category; nothing is sent.
	err = first.Send(context.Background(), mail.NewMessage().
		SetFrom("Ada <ada@example.com>").
		SetTo("grace@example.com").
		SetSubject("Welcome aboard").
		SetTextBody("Hello Grace"))
	if err != nil {
		t.Fatalf("Send: %v", err)
	}

	data, err := readFile(logPath)
	if err != nil {
		t.Fatalf("reading the log target: %v", err)
	}
	if !strings.Contains(data, "[mail]") || !strings.Contains(data, "Subject: Welcome aboard") {
		t.Errorf("logged mail record missing category or content; got:\n%s", data)
	}
}

// fakeMailer captures messages sent through the application's mailer slot.
type fakeMailer struct {
	mu   sync.Mutex
	sent []*mail.Message
}

func (f *fakeMailer) Send(_ context.Context, msg *mail.Message) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.sent = append(f.sent, msg)
	return nil
}

func (f *fakeMailer) count() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return len(f.sent)
}

// TestApplicationSetMailerFake verifies the injection point: unit tests
// install a fake and assert on captured messages without any transport.
func TestApplicationSetMailerFake(t *testing.T) {
	app := New()

	fake := &fakeMailer{}
	app.SetMailer(fake)
	msg := mail.NewMessage().SetFrom("ada@example.com").SetTo("grace@example.com").SetTextBody("hi")
	if err := app.Mailer().Send(context.Background(), msg); err != nil {
		t.Fatalf("Send through the fake: %v", err)
	}
	if fake.count() != 1 {
		t.Fatalf("fake captured %d messages, want 1", fake.count())
	}

	app.SetMailer(nil)
	if _, ok := app.Mailer().(*mail.LogMailer); !ok {
		t.Error("SetMailer(nil) must restore the log-mailer fallback")
	}
}

// TestApplicationMailerReplacedByConfiguredMailer verifies a mailer the
// project's configuration selects installs cleanly.
func TestApplicationMailerReplacedByConfiguredMailer(t *testing.T) {
	app := New()

	m, err := mail.New(mail.Config{
		Driver: "smtp",
		From:   "Robot <noreply@example.com>",
		SMTP:   mail.SMTPConfig{Host: "mail.example.com", Port: 2525, Encryption: "none"},
	}, mail.WithLogger(app.Logger().Category("mail")))
	if err != nil {
		t.Fatalf("mail.New(smtp): %v", err)
	}
	app.SetMailer(m)
	if app.Mailer() != m {
		t.Error("SetMailer must install the constructed mailer")
	}
}
