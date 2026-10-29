package debug

import (
	"bytes"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/a-h/templ"
	"github.com/goyii2/framework/base"
	"github.com/goyii2/framework/log"
	"github.com/goyii2/framework/web"
)

// Panel is the development-time request introspection tool. Mounted on the
// HTTP handler (Mount), it captures every request into a record — the seven
// panels: request/routing, logs, SQL, timeline, session/user, configuration
// summary, mail — writes each as one JSON file under Dir (keeping the most
// recent KeepFiles), serves the browsing pages under /debug, and injects a
// toolbar at the bottom of successful HTML responses linking the page to its
// own record. Access to the pages is restricted to AllowCIDRs (loopback by
// default).
//
// The zero Panel is not usable; build one with New.
type Panel struct {
	// Dir is the directory the per-request JSON records are written to;
	// default "runtime/debug".
	Dir string

	// KeepFiles is how many request files are retained; once more exist the
	// oldest are evicted after every write. Default 200.
	KeepFiles int

	// AllowCIDRs restricts who may open the /debug pages: CIDR blocks or
	// plain IP addresses. Default loopback only (127.0.0.1, ::1). Entries
	// that parse as neither are ignored.
	AllowCIDRs []string

	app     *base.Application
	seq     atomic.Int64
	router  requestRouter
	allow   []*net.IPNet
	allowOn sync.Once
}

// New builds the panel for app. Mounting is what installs it — see Mount
// for the debug-only rule and the ordering guidance.
func New(app *base.Application) *Panel {
	return &Panel{app: app}
}

// Mount installs the panel on the handler: the capture middleware joins the
// global chain and the /debug pages become reachable. It is a no-op unless
// the application runs with debug on, so production assemblies can call it
// unconditionally. This is the existing assembly point with the smallest
// footprint: examples wire it right after web.NewHandler, before mounting
// other global middleware — the capture must sit outside response-writing
// middleware. Page-cache filters (controller or module level) then run
// inside the capture, which is what the toolbar needs: cached pages stay
// clean and a replayed page still gets a fresh toolbar for the replay
// request.
//
// Mounting also lowers the logger's level to debug and binds a routing
// target into it: query and profile records travel at debug level and the
// panels can only see records the logger dispatches. The original targets
// keep their own configured filters; a foreign log component installed via
// SetLog cannot be extended this way and yields empty log panels.
func (p *Panel) Mount(h *web.Handler) {
	if !p.app.Config().App.Debug {
		return
	}
	p.app.SetLog(p.app.Logger().
		WithLevel(slog.LevelDebug).
		WithTarget(&dispatchTarget{router: &p.router}, log.Filter{MinLevel: slog.LevelDebug}))
	h.Use(p.Middleware())
}

// Middleware returns the capture middleware: requests under /debug are
// answered by the panel pages themselves (never captured, never injected,
// access-controlled); every other request runs the inner pipeline with its
// response held, then is recorded, answered — with the toolbar injected into
// eligible HTML — and stored.
func (p *Panel) Middleware() web.Middleware {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if isPanelRequest(r) {
				p.servePanel(w, r)
				return
			}
			p.capture(w, r, next)
		})
	}
}

// capture runs one request through the inner pipeline and records it. The
// response is held back (headers and body) until the pipeline finishes, so
// the toolbar can be woven in before anything reaches the client; a response
// that starts streaming breaks the hold and passes through live from there
// on, without a toolbar.
func (p *Panel) capture(w http.ResponseWriter, r *http.Request, next http.Handler) {
	c := web.FromContext(r.Context())
	if c == nil {
		// Reused outside the application pipeline there is no Context to
		// capture through; serve through unrecorded.
		next.ServeHTTP(w, r)
		return
	}

	start := time.Now()
	target := p.router.begin()
	defer p.router.end()

	base := c.Response()
	hold := &holdWriter{real: base}
	cw := web.NewCaptureWriter(hold)
	hold.capture = cw
	// Restore on unwind rather than inline: a panicking inner pipeline must
	// hand the original writer back too, or the error renderer's response
	// would vanish into the hold.
	restore := c.SwapWriter(cw)
	defer restore()

	// The id is minted up front so the toolbar (rendered before the record
	// is stored) can already link to this entry's detail page.
	e := &Entry{ID: p.nextID(start), Time: start, Config: summarizeConfig(p.app.Config())}
	e.Request = RequestInfo{
		Method:     r.Method,
		Path:       r.URL.Path,
		Query:      r.URL.RawQuery,
		RemoteAddr: r.RemoteAddr,
		Host:       r.Host,
		Proto:      r.Proto,
	}

	// A panic unwinds through the capture: record what is known and let the
	// pipeline's own recovery render the error response.
	defer func() {
		if v := recover(); v != nil {
			e.Request.Status = http.StatusInternalServerError
			e.Request.Error = fmt.Sprintf("panic: %v", v)
			e.Request.DurationMs = elapsedMs(time.Since(start))
			p.fillPanels(e, target, c)
			p.record(e)
			panic(v)
		}
	}()

	next.ServeHTTP(cw, r)
	// The Context writer is restored on unwind; everything below answers
	// through the held response, not through it.

	// The mux has matched by now, so the request carries its pattern; the
	// wildcard names inside it give the match parameters.
	e.Request.Route = r.Pattern
	e.Request.Params = routeParams(r)
	e.Request.Status = cw.StatusCode()
	e.Request.DurationMs = elapsedMs(time.Since(start))
	if err := c.Err(); err != nil {
		// The unified error renderer answers once the chain unwinds, outside
		// the capture; the held (possibly partial) bytes are dropped and no
		// toolbar is injected.
		e.Request.Error = err.Error()
		e.Request.Status = errorStatus(err)
		p.fillPanels(e, target, c)
		p.record(e)
		return
	}
	p.fillPanels(e, target, c)

	if !hold.released() {
		body := cw.Body()
		if toolbarEligible(cw, r) {
			if bar, err := renderToolbar(r, e); err == nil {
				body = injectBeforeBodyEnd(body, bar)
			}
		}
		hold.release(body)
	}
	p.record(e)
}

// fillPanels folds the collected material into the entry's remaining panels.
func (p *Panel) fillPanels(e *Entry, target *log.MemoryTarget, c *web.Context) {
	e.Logs, e.SQL, e.Timeline, e.Mail = collectLogPanels(target.Records())
	e.Session, e.User = collectSessionUser(c)
}

// record stores the entry; failures surface on stderr and never fail the
// response — the panel must not take the page down with it.
func (p *Panel) record(e *Entry) {
	if err := p.store(e); err != nil {
		fmt.Fprintf(os.Stderr, "goyii: debug panel: %v\n", err)
	}
}

// dir resolves the record directory.
func (p *Panel) dir() string {
	if p.Dir != "" {
		return p.Dir
	}
	return filepath.Join("runtime", "debug")
}

// nextID mints a record id: the entry's UTC timestamp plus a process-wide
// sequence, so ids sort chronologically and never collide.
func (p *Panel) nextID(t time.Time) string {
	return fmt.Sprintf("%s-%06d", t.UTC().Format("20060102T150405.000000000"), p.seq.Add(1))
}

// servePanel answers the panel's own pages. They render outside the capture
// — no record is written for browsing the panel, and no toolbar is injected
// into them.
func (p *Panel) servePanel(w http.ResponseWriter, r *http.Request) {
	if !p.allowed(r) {
		http.Error(w, "debug panel: access denied", http.StatusForbidden)
		return
	}
	if r.Method != http.MethodGet {
		w.Header().Set("Allow", http.MethodGet)
		http.Error(w, "debug panel: GET only", http.StatusMethodNotAllowed)
		return
	}
	switch r.URL.Path {
	case "/debug", "/debug/":
		entries, err := p.history(200)
		if err != nil {
			http.Error(w, "debug panel: reading history: "+err.Error(), http.StatusInternalServerError)
			return
		}
		writePage(w, r, historyPage(p.app.Config().App.Name, entries))
	case "/debug/view":
		e, err := p.load(r.URL.Query().Get("id"))
		if err != nil {
			http.Error(w, "debug panel: no such request record", http.StatusNotFound)
			return
		}
		writePage(w, r, detailPage(e))
	default:
		http.Error(w, "debug panel: unknown page", http.StatusNotFound)
	}
}

// writePage renders one panel page as an HTML response.
func writePage(w http.ResponseWriter, r *http.Request, comp templ.Component) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := comp.Render(r.Context(), w); err != nil {
		http.Error(w, "debug panel: rendering page: "+err.Error(), http.StatusInternalServerError)
	}
}

// renderToolbar renders the toolbar component into the bytes spliced into
// an HTML response.
func renderToolbar(r *http.Request, e *Entry) ([]byte, error) {
	var buf bytes.Buffer
	if err := toolbar(e).Render(r.Context(), &buf); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

// allowed reports whether the request's client address may open the panel:
// RemoteAddr's host must fall inside one of the configured networks,
// loopback by default.
func (p *Panel) allowed(r *http.Request) bool {
	p.allowOn.Do(p.parseAllow)
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		host = r.RemoteAddr
	}
	ip := net.ParseIP(host)
	if ip == nil {
		return false
	}
	for _, n := range p.allow {
		if n.Contains(ip) {
			return true
		}
	}
	return false
}

// parseAllow compiles AllowCIDRs into networks. Without configuration the
// loopback networks are the default; configured entries replace it, so a
// panel opened to a LAN is not reachable from loopback unless listed.
// Entries that parse as neither a CIDR block nor an IP are dropped.
func (p *Panel) parseAllow() {
	if len(p.AllowCIDRs) == 0 {
		p.allow = []*net.IPNet{
			{IP: net.IPv4(127, 0, 0, 1).To4(), Mask: net.CIDRMask(32, 32)},
			{IP: net.ParseIP("::1"), Mask: net.CIDRMask(128, 128)},
		}
		return
	}
	for _, s := range p.AllowCIDRs {
		var ipnet *net.IPNet
		if _, n, err := net.ParseCIDR(s); err == nil {
			ipnet = n
		} else if ip := net.ParseIP(s); ip != nil {
			if v4 := ip.To4(); v4 != nil {
				ipnet = &net.IPNet{IP: v4, Mask: net.CIDRMask(32, 32)}
			} else {
				ipnet = &net.IPNet{IP: ip, Mask: net.CIDRMask(128, 128)}
			}
		}
		if ipnet != nil {
			p.allow = append(p.allow, ipnet)
		}
	}
}

// isPanelRequest reports whether the path belongs to the panel's own pages.
func isPanelRequest(r *http.Request) bool {
	return r.URL.Path == "/debug" || strings.HasPrefix(r.URL.Path, "/debug/")
}

// routeParams extracts the wildcard values of the pattern the mux matched:
// every {name} in the pattern contributes r.PathValue(name).
func routeParams(r *http.Request) map[string]string {
	pattern := r.Pattern
	var params map[string]string
	for i := 0; i < len(pattern); i++ {
		if pattern[i] != '{' {
			continue
		}
		end := strings.IndexByte(pattern[i:], '}')
		if end < 0 {
			break
		}
		name := strings.TrimSuffix(pattern[i+1:i+end], "...")
		if name != "" {
			if v := r.PathValue(name); v != "" {
				if params == nil {
					params = map[string]string{}
				}
				params[name] = v
			}
		}
		i += end
	}
	return params
}

// toolbarEligible reports whether the toolbar belongs in the held response:
// a complete 200 HTML page (a flushed one has left the holding regime, a
// HEAD one carries no body to the client, and anything but text/html is not
// a page). Panel pages never reach this check — they are answered before
// the capture.
func toolbarEligible(cw *web.CaptureWriter, r *http.Request) bool {
	return cw.StatusCode() == http.StatusOK &&
		cw.Err() == nil &&
		!cw.Flushed() &&
		r.Method != http.MethodHead &&
		mediaType(cw.Header().Get("Content-Type")) == "text/html"
}

// mediaType trims parameters and case off a Content-Type header value.
func mediaType(v string) string {
	mt, _, _ := strings.Cut(v, ";")
	return strings.ToLower(strings.TrimSpace(mt))
}

// injectBeforeBodyEnd splices the toolbar in front of the document's last
// </body> close tag (case-insensitive); a document without one gets the
// toolbar appended.
func injectBeforeBodyEnd(body, toolbar []byte) []byte {
	idx := bytes.LastIndex(bytes.ToLower(body), []byte("</body>"))
	out := make([]byte, 0, len(body)+len(toolbar))
	if idx < 0 {
		out = append(out, body...)
		return append(out, toolbar...)
	}
	out = append(out, body[:idx]...)
	out = append(out, toolbar...)
	return append(out, body[idx:]...)
}

// errorStatus maps a recorded error to the status the unified error
// renderer will answer with.
func errorStatus(err error) int {
	var he web.HTTPError
	if errors.As(err, &he) {
		return he.Status()
	}
	return http.StatusInternalServerError
}

// elapsedMs renders a duration as fractional milliseconds.
func elapsedMs(d time.Duration) float64 {
	return float64(d.Nanoseconds()) / 1e6
}

// holdWriter is the response holder underneath the capture: it swallows the
// body the CaptureWriter forwards, so the capture middleware can rewrite the
// response (weave the toolbar in) before anything reaches the client. Flush
// breaks the hold: everything captured so far is dumped and later writes pass
// through live — streaming responses must not be buffered until the end.
//
// Header passes the wrapped writer's live map through: values accumulate
// there during the hold (nothing reaches the wire before release anyway), and
// on the error path — where the unified renderer answers through the restored
// writer — headers the action already set (Allow, Location) survive instead of
// vanishing with a private copy.
type holdWriter struct {
	real    http.ResponseWriter
	capture *web.CaptureWriter
	live    bool
}

// Header returns the wrapped writer's live header map.
func (h *holdWriter) Header() http.Header { return h.real.Header() }

// WriteHeader records nothing itself: the capture writer above it tracks the
// status, and while holding there is no client write to make. Once live the
// wrapped writer suppresses duplicates on its own.
func (h *holdWriter) WriteHeader(int) {}

// Write swallows the bytes while holding (the capture keeps the copy); after
// the hold broke it passes through live.
func (h *holdWriter) Write(b []byte) (int, error) {
	if h.live {
		return h.real.Write(b)
	}
	return len(b), nil
}

// Flush breaks the hold, dumping what the response holds so far, then
// flushes the wrapped writer when it supports it.
func (h *holdWriter) Flush() {
	if !h.live {
		h.live = true
		h.release(h.capture.Body())
	}
	if f, ok := h.real.(http.Flusher); ok {
		f.Flush()
	}
}

// released reports whether the hold broke and the response streams live.
func (h *holdWriter) released() bool { return h.live }

// release sends the (possibly rewritten) body to the wrapped writer. The
// headers are already on the wrapped writer's live map (Header passes
// through); Content-Length is dropped because the body may have been
// rewritten, and one-shot writes let net/http recompute it.
func (h *holdWriter) release(body []byte) {
	h.real.Header().Del("Content-Length")
	h.real.WriteHeader(h.capture.StatusCode())
	_, _ = h.real.Write(body)
}
