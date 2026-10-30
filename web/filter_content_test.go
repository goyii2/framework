package web_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/goyii2/framework/base"
	"github.com/goyii2/framework/web"
)

// contentApp mounts the negotiator globally; GET /data echoes the
// negotiated response format, GET /boom returns a not-found error whose
// rendering must follow the negotiated format, POST /data accepts JSON.
func contentApp(t *testing.T, neg web.ContentNegotiator) *web.Handler {
	t.Helper()
	app := base.New()
	h := web.NewHandler(app, web.WithoutCsrf())
	h.Use(neg.Middleware())
	h.Get("/data", func(c *web.Context) error {
		return c.JSON(map[string]string{"format": c.ResponseFormat()})
	})
	h.Get("/boom", func(c *web.Context) error { return web.NotFound("gone") })
	h.Post("/data", func(c *web.Context) error {
		return c.JSONStatus(http.StatusCreated, map[string]string{"ok": "yes"})
	})
	h.Handle("/data", func(c *web.Context) error { // any-method mount (DELETE etc.)
		return c.JSON(map[string]string{"ok": "any"})
	})
	return h
}

func TestContentNegotiatorPublishesResponseFormat(t *testing.T) {
	h := contentApp(t, web.ContentNegotiator{})

	cases := []struct {
		accept string
		want   string
	}{
		{"", "json"}, // no preference: first offered
		{"*/*", "json"},
		{"application/json", "json"},
		{"text/html,application/xhtml+xml", "html"},
		{"text/html;q=0.5,application/json;q=0.9", "json"}, // q decides
		{"application/xml,text/html;q=0.1", "html"},        // skip unsupported, take next
		{"text/*", "html"},
	}
	for _, tc := range cases {
		r := httptest.NewRequest(http.MethodGet, "/data", nil)
		r.Header.Set("Accept", tc.accept)
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, r)
		if rec.Code != http.StatusOK {
			t.Fatalf("Accept %q status = %d, want 200", tc.accept, rec.Code)
		}
		var body map[string]string
		if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
			t.Fatalf("Accept %q decode: %v", tc.accept, err)
		}
		if body["format"] != tc.want {
			t.Errorf("Accept %q negotiated %q, want %q", tc.accept, body["format"], tc.want)
		}
	}
}

func TestContentNegotiatorUnacceptableAcceptRejected406(t *testing.T) {
	h := contentApp(t, web.ContentNegotiator{})

	r := httptest.NewRequest(http.MethodGet, "/data", nil)
	r.Header.Set("Accept", "application/xml")
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, r)
	res := rec.Result()

	if res.StatusCode != http.StatusNotAcceptable {
		t.Fatalf("status = %d, want 406", res.StatusCode)
	}
	var body map[string]any
	if err := json.NewDecoder(res.Body).Decode(&body); err != nil {
		t.Fatalf("decoding body: %v", err)
	}
	if body["status"] != float64(http.StatusNotAcceptable) {
		t.Errorf("error body = %v, want the unified error shape", body)
	}
}

func TestContentNegotiatorDrivesErrorRenderingFormat(t *testing.T) {
	h := contentApp(t, web.ContentNegotiator{})

	// JSON-preferred client gets the JSON error page even though browsers'
	// default headers would prefer HTML.
	r := httptest.NewRequest(http.MethodGet, "/boom", nil)
	r.Header.Set("Accept", "application/json")
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, r)
	if got := rec.Header().Get("Content-Type"); got != "application/json; charset=utf-8" {
		t.Errorf("JSON error Content-Type = %q", got)
	}

	r = httptest.NewRequest(http.MethodGet, "/boom", nil)
	r.Header.Set("Accept", "text/html")
	rec = httptest.NewRecorder()
	h.ServeHTTP(rec, r)
	if got := rec.Header().Get("Content-Type"); got != "text/html; charset=utf-8" {
		t.Errorf("HTML error Content-Type = %q", got)
	}
	if !strings.Contains(rec.Body.String(), "gone") {
		t.Errorf("HTML error body = %q, want the error message inside", rec.Body.String())
	}
}

func TestContentNegotiatorCustomFormatsPreferenceOrder(t *testing.T) {
	h := contentApp(t, web.ContentNegotiator{
		ResponseFormats: []web.Format{
			{Type: "application/xml", Format: "xml"},
			{Type: "application/json", Format: "json"},
		},
	})

	// A wildcard Accept takes the first offered format, so the declared
	// order is the preference.
	r := httptest.NewRequest(http.MethodGet, "/data", nil)
	r.Header.Set("Accept", "*/*")
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, r)
	var body map[string]string
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if body["format"] != "xml" {
		t.Errorf("wildcard negotiated %q, want xml (first offered)", body["format"])
	}
}

func TestContentNegotiatorRejectsUnsupportedRequestContentType415(t *testing.T) {
	h := contentApp(t, web.ContentNegotiator{
		RequestFormats: []web.Format{{Type: "application/json", Format: "json"}},
	})

	r := httptest.NewRequest(http.MethodPost, "/data", strings.NewReader(`{"a":1}`))
	r.Header.Set("Content-Type", "text/plain")
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, r)
	if rec.Code != http.StatusUnsupportedMediaType {
		t.Fatalf("status = %d, want 415", rec.Code)
	}

	r = httptest.NewRequest(http.MethodPost, "/data", strings.NewReader(`{"a":1}`))
	r.Header.Set("Content-Type", "application/json; charset=utf-8") // params ignored
	rec = httptest.NewRecorder()
	h.ServeHTTP(rec, r)
	if rec.Code != http.StatusCreated {
		t.Fatalf("status = %d, want 201 for a supported content type", rec.Code)
	}
}

func TestContentNegotiatorAllowsBodylessRequestsWithoutContentType(t *testing.T) {
	h := contentApp(t, web.ContentNegotiator{
		RequestFormats: []web.Format{{Type: "application/json", Format: "json"}},
	})

	// DELETE-style calls carry no body and no Content-Type: not a 415.
	if res := req(t, h, http.MethodDelete, "/data"); res.StatusCode != http.StatusOK {
		t.Errorf("DELETE status = %d, want 200 (no Content-Type means nothing to reject)", res.StatusCode)
	}
}
