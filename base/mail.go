package base

import (
	"github.com/goyii2/framework/mail"
)

// Mailer returns the outgoing-mail component. The value is the mail package's
// Mailer interface — the log mailer or SMTP delivery behind one contract — so
// request code depends on the contract, never a transport; tests replace it
// with SetMailer.
func (a *Application) Mailer() mail.Mailer { return a.mail }

// SetMailer replaces the outgoing-mail component (e.g. a test fake or the
// mailer built from the project's configuration); nil restores the log-mailer
// fallback, which records messages through the log component under the
// "mail" category.
func (a *Application) SetMailer(m mail.Mailer) {
	if m == nil {
		m = newDefaultMail(a)
	}
	a.mail = m
}

// newDefaultMail builds the fallback mail component: the log mailer, which
// records every message through the log component under the "mail" category
// instead of sending.
func newDefaultMail(a *Application) mail.Mailer {
	return mail.NewLogMailer(appLogSink{a: a})
}

// appLogSink forwards the fallback mailer's records into the application's
// log component as it is at call time, under the mail category — so a logger
// installed after construction (the assembly order: build the app, then
// install the configured logger) still receives them.
type appLogSink struct{ a *Application }

func (s appLogSink) Info(msg string, args ...any) {
	s.a.Logger().Category("mail").Info(msg, args...)
}
