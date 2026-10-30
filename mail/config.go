package mail

import (
	"fmt"
	"strings"
)

// Config carries the outgoing-mail component values. The zero value selects
// the log driver: every message is recorded instead of sent, so sending code
// runs unchanged before any SMTP server exists.
type Config struct {
	Driver string `yaml:"driver"` // log | smtp; empty = log
	From   string `yaml:"from"`   // default sender for messages that set none, e.g. "Ada <noreply@example.com>"

	// SMTP carries the values of the smtp driver; see SMTPConfig.
	SMTP SMTPConfig `yaml:"smtp"`
}

// Option customizes mailer construction; options carry cross-component
// dependencies, values live in Config.
type Option func(*options)

// options accumulates what the options set.
type options struct {
	sink LogSink
}

// WithLogger hands the mailer the sink delivery-independent output goes
// through — the log mailer records every message there, typically the
// application's log component scoped to the "mail" category. Without it the
// log mailer's output is dropped while Send still succeeds.
func WithLogger(sink LogSink) Option {
	return func(o *options) { o.sink = sink }
}

// New builds the mailer Config names: "log" (also the empty default) records
// messages through the sink, "smtp" delivers through a real server. Config's
// From becomes the default sender for messages that set none. A
// configuration New cannot use — an unknown driver, smtp without a host, an
// unparseable From — fails construction with an error.
func New(cfg Config, opts ...Option) (Mailer, error) {
	var o options
	for _, opt := range opts {
		if opt != nil {
			opt(&o)
		}
	}
	var inner Mailer
	switch strings.ToLower(strings.TrimSpace(cfg.Driver)) {
	case "", "log":
		inner = NewLogMailer(o.sink)
	case "smtp":
		m, err := NewSmtpMailer(SMTPConfig{
			Host:       cfg.SMTP.Host,
			Port:       cfg.SMTP.Port,
			Username:   cfg.SMTP.Username,
			Password:   cfg.SMTP.Password,
			Encryption: cfg.SMTP.Encryption,
		})
		if err != nil {
			return nil, err
		}
		inner = m
	default:
		return nil, fmt.Errorf("mail: unknown driver %q (want log or smtp)", cfg.Driver)
	}
	if cfg.From == "" {
		return inner, nil
	}
	from, err := parseAddress(cfg.From)
	if err != nil {
		return nil, fmt.Errorf("mail: invalid mail.from %q: %w", cfg.From, err)
	}
	return withDefaultFrom{from: from, inner: inner}, nil
}
