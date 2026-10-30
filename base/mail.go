package base

import (
	"fmt"
	"os"

	"github.com/goyii2/framework/config"
	"github.com/goyii2/framework/mail"
)

// Mailer returns the outgoing-mail component, building it from the
// configuration's mail section on first access. The value is the mail
// package's Mailer interface — the log mailer (default) or SMTP delivery
// behind one contract — so request code depends on the contract, never a
// transport; tests replace it with SetMailer.
func (a *Application) Mailer() mail.Mailer { return a.mailComp.get() }

// SetMailer replaces the outgoing-mail component (e.g. a test fake or a
// pre-wired transport); nil restores lazy initialization from the
// configuration.
func (a *Application) SetMailer(m mail.Mailer) { a.mailComp.set(m) }

// newDefaultMail builds the default mail component from the mail
// configuration section. Delivery-independent mailer output (the log
// mailer's recorded messages) goes through the application's log component
// under the "mail" category. A section the constructor rejects (an unknown
// driver, smtp without a host) is reported on stderr and degrades to the log
// mailer so the application still starts.
func newDefaultMail(cfg config.Config, logComp *component[Log]) mail.Mailer {
	sink := loggerOf(logComp).Category("mail")
	m, err := mail.New(mail.Config{
		Driver: cfg.Mail.Driver,
		From:   cfg.Mail.From,
		SMTP:   mail.SMTPConfig(cfg.Mail.SMTP),
	}, mail.WithLogger(sink))
	if err != nil {
		fmt.Fprintf(os.Stderr, "goyii: %v; falling back to the log mailer\n", err)
		return mail.NewLogMailer(sink)
	}
	return m
}
