package vite

import (
	"bufio"
	"crypto/sha1"
	"encoding/base64"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
)

// TestDevProxyForwardsUnderPrefix checks both directions of the routing
// rule: paths under the prefix reach the dev server (path, query and
// forwarding headers intact), everything else falls through to the inner
// handler.
func TestDevProxyForwardsUnderPrefix(t *testing.T) {
	var mu sync.Mutex
	var seenPath, seenQuery, seenProto string
	backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		defer mu.Unlock()
		seenPath = r.URL.Path
		seenQuery = r.URL.RawQuery
		seenProto = r.Header.Get("X-Forwarded-Proto")
		w.WriteHeader(http.StatusOK)
	}))
	defer backend.Close()

	inner := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusTeapot) // 418: only the inner handler answers
	})
	handler := (&Vite{DevServer: backend.URL}).DevProxy("/assets")(inner)
	proxy := httptest.NewServer(handler)
	defer proxy.Close()

	resp, err := http.Get(proxy.URL + "/assets/main.js?v=1729574400")
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200 from the dev server", resp.StatusCode)
	}
	mu.Lock()
	defer mu.Unlock()
	if seenPath != "/assets/main.js" || seenQuery != "v=1729574400" {
		t.Errorf("backend saw %s?%s", seenPath, seenQuery)
	}
	if seenProto != "http" {
		t.Errorf("backend X-Forwarded-Proto = %q, want http", seenProto)
	}

	resp, err = http.Get(proxy.URL + "/post/view")
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusTeapot {
		t.Fatalf("status = %d, want 418 from the inner handler", resp.StatusCode)
	}
}

func TestDevProxyPrefixMatching(t *testing.T) {
	backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	defer backend.Close()

	for _, prefix := range []string{"/assets", "/assets/", " /assets "} {
		match := (&Vite{DevServer: backend.URL}).DevProxy(prefix)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusTeapot)
		}))
		for path, wantForward := range map[string]bool{
			"/assets":      true,
			"/assets/":     true,
			"/assets/x.js": true,
			"/assetsfoo":   false,
			"/other":       false,
		} {
			rec := httptest.NewRecorder()
			match.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, path, nil))
			forwarded := rec.Code != http.StatusTeapot
			if forwarded != wantForward {
				t.Errorf("prefix %q path %q forwarded = %v, want %v", prefix, path, forwarded, wantForward)
			}
		}
	}
}

func TestDevProxyNoPrefixForwardsEverything(t *testing.T) {
	backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}))
	defer backend.Close()

	handler := (&Vite{DevServer: backend.URL}).DevProxy()(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusTeapot)
	}))
	proxy := httptest.NewServer(handler)
	defer proxy.Close()

	resp, err := http.Get(proxy.URL + "/anything/at/all")
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusNoContent {
		t.Fatalf("status = %d, want 204 from the dev server", resp.StatusCode)
	}
}

func TestDevProxyHostOnlyConfigGetsScheme(t *testing.T) {
	backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	defer backend.Close()
	// httptest URLs carry a scheme; strip it to check the http:// fallback.
	host := strings.TrimPrefix(backend.URL, "http://")
	if v := (&Vite{DevServer: host}).devTarget(); v.String() != backend.URL {
		t.Errorf("devTarget = %q, want %q", v.String(), backend.URL)
	}
	if v := (&Vite{}).devTarget(); v.String() != "http://localhost:5173" {
		t.Errorf("default devTarget = %q", v.String())
	}
	if v := (&Vite{DevServer: "http://::bad::"}).devTarget(); v.String() != "http://localhost:5173" {
		t.Errorf("broken devTarget = %q, want fallback", v.String())
	}
}

func TestDevProxyUnreachableReturns502(t *testing.T) {
	// Grab a port with nothing listening on it.
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	dead := "http://" + l.Addr().String()
	l.Close()

	handler := (&Vite{DevServer: dead}).DevProxy("/assets")(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	proxy := httptest.NewServer(handler)
	defer proxy.Close()

	resp, err := http.Get(proxy.URL + "/assets/main.js")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusBadGateway {
		t.Fatalf("status = %d, want 502", resp.StatusCode)
	}
	if !strings.Contains(string(body), "vite: dev server") {
		t.Errorf("body = %q, want a vite hint", body)
	}
}

// wsGUID and wsAccept implement the RFC 6455 handshake digest, enough to
// run a real WebSocket upgrade through the proxy without a dependency.
const wsGUID = "258EAFA5-E914-47DA-95CA-C5AB0DC85B11"

func wsAccept(key string) string {
	sum := sha1.Sum([]byte(key + wsGUID))
	return base64.StdEncoding.EncodeToString(sum[:])
}

// TestDevProxyPassesWebSocketUpgrade drives a real HTTP upgrade handshake
// through the middleware: the dev server must receive the Upgrade/Connection
// headers, answer 101, and the frame it writes afterwards must reach the
// client over the hijacked connection — exactly what HMR needs.
func TestDevProxyPassesWebSocketUpgrade(t *testing.T) {
	type handshake struct{ upgrade, connection, acceptKey string }
	seen := make(chan handshake, 1)
	backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hj, ok := w.(http.Hijacker)
		if !ok {
			t.Error("backend ResponseWriter cannot hijack")
			return
		}
		accept := wsAccept(r.Header.Get("Sec-WebSocket-Key"))
		seen <- handshake{
			upgrade:    r.Header.Get("Upgrade"),
			connection: r.Header.Get("Connection"),
			acceptKey:  r.Header.Get("Sec-WebSocket-Key"),
		}
		conn, buf, err := hj.Hijack()
		if err != nil {
			t.Errorf("backend hijack: %v", err)
			return
		}
		defer conn.Close()
		// 101 switch, then one unmasked text frame "hi!".
		response := "HTTP/1.1 101 Switching Protocols\r\n" +
			"Upgrade: websocket\r\n" +
			"Connection: Upgrade\r\n" +
			"Sec-WebSocket-Accept: " + accept + "\r\n\r\n"
		if _, err := buf.WriteString(response); err != nil {
			t.Errorf("backend write 101: %v", err)
			return
		}
		if _, err := buf.Write([]byte{0x81, 0x03, 'h', 'i', '!'}); err != nil {
			t.Errorf("backend write frame: %v", err)
			return
		}
		if err := buf.Flush(); err != nil {
			t.Errorf("backend flush: %v", err)
		}
	}))
	defer backend.Close()

	handler := (&Vite{DevServer: backend.URL}).DevProxy()(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Error("inner handler reached for a proxied path")
	}))
	proxy := httptest.NewServer(handler)
	defer proxy.Close()

	conn, err := net.Dial("tcp", proxy.Listener.Addr().String())
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	request := "GET / HTTP/1.1\r\n" +
		"Host: " + proxy.Listener.Addr().String() + "\r\n" +
		"Upgrade: websocket\r\n" +
		"Connection: Upgrade\r\n" +
		"Sec-WebSocket-Key: dGhlIHNhbXBsZSBub25jZQ==\r\n" +
		"Sec-WebSocket-Version: 13\r\n\r\n"
	if _, err := conn.Write([]byte(request)); err != nil {
		t.Fatal(err)
	}

	br := bufio.NewReader(conn)
	head, err := http.ReadResponse(br, nil)
	if err != nil {
		t.Fatal(err)
	}
	if head.StatusCode != http.StatusSwitchingProtocols {
		t.Fatalf("status = %d, want 101 through the proxy", head.StatusCode)
	}
	if got := head.Header.Get("Upgrade"); !strings.EqualFold(got, "websocket") {
		t.Errorf("response Upgrade = %q, want websocket", got)
	}
	// RFC 6455 sample key -> fixed accept digest proves the digest survived
	// the round trip untouched.
	if got := head.Header.Get("Sec-WebSocket-Accept"); got != "s3pPLMBiTxaQ9kYGzzhZRbK+xOo=" {
		t.Errorf("Sec-WebSocket-Accept = %q", got)
	}

	frame := make([]byte, 5)
	if _, err := io.ReadFull(br, frame); err != nil {
		t.Fatalf("read frame: %v", err)
	}
	if string(frame) != string([]byte{0x81, 0x03, 'h', 'i', '!'}) {
		t.Errorf("frame = % x, want the backend's \"hi!\" text frame", frame)
	}

	hs := <-seen
	if !strings.EqualFold(hs.upgrade, "websocket") {
		t.Errorf("backend saw Upgrade = %q, want websocket", hs.upgrade)
	}
	if !strings.Contains(strings.ToLower(hs.connection), "upgrade") {
		t.Errorf("backend saw Connection = %q, want an Upgrade token", hs.connection)
	}
	if hs.acceptKey != "dGhlIHNhbXBsZSBub25jZQ==" {
		t.Errorf("backend saw Sec-WebSocket-Key = %q", hs.acceptKey)
	}
}
