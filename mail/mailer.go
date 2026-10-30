package mail

import (
	"context"
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
