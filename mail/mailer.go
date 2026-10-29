package mail

import (
	"context"
	"fmt"
	"strings"

	"github.com/goyii2/framework/config"
)

// Mailer sends messages. Implementations range from the development
// LogMailer to real SMTP delivery; tests install a fake to capture messages
// without side effects.
type Mailer interface {
	// Send delivers msg. It reports the message's builder error (invalid
	// address, unreadable attachment file) before doing any work.
	Send(ctx context.Context, msg *Message) error
}

// LogSink is the logging contract mailers use for delivery-independent
// output; the framework log component satisfies it directly.
type LogSink interface {
	Info(msg string, args ...any)
}

// NewFromConfig builds the mailer named by cfg.Driver: "log" (also the
// empty default) yields the LogMailer, "smtp" the SmtpMailer. cfg.From
// becomes the default sender for messages that set none. A configuration
// that cannot be used (unknown driver, smtp without a host, unparseable
// from) yields a mailer whose Send fails with a configuration error rather
// than a nil component, so the mistake surfaces at the first send instead
// of breaking application assembly.
func NewFromConfig(cfg config.Mail, sink LogSink) Mailer {
	if cfg.From != "" {
		from, err := parseAddress(cfg.From)
		if err != nil {
			return failingMailer{fmt.Errorf("mail: invalid mail.from %q: %w", cfg.From, err)}
		}
		return withDefaultFrom{from: from, inner: driverMailer(cfg, sink)}
	}
	return driverMailer(cfg, sink)
}

// driverMailer resolves the driver name to its mailer.
func driverMailer(cfg config.Mail, sink LogSink) Mailer {
	switch strings.ToLower(strings.TrimSpace(cfg.Driver)) {
	case "", "log":
		return NewLogMailer(sink)
	case "smtp":
		m, err := NewSmtpMailer(SMTPConfig{
			Host:       cfg.SMTP.Host,
			Port:       cfg.SMTP.Port,
			Username:   cfg.SMTP.Username,
			Password:   cfg.SMTP.Password,
			Encryption: cfg.SMTP.Encryption,
		})
		if err != nil {
			return failingMailer{err}
		}
		return m
	default:
		return failingMailer{fmt.Errorf("mail: unknown driver %q (want log or smtp)", cfg.Driver)}
	}
}

// failingMailer reports one fixed configuration error from every Send.
type failingMailer struct{ err error }

func (f failingMailer) Send(context.Context, *Message) error { return f.err }

// withDefaultFrom fills in the configured sender when a message sets none.
type withDefaultFrom struct {
	inner Mailer
	from  Address
}

func (w withDefaultFrom) Send(ctx context.Context, msg *Message) error {
	if msg != nil && msg.from.Email == "" {
		filled := *msg
		filled.from = w.from
		msg = &filled
	}
	return w.inner.Send(ctx, msg)
}
