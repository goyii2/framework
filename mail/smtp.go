package mail

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	netmail "net/mail"
	"strings"

	gomail "github.com/wneessen/go-mail"
)

// SMTPConfig carries the values of the smtp driver.
type SMTPConfig struct {
	Host       string
	Port       int    // 0 picks the default for the encryption: 587, or 465 for smtps
	Username   string // empty disables authentication
	Password   string
	Encryption string // none | starttls | smtps (aliases tls/ssl); empty means starttls
}

// Encryption names for the transport security strategies.
const (
	EncryptionNone     = "none"     // plaintext, no transport security
	EncryptionStartTLS = "starttls" // plaintext connect, mandatory STARTTLS upgrade
	EncryptionSMTPS    = "smtps"    // implicit TLS from the first byte
)

// SmtpMailer delivers messages through a standard SMTP server. Each Send
// establishes its own connection (dial, deliver, close): mail volume in
// framework applications is low and per-send connections keep the mailer
// stateless and concurrency-safe without connection-pool bookkeeping.
type SmtpMailer struct {
	client *gomail.Client
	host   string
}

// NewSmtpMailer validates cfg and returns a mailer wired to the server.
// Encryption defaults to STARTTLS — modern submission servers require it —
// and authentication to PLAIN whenever a username is configured (the most
// widely supported SASL mechanism).
func NewSmtpMailer(cfg SMTPConfig) (*SmtpMailer, error) {
	if strings.TrimSpace(cfg.Host) == "" {
		return nil, errors.New("mail: smtp host is required")
	}
	encryption, err := normalizeEncryption(cfg.Encryption)
	if err != nil {
		return nil, err
	}
	port := cfg.Port
	if port == 0 {
		port = 587
		if encryption == EncryptionSMTPS {
			port = 465
		}
	}
	opts := []gomail.Option{gomail.WithPort(port)}
	switch encryption {
	case EncryptionNone:
		opts = append(opts, gomail.WithTLSPolicy(gomail.NoTLS))
	case EncryptionStartTLS:
		opts = append(opts, gomail.WithTLSPolicy(gomail.TLSMandatory))
	case EncryptionSMTPS:
		opts = append(opts, gomail.WithSSL())
	}
	if cfg.Username != "" {
		opts = append(opts,
			gomail.WithSMTPAuth(gomail.SMTPAuthPlain),
			gomail.WithUsername(cfg.Username),
			gomail.WithPassword(cfg.Password),
		)
	}
	client, err := gomail.NewClient(cfg.Host, opts...)
	if err != nil {
		return nil, fmt.Errorf("mail: smtp client for %s: %w", cfg.Host, err)
	}
	return &SmtpMailer{client: client, host: cfg.Host}, nil
}

// Send converts msg into a MIME message and delivers it to the configured
// server. The message needs a sender and at least one recipient across To,
// Cc and Bcc.
func (m *SmtpMailer) Send(ctx context.Context, msg *Message) error {
	if msg == nil {
		return errNilMessage
	}
	if err := msg.Err(); err != nil {
		return err
	}
	if msg.From().Email == "" {
		return errors.New("mail: message has no sender (set one on the message or configure mail.from)")
	}
	if len(msg.Recipients()) == 0 {
		return errors.New("mail: message has no recipient")
	}

	gm := gomail.NewMsg()
	if err := gm.FromFormat(msg.From().Name, msg.From().Email); err != nil {
		return fmt.Errorf("mail: sender: %w", err)
	}
	if rt := msg.ReplyTo(); len(rt) > 0 {
		gm.ReplyToMailAddress(toNetAddrs(rt)...)
	}
	gm.ToMailAddress(toNetAddrs(msg.To())...)
	gm.CcMailAddress(toNetAddrs(msg.Cc())...)
	gm.BccMailAddress(toNetAddrs(msg.Bcc())...)
	gm.Subject(msg.Subject())
	switch {
	case msg.TextBody() != "":
		gm.SetBodyString(gomail.TypeTextPlain, msg.TextBody())
		if msg.HTMLBody() != "" {
			gm.AddAlternativeString(gomail.TypeTextHTML, msg.HTMLBody())
		}
	case msg.HTMLBody() != "":
		// A message without a text part sends HTML as the primary body.
		gm.SetBodyString(gomail.TypeTextHTML, msg.HTMLBody())
	}
	for _, a := range msg.Attachments() {
		if err := gm.AttachReader(a.Name, bytes.NewReader(a.Data),
			gomail.WithFileContentType(gomail.ContentType(a.ContentType))); err != nil {
			return fmt.Errorf("mail: attach %s: %w", a.Name, err)
		}
	}
	if err := m.client.DialAndSendWithContext(ctx, gm); err != nil {
		return fmt.Errorf("mail: send via %s: %w", m.host, err)
	}
	return nil
}

// normalizeEncryption maps the configured transport security to its
// canonical form, accepting the Yii2 swiftmailer spellings as aliases.
func normalizeEncryption(v string) (string, error) {
	switch strings.ToLower(strings.TrimSpace(v)) {
	case "", EncryptionStartTLS, "tls":
		return EncryptionStartTLS, nil
	case EncryptionNone:
		return EncryptionNone, nil
	case EncryptionSMTPS, "ssl":
		return EncryptionSMTPS, nil
	default:
		return "", fmt.Errorf("mail: unknown smtp encryption %q (want none, starttls or smtps)", v)
	}
}

// toNetAddrs converts framework addresses to the standard library form the
// underlying client accepts.
func toNetAddrs(addrs []Address) []*netmail.Address {
	out := make([]*netmail.Address, 0, len(addrs))
	for _, a := range addrs {
		out = append(out, &netmail.Address{Name: a.Name, Address: a.Email})
	}
	return out
}
