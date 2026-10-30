package notifier

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/goyii2/framework/examples/basic/config"
	"github.com/goyii2/framework/mail"
)

// captureMailer records the one message Send hands it.
type captureMailer struct {
	msg *mail.Message
	err error
}

func (c *captureMailer) Send(_ context.Context, msg *mail.Message) error {
	c.msg = msg
	return c.err
}

// validEmail is the section shape New accepts: both credentials present.
func validEmail() config.EmailConfig {
	return config.EmailConfig{
		APIKey: "test-key",
		Sender: "Notifications <notifications@goyii.example>",
	}
}

func TestNewRejectsIncompleteSection(t *testing.T) {
	cases := []struct {
		name string
		cfg  config.EmailConfig
		want string
	}{
		{"empty api key", config.EmailConfig{Sender: "a@example.com"}, "email.apiKey"},
		{"blank api key", config.EmailConfig{APIKey: "  ", Sender: "a@example.com"}, "email.apiKey"},
		{"empty sender", config.EmailConfig{APIKey: "k"}, "email.sender"},
		{"blank sender", config.EmailConfig{APIKey: "k", Sender: " \t "}, "email.sender"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := New(tc.cfg, &captureMailer{})
			if err == nil {
				t.Fatal("New succeeded, want construction error")
			}
			if !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("New error = %q, want it to mention %q", err.Error(), tc.want)
			}
		})
	}
}

func TestSenderComesFromSection(t *testing.T) {
	n, err := New(validEmail(), &captureMailer{})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	if got, want := n.Sender(), "Notifications <notifications@goyii.example>"; got != want {
		t.Fatalf("Sender() = %q, want %q", got, want)
	}
}

func TestSendComposesFromConfiguredSender(t *testing.T) {
	capture := &captureMailer{}
	n, err := New(validEmail(), capture)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	if err := n.Send(context.Background(), "guest@example.com", "Hello", "First line."); err != nil {
		t.Fatalf("Send: %v", err)
	}
	msg := capture.msg
	if msg == nil {
		t.Fatal("no message reached the mailer")
	}
	if got, want := msg.From().String(), "Notifications <notifications@goyii.example>"; got != want {
		t.Fatalf("From = %q, want %q", got, want)
	}
	if got, want := msg.To()[0].String(), "guest@example.com"; got != want {
		t.Fatalf("To = %q, want %q", got, want)
	}
	if got, want := msg.Subject(), "Hello"; got != want {
		t.Fatalf("Subject = %q, want %q", got, want)
	}
	if got, want := msg.TextBody(), "First line."; got != want {
		t.Fatalf("TextBody = %q, want %q", got, want)
	}
}

func TestSendPropagatesErrors(t *testing.T) {
	want := errors.New("provider unreachable")
	n, err := New(validEmail(), &captureMailer{err: want})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	if got := n.Send(context.Background(), "guest@example.com", "Hello", "Body"); !errors.Is(got, want) {
		t.Fatalf("Send error = %v, want %v", got, want)
	}
}

func TestSendRejectsUnparseableAddress(t *testing.T) {
	capture := &captureMailer{}
	n, err := New(validEmail(), capture)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	if err := n.Send(context.Background(), "not an address at all", "Hello", "Body"); err == nil {
		t.Fatal("Send succeeded with an unparseable recipient, want the compose error")
	}
	if capture.msg != nil {
		t.Fatal("a broken message reached the mailer")
	}
}
