package web

import (
	"errors"
	"net/http"
	"testing"
)

func TestHTTPErrorConstructors(t *testing.T) {
	cases := []struct {
		err         error
		wantStatus  int
		wantMessage string
		wantText    string
	}{
		{BadRequest("bad input"), http.StatusBadRequest, "bad input", "Bad Request: bad input"},
		{Unauthorized("login required"), http.StatusUnauthorized, "login required", "Unauthorized: login required"},
		{Forbidden("members only"), http.StatusForbidden, "members only", "Forbidden: members only"},
		{NotFound("post not found"), http.StatusNotFound, "post not found", "Not Found: post not found"},
		{ServerError("disk on fire"), http.StatusInternalServerError, "disk on fire", "Internal Server Error: disk on fire"},
		{NewHTTPError(http.StatusTeapot, "short and stout"), http.StatusTeapot, "short and stout", "I'm a teapot: short and stout"},
	}
	for _, tc := range cases {
		var he HTTPError
		if !errors.As(tc.err, &he) {
			t.Errorf("%v does not implement HTTPError via errors.As", tc.err)
			continue
		}
		if he.Status() != tc.wantStatus {
			t.Errorf("Status() = %d, want %d (%v)", he.Status(), tc.wantStatus, tc.err)
		}
		if he.Message() != tc.wantMessage {
			t.Errorf("Message() = %q, want %q", he.Message(), tc.wantMessage)
		}
		if tc.err.Error() != tc.wantText {
			t.Errorf("Error() = %q, want %q", tc.err.Error(), tc.wantText)
		}
	}
}

func TestWantsJSON(t *testing.T) {
	cases := []struct {
		accept string
		want   bool
	}{
		{"", true},                         // absent header
		{"*/*", true},                      // wildcard only
		{"application/json", true},         // explicit JSON
		{"application/vnd.api+json", true}, // structured JSON suffix
		{"APPLICATION/JSON", true},         // case-insensitive media type
		{"text/html", false},               // explicit HTML
		{"application/xhtml+xml", false},   // XHTML
		{"text/html,application/xhtml+xml,application/xml;q=0.9,*/*;q=0.8", false}, // browser default
		{"application/json;q=0.8,text/html", false},                                // HTML wins on quality
		{"text/html;q=0.5,application/json;q=0.9", true},                           // JSON wins on quality
		{"application/xml,text/html;q=0.1", false},                                 // position tie-break: xml ignored, html matches
		{"application/xml", true},                                                  // no known type pinned
		{"garbage", true},                                                          // malformed header falls back to JSON
	}
	for _, tc := range cases {
		if got := wantsJSON(tc.accept); got != tc.want {
			t.Errorf("wantsJSON(%q) = %v, want %v", tc.accept, got, tc.want)
		}
	}
}
