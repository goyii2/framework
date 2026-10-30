// Package notifier is this project's own component, and the reference shape
// for capabilities the framework does not ship: the project holds the struct,
// constructs it from its own configuration section (config.Email) in the
// application entry, and hands it to the consumers it knows — constructor
// injection, the same way the auth manager reaches the controllers. There is
// no registry and no framework base class involved.
//
// The notional integration is a transactional-mail provider authenticated by
// an API key; the walkthrough delivers through the application's mail
// component instead, so the demo runs without a network or an external
// account. The provider key is still validated at boot and echoed on the
// startup log — an invalid or missing credential ends the process in the
// application entry, not in the middle of a request.
package notifier

import (
	"context"
	"errors"
	"strings"

	"github.com/goyii2/framework/examples/basic/config"
	"github.com/goyii2/framework/mail"
)

// Notifier sends the application's transactional notifications. Construct it
// with New; the zero value is not usable.
type Notifier struct {
	apiKey string
	sender string
	mailer mail.Mailer
}

// New validates the custom email section and binds its values: both the
// provider key and the sender address must be present (the address format is
// checked on compose, where the mail component validates every address).
// mailer is the delivery channel the messages ride on — the application's
// mail component, injected here so the component has no global lookup.
func New(cfg config.EmailConfig, mailer mail.Mailer) (*Notifier, error) {
	if strings.TrimSpace(cfg.APIKey) == "" {
		return nil, errors.New("notifier: email.apiKey must not be empty")
	}
	if strings.TrimSpace(cfg.Sender) == "" {
		return nil, errors.New("notifier: email.sender must not be empty")
	}
	return &Notifier{apiKey: cfg.APIKey, sender: cfg.Sender, mailer: mailer}, nil
}

// Sender returns the sender address every notification carries (the
// configured email.sender).
func (n *Notifier) Sender() string { return n.sender }

// Send composes the notification — from the configured sender, to one
// recipient — and delivers it through the mailer. The message builder
// validates addresses as they are set; Send surfaces a compose error itself,
// so a broken address fails this call regardless of the mailer behind it.
func (n *Notifier) Send(ctx context.Context, to, subject, body string) error {
	msg := mail.NewMessage().
		SetFrom(n.sender).
		SetTo(to).
		SetSubject(subject).
		SetTextBody(body)
	if err := msg.Err(); err != nil {
		return err
	}
	return n.mailer.Send(ctx, msg)
}
