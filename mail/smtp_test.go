package mail

import (
	"bufio"
	"context"
	"encoding/base64"
	"fmt"
	"net"
	"strconv"
	"strings"
	"sync"
	"testing"
)

// stubOption tweaks one stub server behavior.
type stubOption func(*stubSMTPServer)

// advertiseAUTH lists "AUTH PLAIN" in the EHLO capabilities and accepts any
// AUTH PLAIN attempt, recording the decoded identity.
func advertiseAUTH(s *stubSMTPServer) { s.advertiseAuth = true }

// failRCPT rejects every RCPT TO with a 550 so delivery fails.
func failRCPT(s *stubSMTPServer) { s.failRcpt = true }

// stubSMTPServer is a minimal in-process SMTP server speaking enough of the
// protocol (greeting, EHLO, optional AUTH, MAIL/RCPT/DATA, QUIT) to
// exercise a real client conversation without any network egress.
type stubSMTPServer struct {
	ln net.Listener

	mu           sync.Mutex
	cmds         []string // every command line received, in order
	data         string   // raw message data between DATA and the terminating dot
	authUser     string   // identity decoded from AUTH PLAIN
	authPassword string

	advertiseAuth bool
	failRcpt      bool
}

// newStubSMTPServer starts a server on a loopback port chosen by the
// system; it shuts down with the test.
func newStubSMTPServer(t *testing.T, opts ...stubOption) *stubSMTPServer {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listening for the stub SMTP server: %v", err)
	}
	s := &stubSMTPServer{ln: ln}
	for _, opt := range opts {
		opt(s)
	}
	t.Cleanup(func() { _ = ln.Close() })
	go func() {
		for {
			conn, err := ln.Accept()
			if err != nil {
				return
			}
			go s.handle(conn)
		}
	}()
	return s
}

// port returns the port the server listens on.
func (s *stubSMTPServer) port() int {
	t := strings.LastIndex(s.ln.Addr().String(), ":")
	port, err := strconv.Atoi(s.ln.Addr().String()[t+1:])
	if err != nil {
		panic(fmt.Sprintf("stub SMTP server address %s: %v", s.ln.Addr(), err))
	}
	return port
}

// commands returns the command lines seen so far joined by newlines.
func (s *stubSMTPServer) commands() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return strings.Join(s.cmds, "\n")
}

// messageData returns the raw data received after DATA.
func (s *stubSMTPServer) messageData() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.data
}

// identity returns the credentials decoded from AUTH PLAIN.
func (s *stubSMTPServer) identity() (user, password string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.authUser, s.authPassword
}

// record appends one command line.
func (s *stubSMTPServer) record(line string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.cmds = append(s.cmds, line)
}

func (s *stubSMTPServer) setData(data string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.data = data
}

func (s *stubSMTPServer) setIdentity(user, password string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.authUser = user
	s.authPassword = password
}

// handle drives one connection through the SMTP conversation.
func (s *stubSMTPServer) handle(conn net.Conn) {
	defer conn.Close()
	r := bufio.NewReader(conn)
	w := bufio.NewWriter(conn)
	writeLine := func(line string) {
		fmt.Fprintf(w, "%s\r\n", line)
		_ = w.Flush()
	}

	writeLine("220 stub.example ESMTP goyii-test ready")
	for {
		line, err := r.ReadString('\n')
		if err != nil {
			return
		}
		line = strings.TrimRight(line, "\r\n")
		upper := strings.ToUpper(line)
		switch {
		case strings.HasPrefix(upper, "EHLO"), strings.HasPrefix(upper, "HELO"):
			s.record(line)
			if s.advertiseAuth {
				writeLine("250-stub.example")
				writeLine("250-AUTH PLAIN")
				writeLine("250 OK")
			} else {
				writeLine("250-stub.example")
				writeLine("250 OK")
			}
		case strings.HasPrefix(upper, "AUTH"):
			fields := strings.Fields(line)
			if len(fields) >= 3 && strings.EqualFold(fields[0], "AUTH") && strings.EqualFold(fields[1], "PLAIN") {
				s.record(line)
				s.decodePLAIN(fields[2])
				writeLine("235 2.7.0 authenticated")
				continue
			}
			// AUTH PLAIN without initial response: prompt, read the payload.
			s.record(line)
			writeLine("334 ")
			payload, err := r.ReadString('\n')
			if err != nil {
				return
			}
			s.decodePLAIN(strings.TrimSpace(payload))
			writeLine("235 2.7.0 authenticated")
		case strings.HasPrefix(upper, "MAIL FROM:"):
			s.record(line)
			writeLine("250 OK")
		case strings.HasPrefix(upper, "RCPT TO:"):
			s.record(line)
			if s.failRcpt {
				writeLine("550 5.1.1 user unknown")
			} else {
				writeLine("250 OK")
			}
		case upper == "DATA":
			s.record(line)
			writeLine("354 end data with <CRLF>.<CRLF>")
			var b strings.Builder
			for {
				body, err := r.ReadString('\n')
				if err != nil {
					return
				}
				trimmed := strings.TrimRight(body, "\r\n")
				if trimmed == "." {
					break
				}
				b.WriteString(trimmed)
				b.WriteString("\n")
			}
			s.setData(b.String())
			writeLine("250 OK message accepted")
		case upper == "QUIT":
			s.record(line)
			writeLine("221 bye")
			return
		case upper == "RSET", upper == "NOOP":
			s.record(line)
			writeLine("250 OK")
		default:
			s.record(line)
			writeLine("500 5.5.2 command not recognized")
		}
	}
}

// decodePLAIN decodes an AUTH PLAIN payload (authzid\0authcid\0password)
// and records the identity.
func (s *stubSMTPServer) decodePLAIN(encoded string) {
	raw, err := base64.StdEncoding.DecodeString(encoded)
	if err != nil {
		return
	}
	parts := strings.Split(string(raw), "\x00")
	if len(parts) == 3 {
		s.setIdentity(parts[1], parts[2])
	}
}

// TestSmtpMailerDeliversThroughStubServer walks the full conversation: the
// mailer must greet, hand the envelope to every recipient (To, Cc and Bcc),
// submit a MIME body with both text and HTML parts and the attachment, and
// close with QUIT — while keeping the Bcc header out of the delivered data.
func TestSmtpMailerDeliversThroughStubServer(t *testing.T) {
	stub := newStubSMTPServer(t)
	mailer, err := NewSmtpMailer(SMTPConfig{
		Host:       "127.0.0.1",
		Port:       stub.port(),
		Encryption: EncryptionNone,
	})
	if err != nil {
		t.Fatalf("NewSmtpMailer: %v", err)
	}

	pdf := []byte("%PDF-1.4 goyii stub attachment")
	msg := NewMessage().
		SetFrom("Ada Lovelace <ada@example.com>").
		SetTo("grace@example.com", "Alan Turing <alan@example.com>").
		SetCc("carol@example.com").
		SetBcc("dave@example.com").
		SetReplyTo("replies@example.com").
		SetSubject("Quarterly report").
		SetTextBody("The summary is attached.").
		SetHTMLBody("<p>The summary is <b>attached</b>.</p>").
		AttachContent("report.pdf", pdf, "application/pdf")

	if err := mailer.Send(context.Background(), msg); err != nil {
		t.Fatalf("Send: %v", err)
	}

	for _, want := range []string{
		"EHLO",
		"MAIL FROM:<ada@example.com>",
		"RCPT TO:<grace@example.com>",
		"RCPT TO:<alan@example.com>",
		"RCPT TO:<carol@example.com>",
		"RCPT TO:<dave@example.com>",
		"DATA",
		"QUIT",
	} {
		if !strings.Contains(stub.commands(), want) {
			t.Errorf("SMTP conversation missing %q; got commands:\n%s", want, stub.commands())
		}
	}

	data := stub.messageData()
	// Address headers carry the client's wire format: display names quoted,
	// nameless addresses bare in angle brackets.
	for _, want := range []string{
		"Subject: Quarterly report",
		"From: \"Ada Lovelace\" <ada@example.com>",
		"To: <grace@example.com>, \"Alan Turing\" <alan@example.com>",
		"Cc: <carol@example.com>",
		"Reply-To: <replies@example.com>",
		"Content-Type: multipart/alternative",
		"Content-Type: text/plain; charset=UTF-8",
		"The summary is attached.",
		"Content-Type: text/html; charset=UTF-8",
		"The summary is <b>attached</b>.",
		"Content-Type: multipart/mixed",
		"Content-Type: application/pdf; name=\"report.pdf\"",
		"Content-Disposition: attachment; filename=\"report.pdf\"",
		base64.StdEncoding.EncodeToString(pdf),
	} {
		if !strings.Contains(data, want) {
			t.Errorf("message data missing %q; data:\n%s", want, data)
		}
	}
	// Bcc travels in the envelope only; the delivered data must not name it.
	if strings.Contains(data, "Bcc") || strings.Contains(data, "dave@example.com") {
		t.Errorf("message data must not carry the Bcc header or its addresses:\n%s", data)
	}
}

// TestSmtpMailerDeliversHTMLOnlyMessage checks a message without a text
// part still delivers a well-formed HTML body.
func TestSmtpMailerDeliversHTMLOnlyMessage(t *testing.T) {
	stub := newStubSMTPServer(t)
	mailer, err := NewSmtpMailer(SMTPConfig{Host: "127.0.0.1", Port: stub.port(), Encryption: EncryptionNone})
	if err != nil {
		t.Fatalf("NewSmtpMailer: %v", err)
	}

	msg := NewMessage().SetFrom("ada@example.com").SetTo("grace@example.com").
		SetSubject("Ping").SetHTMLBody("<p>Only HTML.</p>")
	if err := mailer.Send(context.Background(), msg); err != nil {
		t.Fatalf("Send: %v", err)
	}
	if data := stub.messageData(); !strings.Contains(data, "Content-Type: text/html") || !strings.Contains(data, "<p>Only HTML.</p>") {
		t.Errorf("HTML-only message data =\n%s", stub.messageData())
	}
}

// TestSmtpMailerAuthenticatesWithPlain exercises the AUTH PLAIN path: the
// stub advertises PLAIN, and the credentials the client submits must be the
// configured ones.
func TestSmtpMailerAuthenticatesWithPlain(t *testing.T) {
	stub := newStubSMTPServer(t, advertiseAUTH)
	mailer, err := NewSmtpMailer(SMTPConfig{
		Host:       "127.0.0.1",
		Port:       stub.port(),
		Encryption: EncryptionNone,
		Username:   "alerts",
		Password:   "s3cret",
	})
	if err != nil {
		t.Fatalf("NewSmtpMailer: %v", err)
	}

	msg := NewMessage().SetFrom("alerts@example.com").SetTo("ops@example.com").SetSubject("CPU load").
		SetTextBody("90%")
	if err := mailer.Send(context.Background(), msg); err != nil {
		t.Fatalf("Send: %v", err)
	}

	user, password := stub.identity()
	if user != "alerts" || password != "s3cret" {
		t.Errorf("AUTH PLAIN identity = %q/%q, want alerts/s3cret", user, password)
	}
	if !strings.Contains(stub.commands(), "MAIL FROM:<alerts@example.com>") {
		t.Errorf("authenticated session did not deliver; commands:\n%s", stub.commands())
	}
}

// TestSmtpMailerRecipientRejected verifies a permanent RCPT failure fails
// the Send.
func TestSmtpMailerRecipientRejected(t *testing.T) {
	stub := newStubSMTPServer(t, failRCPT)
	mailer, err := NewSmtpMailer(SMTPConfig{Host: "127.0.0.1", Port: stub.port(), Encryption: EncryptionNone})
	if err != nil {
		t.Fatalf("NewSmtpMailer: %v", err)
	}

	msg := NewMessage().SetFrom("ada@example.com").SetTo("nobody@example.com").SetTextBody("hi")
	if err := mailer.Send(context.Background(), msg); err == nil {
		t.Fatal("Send with a rejected recipient must fail")
	}
}

// TestSmtpMailerRequiresStartTLS: the default encryption is starttls, so a
// server without STARTTLS must be refused before any delivery attempt.
func TestSmtpMailerRequiresStartTLS(t *testing.T) {
	stub := newStubSMTPServer(t)
	mailer, err := NewSmtpMailer(SMTPConfig{Host: "127.0.0.1", Port: stub.port()})
	if err != nil {
		t.Fatalf("NewSmtpMailer: %v", err)
	}

	msg := NewMessage().SetFrom("ada@example.com").SetTo("grace@example.com").SetTextBody("hi")
	if err := mailer.Send(context.Background(), msg); err == nil {
		t.Fatal("Send must fail when mandatory STARTTLS is unavailable")
	}
	if cmds := stub.commands(); strings.Contains(cmds, "MAIL FROM") {
		t.Errorf("no delivery may happen without STARTTLS; commands:\n%s", cmds)
	}
}

// TestSmtpMailerUnreachableServer surfaces a connection failure with the
// host named.
func TestSmtpMailerUnreachableServer(t *testing.T) {
	// Reserve a port then close the listener so nothing answers on it.
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("reserving a port: %v", err)
	}
	port := ln.Addr().(*net.TCPAddr).Port
	_ = ln.Close()

	mailer, err := NewSmtpMailer(SMTPConfig{Host: "127.0.0.1", Port: port, Encryption: EncryptionNone})
	if err != nil {
		t.Fatalf("NewSmtpMailer: %v", err)
	}
	msg := NewMessage().SetFrom("ada@example.com").SetTo("grace@example.com").SetTextBody("hi")
	err = mailer.Send(context.Background(), msg)
	if err == nil {
		t.Fatal("Send to a closed port must fail")
	}
	if !strings.Contains(err.Error(), "127.0.0.1") {
		t.Errorf("Send error must name the server host, got: %v", err)
	}
}

// TestSmtpMailerSurfacesBuilderError: an invalid address captured while
// building must fail the Send before any server contact.
func TestSmtpMailerSurfacesBuilderError(t *testing.T) {
	stub := newStubSMTPServer(t)
	mailer, err := NewSmtpMailer(SMTPConfig{Host: "127.0.0.1", Port: stub.port(), Encryption: EncryptionNone})
	if err != nil {
		t.Fatalf("NewSmtpMailer: %v", err)
	}

	msg := NewMessage().SetFrom("ada@example.com").SetTo("not an address").SetTextBody("hi")
	if err := mailer.Send(context.Background(), msg); err == nil || !strings.Contains(err.Error(), "not an address") {
		t.Fatalf("Send must surface the builder error, got: %v", err)
	}
	if strings.Contains(stub.commands(), "EHLO") {
		t.Errorf("a message with a builder error must not reach the server; commands:\n%s", stub.commands())
	}
}

// TestSmtpMailerMissingSenderAndRecipient pin the envelope requirements.
func TestSmtpMailerMissingSenderAndRecipient(t *testing.T) {
	stub := newStubSMTPServer(t)
	mailer, err := NewSmtpMailer(SMTPConfig{Host: "127.0.0.1", Port: stub.port(), Encryption: EncryptionNone})
	if err != nil {
		t.Fatalf("NewSmtpMailer: %v", err)
	}

	noSender := NewMessage().SetTo("grace@example.com").SetTextBody("hi")
	if err := mailer.Send(context.Background(), noSender); err == nil || !strings.Contains(err.Error(), "no sender") {
		t.Errorf("Send without sender error = %v, want no-sender failure", err)
	}
	noRecipient := NewMessage().SetFrom("ada@example.com").SetTextBody("hi")
	if err := mailer.Send(context.Background(), noRecipient); err == nil || !strings.Contains(err.Error(), "no recipient") {
		t.Errorf("Send without recipient error = %v, want no-recipient failure", err)
	}
	if cmds := stub.commands(); cmds != "" {
		t.Errorf("invalid messages must not reach the server; commands:\n%s", cmds)
	}
}

// TestSmtpMailerNilMessage guards the Send contract.
func TestSmtpMailerNilMessage(t *testing.T) {
	mailer, err := NewSmtpMailer(SMTPConfig{Host: "mail.example.com", Encryption: EncryptionNone})
	if err != nil {
		t.Fatalf("NewSmtpMailer: %v", err)
	}
	if err := mailer.Send(context.Background(), nil); err == nil {
		t.Error("Send(nil) must fail")
	}
}

// TestNewSmtpMailerValidation covers the constructor's config checks.
func TestNewSmtpMailerValidation(t *testing.T) {
	if _, err := NewSmtpMailer(SMTPConfig{}); err == nil || !strings.Contains(err.Error(), "host is required") {
		t.Errorf("missing host error = %v, want host-required failure", err)
	}
	if _, err := NewSmtpMailer(SMTPConfig{Host: "mail.example.com", Encryption: "carrier-pigeon"}); err == nil ||
		!strings.Contains(err.Error(), "carrier-pigeon") {
		t.Errorf("unknown encryption error = %v, want it named", err)
	}
}

// TestNormalizeEncryption covers the encryption aliases, including the
// Yii2 swiftmailer spellings.
func TestNormalizeEncryption(t *testing.T) {
	for input, want := range map[string]string{
		"":         EncryptionStartTLS,
		"starttls": EncryptionStartTLS,
		"TLS":      EncryptionStartTLS,
		"none":     EncryptionNone,
		"smtps":    EncryptionSMTPS,
		"SSL":      EncryptionSMTPS,
	} {
		got, err := normalizeEncryption(input)
		if err != nil || got != want {
			t.Errorf("normalizeEncryption(%q) = %q, %v; want %q", input, got, err, want)
		}
	}
	if _, err := normalizeEncryption("smoke-signal"); err == nil {
		t.Error("unknown encryption must fail")
	}
}
