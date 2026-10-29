// Package mail sends outgoing email through a swappable Mailer: a
// development LogMailer that records messages in the application log
// without sending, and an SmtpMailer delivering through a standard SMTP
// server. Messages are built with the chainable Set* methods mirroring the
// Yii2 message interface (SetFrom, SetTo, SetSubject, SetTextBody, ...).
//
// The application wires the mailer from the mail configuration section; an
// unconfigured application gets the log mailer, so sending code is
// functional before any SMTP server exists:
//
//	err := app.Mailer().Send(ctx, mail.NewMessage().
//		SetFrom("Ada <noreply@example.com>").
//		SetTo("grace@example.com").
//		SetSubject("Welcome").
//		SetTextBody("Hello Grace").
//		SetHTMLBody("<p>Hello Grace</p>").
//		AttachContent("report.pdf", pdfBytes, "application/pdf"))
//
// Tests install a fake Mailer (or the LogMailer with a capturing sink) and
// assert on the Message without any network traffic.
package mail
