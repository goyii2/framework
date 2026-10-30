package base

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/goyii2/framework/config"
	"github.com/goyii2/framework/mail"
)

func TestApplicationMailerDefaultsToLog(t *testing.T) {
	logPath := filepath.Join(t.TempDir(), "mail.log")
	app := New(config.Config{
		Log: config.Log{Targets: []config.LogTarget{{Type: "file", Path: logPath}}},
	})
	t.Cleanup(func() { _ = app.Logger().Close() })

	first := app.Mailer()
	if _, ok := first.(*mail.LogMailer); !ok {
		t.Fatalf("unconfigured Mailer() = %T, want *mail.LogMailer", first)
	}
	if app.Mailer() != first {
		t.Error("repeated Mailer() returned a different instance")
	}

	// The log mailer records through the application's log component under
	// the mail category; nothing is sent.
	err := first.Send(context.Background(), mail.NewMessage().
		SetFrom("Ada <ada@example.com>").
		SetTo("grace@example.com").
		SetSubject("Welcome aboard").
		SetTextBody("Hello Grace"))
	if err != nil {
		t.Fatalf("Send: %v", err)
	}

	data, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatalf("reading the log target: %v", err)
	}
	if !strings.Contains(string(data), "[mail]") || !strings.Contains(string(data), "Subject: Welcome aboard") {
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
	app := New(config.Default())

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
		t.Error("SetMailer(nil) must restore lazy initialization from the configuration")
	}
}

// TestApplicationMailerSmtpWithoutHostFallsBackToLog checks the transition
// through the component holder: an smtp section the constructor rejects
// (here: no host) degrades to the log mailer with a warning, instead of a
// mailer that fails on every Send.
func TestApplicationMailerSmtpWithoutHostFallsBackToLog(t *testing.T) {
	app := New(config.Config{Mail: config.Mail{Driver: "smtp"}})

	if _, ok := app.Mailer().(*mail.LogMailer); !ok {
		t.Fatalf("Mailer() through a rejected smtp section is %T, want the log-mailer fallback", app.Mailer())
	}
	err := app.Mailer().Send(context.Background(),
		mail.NewMessage().SetFrom("ada@example.com").SetTo("g@example.com").SetTextBody("hi"))
	if err != nil {
		t.Fatalf("Send through the fallback: %v", err)
	}
}

// TestApplicationMailerConcurrentAccess exercises lazy initialization and
// replacement of the mailer from many goroutines; run with -race.
func TestApplicationMailerConcurrentAccess(t *testing.T) {
	app := New(config.Default())

	var wg sync.WaitGroup
	for i := 0; i < 64; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			m := app.Mailer()
			if i%4 == 0 {
				app.SetMailer(&fakeMailer{})
			}
			_ = m
		}(i)
	}
	wg.Wait()
}
