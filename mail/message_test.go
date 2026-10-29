package mail

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestMessageBuilderChain(t *testing.T) {
	m := NewMessage().
		SetFrom("Ada Lovelace <ada@example.com>").
		SetReplyTo("replies@example.com").
		SetTo("grace@example.com", "Alan <alan@example.com>").
		SetCc("carol@example.com").
		SetBcc("dave@example.com").
		SetSubject("Quarterly report").
		SetTextBody("The summary is attached.").
		SetHTMLBody("<p>Attached.</p>").
		AttachContent("report.pdf", []byte("pdf"), "application/pdf")

	wantFrom := Address{Email: "ada@example.com", Name: "Ada Lovelace"}
	if got := m.From(); got != wantFrom {
		t.Errorf("From() = %+v, want %+v", got, wantFrom)
	}
	if got, want := m.From().String(), "Ada Lovelace <ada@example.com>"; got != want {
		t.Errorf("From().String() = %q, want %q", got, want)
	}
	wantReply := []Address{{Email: "replies@example.com"}}
	if got := m.ReplyTo(); len(got) != 1 || got[0] != wantReply[0] {
		t.Errorf("ReplyTo() = %+v, want %+v", got, wantReply)
	}
	if got := m.To(); len(got) != 2 || got[0].Email != "grace@example.com" || got[1].Name != "Alan" {
		t.Errorf("To() = %+v", got)
	}
	if got := m.Cc(); len(got) != 1 || got[0].Email != "carol@example.com" {
		t.Errorf("Cc() = %+v", got)
	}
	if got := m.Bcc(); len(got) != 1 || got[0].Email != "dave@example.com" {
		t.Errorf("Bcc() = %+v", got)
	}
	if got := m.Recipients(); len(got) != 4 {
		t.Errorf("Recipients() = %+v, want to+cc+bcc = 4 addresses", got)
	}
	if m.Subject() != "Quarterly report" || m.TextBody() != "The summary is attached." || m.HTMLBody() != "<p>Attached.</p>" {
		t.Errorf("subject/body getters mismatch: %q / %q / %q", m.Subject(), m.TextBody(), m.HTMLBody())
	}
	if got := m.Attachments(); len(got) != 1 || got[0].Name != "report.pdf" || got[0].ContentType != "application/pdf" {
		t.Errorf("Attachments() = %+v", got)
	}
	if m.Err() != nil {
		t.Errorf("Err() = %v, want nil", m.Err())
	}
}

func TestMessageAddressForms(t *testing.T) {
	// Bare and named forms must parse to the same structure.
	m := NewMessage().SetTo("grace@example.com")
	if got := m.To()[0]; got.Email != "grace@example.com" || got.Name != "" {
		t.Errorf("bare address parsed to %+v", got)
	}
	m = NewMessage().SetTo("  Grace Hopper <grace@example.com>  ")
	if got := m.To()[0]; got.Email != "grace@example.com" || got.Name != "Grace Hopper" {
		t.Errorf("named address parsed to %+v", got)
	}
	bare := Address{Email: "g@example.com"}
	if got := bare.String(); got != "g@example.com" {
		t.Errorf("Address.String() without name = %q", got)
	}
}

func TestMessageSettersReplaceEarlierValues(t *testing.T) {
	m := NewMessage().SetTo("first@example.com").SetTo("second@example.com")
	if got := m.To(); len(got) != 1 || got[0].Email != "second@example.com" {
		t.Errorf("SetTo must replace, got %+v", got)
	}
	m.SetSubject("one").SetSubject("two")
	if m.Subject() != "two" {
		t.Errorf("SetSubject must replace, got %q", m.Subject())
	}
}

func TestMessageCapturesFirstError(t *testing.T) {
	m := NewMessage().SetFrom("ada@example.com").SetTo("broken", "grace@example.com")
	if m.Err() == nil || !strings.Contains(m.Err().Error(), "broken") {
		t.Fatalf("Err() = %v, want it to name the invalid To address", m.Err())
	}
	// The valid entry still parsed; the invalid one was skipped.
	if got := m.To(); len(got) != 1 || got[0].Email != "grace@example.com" {
		t.Errorf("valid entries must survive a failing one, To() = %+v", got)
	}

	// A later failure does not overwrite the first error.
	m.SetCc("also broken")
	if !strings.Contains(m.Err().Error(), "broken\"") && !strings.Contains(m.Err().Error(), "To") {
		t.Errorf("first error must be kept, got %v", m.Err())
	}
}

func TestMessageAttachFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "notes.txt")
	if err := os.WriteFile(path, []byte("hello attachment"), 0o644); err != nil {
		t.Fatalf("writing attachment file: %v", err)
	}

	m := NewMessage().AttachFile(path)
	if m.Err() != nil {
		t.Fatalf("AttachFile: %v", m.Err())
	}
	got := m.Attachments()
	if len(got) != 1 || got[0].Name != "notes.txt" || got[0].ContentType != "text/plain; charset=utf-8" ||
		string(got[0].Data) != "hello attachment" {
		t.Errorf("Attachments() = %+v", got)
	}

	m = NewMessage().AttachFile(filepath.Join(dir, "missing.bin"))
	if m.Err() == nil || !strings.Contains(m.Err().Error(), "missing.bin") {
		t.Errorf("AttachFile of a missing file must be captured in Err(), got %v", m.Err())
	}
}

func TestMessageAttachContentDefaultType(t *testing.T) {
	m := NewMessage().AttachContent("blob.bin", []byte{0x00}, "")
	if got := m.Attachments()[0].ContentType; got != "application/octet-stream" {
		t.Errorf("default content type = %q, want application/octet-stream", got)
	}
}

func TestMessageRender(t *testing.T) {
	m := NewMessage().
		SetFrom("Ada <ada@example.com>").
		SetReplyTo("replies@example.com").
		SetTo("grace@example.com", "Alan <alan@example.com>").
		SetCc("carol@example.com").
		SetBcc("dave@example.com").
		SetSubject("Render check").
		SetTextBody("plain body").
		SetHTMLBody("<p>html body</p>").
		AttachContent("report.pdf", []byte("pdf-bytes"), "application/pdf")

	rendered := m.Render()
	for _, want := range []string{
		"From: Ada <ada@example.com>",
		"Reply-To: replies@example.com",
		"To: grace@example.com, Alan <alan@example.com>",
		"Cc: carol@example.com",
		"Bcc: dave@example.com",
		"Subject: Render check",
		"Attachment: report.pdf (application/pdf, 9 bytes)",
		"plain body",
		"html body",
	} {
		if !strings.Contains(rendered, want) {
			t.Errorf("Render() missing %q; got:\n%s", want, rendered)
		}
	}

	// Omitted fields produce no lines: a bare message renders empty.
	if got := NewMessage().Render(); got != "" {
		t.Errorf("empty message Render() = %q, want empty", got)
	}
	// The attachment summary replaces the raw bytes.
	if strings.Contains(NewMessage().AttachContent("b", []byte("raw-bytes"), "text/plain").Render(), "raw-bytes") {
		t.Error("Render must not dump attachment bytes")
	}
}
