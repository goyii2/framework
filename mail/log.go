package mail

import (
	"context"
	"errors"
)

// errNilMessage is returned by every mailer asked to send nothing.
var errNilMessage = errors.New("mail: nil message")

// LogMailer is the development mailer: Send renders the complete message
// into the application log and reports success — nothing leaves the
// process. This is the default when the mail configuration names no driver,
// so sending code runs unchanged before any SMTP server exists.
type LogMailer struct {
	sink LogSink
}

// NewLogMailer returns a LogMailer writing through sink; a nil sink drops
// the output while Send still succeeds.
func NewLogMailer(sink LogSink) *LogMailer {
	return &LogMailer{sink: sink}
}

// Send records the complete message content (headers, both bodies,
// attachment summaries) through the sink.
func (m *LogMailer) Send(_ context.Context, msg *Message) error {
	if msg == nil {
		return errNilMessage
	}
	if err := msg.Err(); err != nil {
		return err
	}
	if m.sink != nil {
		m.sink.Info(msg.Render())
	}
	return nil
}
