package mail

import (
	"context"
	"strings"
	"sync"
	"testing"
)

// recordingSink captures everything logged through it.
type recordingSink struct {
	mu       sync.Mutex
	messages []string
}

func (r *recordingSink) Info(msg string, args ...any) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.messages = append(r.messages, msg)
}

func (r *recordingSink) all() []string {
	r.mu.Lock()
	defer r.mu.Unlock()
	return append([]string(nil), r.messages...)
}

// TestLogMailerRecordsCompleteMessage: the development mailer must write
// the whole message content — headers, both bodies, attachments — without
// sending anything.
func TestLogMailerRecordsCompleteMessage(t *testing.T) {
	sink := &recordingSink{}
	mailer := NewLogMailer(sink)

	err := mailer.Send(context.Background(), NewMessage().
		SetFrom("Ada <ada@example.com>").
		SetTo("grace@example.com").
		SetBcc("dave@example.com").
		SetSubject("Dev check").
		SetTextBody("plain body").
		SetHTMLBody("<p>html body</p>").
		AttachContent("report.pdf", []byte("pdf"), "application/pdf"))
	if err != nil {
		t.Fatalf("Send: %v", err)
	}

	entries := sink.all()
	if len(entries) != 1 {
		t.Fatalf("logged %d entries, want 1", len(entries))
	}
	for _, want := range []string{
		"From: Ada <ada@example.com>",
		"To: grace@example.com",
		"Bcc: dave@example.com", // visible for the developer, unlike delivered headers
		"Subject: Dev check",
		"Attachment: report.pdf (application/pdf, 3 bytes)",
		"plain body",
		"<p>html body</p>",
	} {
		if !strings.Contains(entries[0], want) {
			t.Errorf("logged message missing %q; got:\n%s", want, entries[0])
		}
	}
}

func TestLogMailerSurfacesBuilderError(t *testing.T) {
	sink := &recordingSink{}
	mailer := NewLogMailer(sink)

	msg := NewMessage().SetTo("broken")
	if err := mailer.Send(context.Background(), msg); err == nil || !strings.Contains(err.Error(), "broken") {
		t.Fatalf("Send must surface the builder error, got %v", err)
	}
	if entries := sink.all(); len(entries) != 0 {
		t.Errorf("a broken message must not be logged, got %v", entries)
	}
}

func TestLogMailerNilSinkAndMessage(t *testing.T) {
	mailer := NewLogMailer(nil)
	if err := mailer.Send(context.Background(), NewMessage().SetFrom("a@example.com").SetTextBody("hi")); err != nil {
		t.Errorf("nil sink must not fail the Send: %v", err)
	}
	if err := mailer.Send(context.Background(), nil); err == nil {
		t.Error("Send(nil) must fail")
	}
}

// TestLogMailerConcurrentSend exercises parallel sends through one shared
// mailer and sink; run with -race.
func TestLogMailerConcurrentSend(t *testing.T) {
	sink := &recordingSink{}
	mailer := NewLogMailer(sink)

	const goroutines, perGoroutine = 32, 20
	var wg sync.WaitGroup
	for i := 0; i < goroutines; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			for j := 0; j < perGoroutine; j++ {
				msg := NewMessage().
					SetFrom("load@example.com").
					SetTo("sink@example.com").
					SetSubject("burst").
					SetTextBody("body")
				if err := mailer.Send(context.Background(), msg); err != nil {
					t.Errorf("Send: %v", err)
				}
			}
		}(i)
	}
	wg.Wait()

	if got := len(sink.all()); got != goroutines*perGoroutine {
		t.Errorf("logged %d messages, want %d", got, goroutines*perGoroutine)
	}
}
