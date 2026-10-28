package web

import (
	"encoding/json"
	"errors"
	"fmt"
	"html"
	"net/http"
	"sort"
	"strconv"
	"strings"
)

// HTTPError is an error carrying an HTTP status code and a safe,
// client-presentable message. It is produced by the NotFound-style
// constructors below and rendered by the unified error handler.
type HTTPError interface {
	error
	// Status is the HTTP status code the response should carry.
	Status() int
	// Message is the client-presentable explanation of the failure.
	Message() string
}

// httpError is the concrete HTTPError implementation.
type httpError struct {
	status  int
	message string
}

func (e *httpError) Error() string {
	return fmt.Sprintf("%s: %s", http.StatusText(e.status), e.message)
}

func (e *httpError) Status() int     { return e.status }
func (e *httpError) Message() string { return e.message }

// NewHTTPError builds an HTTP error with the given status and message; it
// is the generic constructor behind the named helpers below.
func NewHTTPError(status int, message string) error {
	return &httpError{status: status, message: message}
}

// BadRequest reports a malformed request (400).
func BadRequest(message string) error { return NewHTTPError(http.StatusBadRequest, message) }

// Unauthorized reports missing or invalid authentication (401).
func Unauthorized(message string) error { return NewHTTPError(http.StatusUnauthorized, message) }

// Forbidden reports a request the authenticated user may not perform (403).
func Forbidden(message string) error { return NewHTTPError(http.StatusForbidden, message) }

// NotFound reports a resource that does not exist (404).
func NotFound(message string) error { return NewHTTPError(http.StatusNotFound, message) }

// ServerError reports a server-side failure (500).
func ServerError(message string) error {
	return NewHTTPError(http.StatusInternalServerError, message)
}

// errorBody is the JSON shape of rendered errors, mirroring Yii2's error
// response fields (name / message / status).
type errorBody struct {
	Name    string `json:"name"`
	Message string `json:"message"`
	Status  int    `json:"status"`
}

// renderError writes err to the response in the format negotiated from the
// Accept header (JSON by default, HTML when preferred). HTTP errors keep
// their status and message; any other error is logged and becomes a 500
// whose real text is revealed only when the app runs with debug on.
// A response that already started is never overwritten; the error is
// logged instead.
func (h *Handler) renderError(c *Context, err error) {
	status := http.StatusInternalServerError
	var message string
	var he HTTPError
	if errors.As(err, &he) {
		status, message = he.Status(), he.Message()
	} else if h.app.Config().App.Debug {
		message = err.Error()
	}
	if message == "" {
		message = http.StatusText(status)
	}

	logArgs := []any{"method", c.r.Method, "path", c.r.URL.Path, "status", status, "err", err.Error()}
	if c.w.wrote {
		h.app.Log().Error("error after response started; not rendered", logArgs...)
		return
	}
	if status >= 500 {
		h.app.Log().Error("request failed", logArgs...)
	} else {
		h.app.Log().Debug("request rejected", logArgs...)
	}

	name := http.StatusText(status)
	if wantsJSON(c.r.Header.Get("Accept")) {
		c.w.Header().Set("Content-Type", "application/json; charset=utf-8")
		c.w.WriteHeader(status)
		_ = json.NewEncoder(c.w).Encode(errorBody{Name: name, Message: message, Status: status})
		return
	}
	c.w.Header().Set("Content-Type", "text/html; charset=utf-8")
	c.w.WriteHeader(status)
	title := html.EscapeString(fmt.Sprintf("%d %s", status, name))
	body := html.EscapeString(message)
	_, _ = fmt.Fprintf(c.w, "<!DOCTYPE html><html><head><title>%s</title></head><body><h1>%s</h1><p>%s</p></body></html>", title, title, body)
}

// wantsJSON decides the error format from an Accept header. Media ranges
// are consulted in quality order (position breaks ties): JSON types
// (application/json and any +json subtype) select JSON, HTML and XHTML
// select HTML, wildcards express no preference. An absent header or one
// without a concrete preference defaults to JSON.
func wantsJSON(accept string) bool {
	type mediaRange struct {
		typ, sub string
		q        float64
	}
	var rs []mediaRange
	for _, part := range strings.Split(accept, ",") {
		media, params, _ := strings.Cut(strings.TrimSpace(part), ";")
		typ, sub, ok := strings.Cut(media, "/")
		mr := mediaRange{typ: strings.TrimSpace(typ), sub: strings.TrimSpace(sub), q: 1}
		for params != "" {
			var kv string
			kv, params, _ = strings.Cut(params, ";")
			k, v, _ := strings.Cut(strings.TrimSpace(kv), "=")
			if strings.EqualFold(strings.TrimSpace(k), "q") {
				if f, err := strconv.ParseFloat(strings.TrimSpace(v), 64); err == nil {
					mr.q = f
				}
			}
		}
		if !ok || mr.typ == "" || mr.sub == "" {
			continue
		}
		rs = append(rs, mr)
	}
	sort.SliceStable(rs, func(i, j int) bool { return rs[i].q > rs[j].q })
	for _, r := range rs {
		if strings.Contains(r.typ, "*") || strings.Contains(r.sub, "*") {
			continue
		}
		if r.sub == "json" || strings.HasSuffix(r.sub, "+json") {
			return true
		}
		if (r.typ == "text" && r.sub == "html") || r.sub == "xhtml+xml" {
			return false
		}
	}
	return true
}
