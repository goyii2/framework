package mail

import (
	"fmt"
	"mime"
	"net/http"
	"net/mail"
	"os"
	"path/filepath"
	"strings"
)

// Address is one email address with an optional display name.
type Address struct {
	Email string
	Name  string
}

// String renders the address in header form: "Name <email>" when a name is
// set, the bare address otherwise.
func (a Address) String() string {
	if a.Name == "" {
		return a.Email
	}
	return fmt.Sprintf("%s <%s>", a.Name, a.Email)
}

// parseAddress accepts both "user@example.com" and "User <user@example.com>".
func parseAddress(s string) (Address, error) {
	a, err := mail.ParseAddress(strings.TrimSpace(s))
	if err != nil {
		return Address{}, err
	}
	return Address{Email: a.Address, Name: a.Name}, nil
}

// Attachment is one file carried by a message.
type Attachment struct {
	Name        string
	ContentType string
	Data        []byte
}

// Message is one outgoing email: sender and recipients, subject, the
// plain-text and HTML bodies, and attachments. Messages are built with the
// chainable Set* methods; the first invalid input is captured and surfaced
// by Err (and every mailer's Send) instead of interrupting the chain, so a
// whole message can be assembled in one expression.
type Message struct {
	from        Address
	replyTo     []Address
	to          []Address
	cc          []Address
	bcc         []Address
	subject     string
	textBody    string
	htmlBody    string
	attachments []Attachment
	err         error
}

// NewMessage returns an empty message.
func NewMessage() *Message { return &Message{} }

// SetFrom sets the sender; addr accepts both "user@example.com" and
// "User <user@example.com>". A later call replaces an earlier one.
func (m *Message) SetFrom(addr string) *Message {
	a, err := parseAddress(addr)
	if err != nil {
		m.fail(fmt.Errorf("mail: invalid From address %q: %w", addr, err))
		return m
	}
	m.from = a
	return m
}

// SetReplyTo sets the Reply-To addresses, replacing any set earlier; each
// entry accepts the same forms as SetFrom.
func (m *Message) SetReplyTo(addrs ...string) *Message {
	m.replyTo = m.addressList("Reply-To", addrs)
	return m
}

// SetTo sets the primary recipients, replacing any set earlier; each entry
// accepts the same forms as SetFrom.
func (m *Message) SetTo(addrs ...string) *Message {
	m.to = m.addressList("To", addrs)
	return m
}

// SetCc sets the carbon-copy recipients, replacing any set earlier; each
// entry accepts the same forms as SetFrom.
func (m *Message) SetCc(addrs ...string) *Message {
	m.cc = m.addressList("Cc", addrs)
	return m
}

// SetBcc sets the blind carbon-copy recipients, replacing any set earlier.
// They receive the message like everyone else but — as with real SMTP — no
// Bcc line appears in the delivered headers; the log mailer's Render shows
// them for developer visibility.
func (m *Message) SetBcc(addrs ...string) *Message {
	m.bcc = m.addressList("Bcc", addrs)
	return m
}

// SetSubject sets the subject line.
func (m *Message) SetSubject(subject string) *Message {
	m.subject = subject
	return m
}

// SetTextBody sets the plain-text body.
func (m *Message) SetTextBody(body string) *Message {
	m.textBody = body
	return m
}

// SetHTMLBody sets the HTML body, delivered as an alternative part for
// clients that render it.
func (m *Message) SetHTMLBody(body string) *Message {
	m.htmlBody = body
	return m
}

// AttachContent attaches in-memory bytes under name. An empty contentType
// means application/octet-stream.
func (m *Message) AttachContent(name string, data []byte, contentType string) *Message {
	if contentType == "" {
		contentType = "application/octet-stream"
	}
	m.attachments = append(m.attachments, Attachment{Name: name, ContentType: contentType, Data: data})
	return m
}

// AttachFile reads path and attaches its bytes under the file's base name,
// with the content type guessed from the extension or the content itself.
// A read failure is captured and surfaced by Err.
func (m *Message) AttachFile(path string) *Message {
	data, err := os.ReadFile(path)
	if err != nil {
		m.fail(fmt.Errorf("mail: attach %s: %w", path, err))
		return m
	}
	m.attachments = append(m.attachments, Attachment{
		Name:        filepath.Base(path),
		ContentType: detectContentType(path, data),
		Data:        data,
	})
	return m
}

// detectContentType resolves an attachment's content type from the file
// extension, falling back to sniffing the first bytes.
func detectContentType(path string, data []byte) string {
	if t := mime.TypeByExtension(filepath.Ext(path)); t != "" {
		return t
	}
	const sniffLen = 512
	if len(data) > sniffLen {
		data = data[:sniffLen]
	}
	return http.DetectContentType(data)
}

// From returns the sender; Email is empty when none was set.
func (m *Message) From() Address { return m.from }

// ReplyTo returns the Reply-To addresses.
func (m *Message) ReplyTo() []Address { return m.replyTo }

// To returns the primary recipients.
func (m *Message) To() []Address { return m.to }

// Cc returns the carbon-copy recipients.
func (m *Message) Cc() []Address { return m.cc }

// Bcc returns the blind carbon-copy recipients.
func (m *Message) Bcc() []Address { return m.bcc }

// Recipients returns every address the message must be delivered to: To, Cc
// and Bcc combined.
func (m *Message) Recipients() []Address {
	out := make([]Address, 0, len(m.to)+len(m.cc)+len(m.bcc))
	out = append(out, m.to...)
	out = append(out, m.cc...)
	out = append(out, m.bcc...)
	return out
}

// Subject returns the subject line.
func (m *Message) Subject() string { return m.subject }

// TextBody returns the plain-text body.
func (m *Message) TextBody() string { return m.textBody }

// HTMLBody returns the HTML body.
func (m *Message) HTMLBody() string { return m.htmlBody }

// Attachments returns the attachments in the order they were added.
func (m *Message) Attachments() []Attachment { return m.attachments }

// Err reports the first invalid input captured while building the message:
// an unparseable address or an unreadable attachment file.
func (m *Message) Err() error { return m.err }

// addressList parses addrs into a fresh list; header names the field for
// the error message. An invalid entry is recorded on the message and the
// remaining entries still parse.
func (m *Message) addressList(header string, addrs []string) []Address {
	list := make([]Address, 0, len(addrs))
	for _, raw := range addrs {
		a, err := parseAddress(raw)
		if err != nil {
			m.fail(fmt.Errorf("mail: invalid %s address %q: %w", header, raw, err))
			continue
		}
		list = append(list, a)
	}
	return list
}

// fail records the first builder error; later ones are dropped so Send
// reports a single clear cause.
func (m *Message) fail(err error) {
	if m.err == nil {
		m.err = err
	}
}

// Render returns the complete readable message: the header lines (From,
// Reply-To, To, Cc, Bcc, Subject) followed by the text and HTML bodies and
// a per-attachment summary (name, type, size) — attachment bytes themselves
// are not dumped. Bcc appears here even though real SMTP leaves it out of
// the delivered headers, so developers see exactly who would receive the
// message.
func (m *Message) Render() string {
	var b strings.Builder
	if m.from.Email != "" {
		fmt.Fprintf(&b, "From: %s\r\n", m.from)
	}
	writeAddressLine(&b, "Reply-To", m.replyTo)
	writeAddressLine(&b, "To", m.to)
	writeAddressLine(&b, "Cc", m.cc)
	writeAddressLine(&b, "Bcc", m.bcc)
	if m.subject != "" {
		fmt.Fprintf(&b, "Subject: %s\r\n", m.subject)
	}
	for _, a := range m.attachments {
		fmt.Fprintf(&b, "Attachment: %s (%s, %d bytes)\r\n", a.Name, a.ContentType, len(a.Data))
	}
	if m.textBody != "" {
		fmt.Fprintf(&b, "\r\n--- text body ---\r\n%s\r\n", m.textBody)
	}
	if m.htmlBody != "" {
		fmt.Fprintf(&b, "\r\n--- html body ---\r\n%s\r\n", m.htmlBody)
	}
	return b.String()
}

// writeAddressLine writes one "Header: addr, addr" line when addrs is
// non-empty.
func writeAddressLine(b *strings.Builder, header string, addrs []Address) {
	if len(addrs) == 0 {
		return
	}
	values := make([]string, 0, len(addrs))
	for _, a := range addrs {
		values = append(values, a.String())
	}
	fmt.Fprintf(b, "%s: %s\r\n", header, strings.Join(values, ", "))
}
