package web

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/goyii2/framework/config"
)

func TestContextErrRecordsAndReadsError(t *testing.T) {
	c := newContext(nil, httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, "/", nil))
	if c.Err() != nil {
		t.Fatalf("fresh context error = %v, want nil", c.Err())
	}
	err := NotFound("nope")
	c.SetError(err)
	if c.Err() != err {
		t.Fatalf("Err() = %v, want the recorded error", c.Err())
	}
}

func TestSwapWriterRedirectsActionWritesAndRestores(t *testing.T) {
	rec := httptest.NewRecorder()
	c := newContext(nil, rec, httptest.NewRequest(http.MethodGet, "/", nil))

	cw := NewCaptureWriter(httptest.NewRecorder())
	restore := c.SwapWriter(cw)

	// Writes through the Context (how actions write) land in the swap's
	// writer, not the original one.
	c.Response().Header().Set("Content-Type", "text/html")
	c.Response().WriteHeader(http.StatusTeapot)
	if _, err := c.Response().Write([]byte("captured")); err != nil {
		t.Fatalf("write: %v", err)
	}
	if rec.Body.Len() != 0 {
		t.Fatalf("original writer received bytes: %q", rec.Body.String())
	}
	if cw.Body() == nil || string(cw.Body()) != "captured" {
		t.Errorf("swap writer captured %q", cw.Body())
	}
	if cw.Header().Get("Content-Type") != "text/html" {
		t.Errorf("headers went to the wrong writer: %v", cw.Header())
	}

	restore()
	// After the restore the Context writes to the original writer again.
	if _, err := c.Response().Write([]byte("restored")); err != nil {
		t.Fatalf("write after restore: %v", err)
	}
	if got := rec.Body.String(); got != "restored" {
		t.Errorf("original writer got %q, want %q", got, "restored")
	}
	// The restored writer reports the status it wrote itself.
	if c.StatusCode() != http.StatusOK {
		t.Errorf("restored status = %d, want 200", c.StatusCode())
	}
}

func TestSessionAllReturnsIndependentCopy(t *testing.T) {
	m := NewSessionManager(config.Session{Name: "s", Backend: "file", Path: t.TempDir()})
	s := newSession(m, httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, "/", nil))
	s.Set("cart", "books")

	all := s.All()
	if all["cart"] != "books" {
		t.Fatalf("All() = %v, want cart=books", all)
	}
	// Mutating the snapshot affects no live state.
	all["cart"] = "mutated"
	all["extra"] = "x"
	if s.Get("cart") != "books" {
		t.Errorf("mutating All() changed the session: %v", s.Get("cart"))
	}
	if s.Get("extra") != nil {
		t.Errorf("mutating All() wrote into the session: %v", s.Get("extra"))
	}
}
