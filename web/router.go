package web

import (
	"fmt"
	"net/http"
	"reflect"
	"regexp"
	"slices"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/goyii2/framework/base"
	"github.com/goyii2/framework/view"
)

// Route is the registration handle returned by the method helpers; Name
// optionally assigns the Yii2-style route id (e.g. "post/view") used for
// reverse URL generation.
type Route struct {
	h    *Handler
	path string // mounted path pattern, without the method prefix
}

// Name registers the route's pattern under name for reverse URL generation
// (url.To / Handler.URLs). Naming is optional; registering the same name
// twice panics like a duplicate ServeMux pattern — both are boot-time
// wiring mistakes. Returns the route so calls can chain.
func (rt *Route) Name(name string) *Route {
	rt.h.urls.Add(name, rt.path)
	return rt
}

// Path returns the mounted path pattern without the method prefix.
func (rt *Route) Path() string { return rt.path }

// Get mounts action on GET requests to path (net/http ServeMux pattern
// syntax, e.g. "/post/{id:[0-9]+}"). Path parameters are read with the
// Context.Param helpers.
func (h *Handler) Get(path string, action Action) *Route {
	return h.route(http.MethodGet, path, action)
}

// Post mounts action on POST requests to path.
func (h *Handler) Post(path string, action Action) *Route {
	return h.route(http.MethodPost, path, action)
}

// Put mounts action on PUT requests to path.
func (h *Handler) Put(path string, action Action) *Route {
	return h.route(http.MethodPut, path, action)
}

// Delete mounts action on DELETE requests to path.
func (h *Handler) Delete(path string, action Action) *Route {
	return h.route(http.MethodDelete, path, action)
}

// Patch mounts action on PATCH requests to path.
func (h *Handler) Patch(path string, action Action) *Route {
	return h.route(http.MethodPatch, path, action)
}

// Head mounts action on HEAD requests to path.
func (h *Handler) Head(path string, action Action) *Route {
	return h.route(http.MethodHead, path, action)
}

// Options mounts action on OPTIONS requests to path.
func (h *Handler) Options(path string, action Action) *Route {
	return h.route(http.MethodOptions, path, action)
}

// route mounts action for one method and returns the registration handle.
func (h *Handler) route(method, path string, action Action) *Route {
	h.Handle(method+" "+path, action)
	return &Route{h: h, path: path}
}

// ModuleRoutes registers routes below one module's prefix. Modules must be
// declared on the application first (base.NewModule +
// Application.RegisterModule); mounting under an unknown prefix is an error
// so misdeclared mounts surface at boot. The module's own controller
// registry stays the convention-fallback scope for its prefix.
type ModuleRoutes struct {
	h      *Handler
	prefix string
	layout view.Layout
}

// Module returns the route registrar scoped to the module registered under
// prefix.
func (h *Handler) Module(prefix string) (*ModuleRoutes, error) {
	if _, ok := h.app.Module(prefix); !ok {
		return nil, fmt.Errorf("web: no module registered under %q", prefix)
	}
	return &ModuleRoutes{h: h, prefix: strings.TrimRight(prefix, "/")}, nil
}

// Layout sets the page frame for the routes mounted below the module —
// the module-level layout override. Call it before mounting the module's
// routes (it applies to subsequently mounted actions); actions can still
// override the frame per page with Context.SetLayout. Returns the registrar
// so calls chain.
func (m *ModuleRoutes) Layout(l view.Layout) *ModuleRoutes {
	m.layout = l
	return m
}

// wrap applies the module's layout to action, if one is set.
func (m *ModuleRoutes) wrap(action Action) Action {
	if m.layout == nil {
		return action
	}
	return func(c *Context) error {
		c.SetLayout(m.layout)
		return action(c)
	}
}

// Get mounts action on GET requests below the module prefix; path is
// joined onto the prefix (a leading slash is optional).
func (m *ModuleRoutes) Get(path string, action Action) *Route {
	return m.h.route(http.MethodGet, m.join(path), m.wrap(action))
}

// Post mounts action on POST requests below the module prefix.
func (m *ModuleRoutes) Post(path string, action Action) *Route {
	return m.h.route(http.MethodPost, m.join(path), m.wrap(action))
}

// Put mounts action on PUT requests below the module prefix.
func (m *ModuleRoutes) Put(path string, action Action) *Route {
	return m.h.route(http.MethodPut, m.join(path), m.wrap(action))
}

// Delete mounts action on DELETE requests below the module prefix.
func (m *ModuleRoutes) Delete(path string, action Action) *Route {
	return m.h.route(http.MethodDelete, m.join(path), m.wrap(action))
}

// Patch mounts action on PATCH requests below the module prefix.
func (m *ModuleRoutes) Patch(path string, action Action) *Route {
	return m.h.route(http.MethodPatch, m.join(path), m.wrap(action))
}

// join resolves path below the module prefix.
func (m *ModuleRoutes) join(path string) string {
	return m.prefix + "/" + strings.TrimLeft(path, "/")
}

// wildcardConstraint is the per-request constraint of a "{name:regexp}"
// path wildcard; the ServeMux itself has no regexp wildcards, so the
// constraint is enforced in front of the mounted action.
type wildcardConstraint struct {
	name string
	re   *regexp.Regexp
}

// verbBinding remembers one explicitly mounted path pattern: the verbs it
// was mounted for (anyVerb when a mount carried no method prefix) and the
// wildcard constraints collected from "{name:regexp}" segments. It powers
// the 405 + Allow answer for requests that hit a bound path with an unbound
// verb.
type verbBinding struct {
	pattern     string // raw path pattern (method prefix stripped)
	verbs       []string
	anyVerb     bool
	constraints []wildcardConstraint
}

// recordVerbs adds the pattern's verb to the binding table. Host-prefixed
// patterns are skipped: they never reach the convention fallback anyway, so
// answering 405 for them is not the handler's business.
func (h *Handler) recordVerbs(pattern string, constraints []wildcardConstraint) {
	method, path := splitVerbPattern(pattern)
	if !strings.HasPrefix(path, "/") {
		return
	}
	b := h.verbBindings[path]
	if b == nil {
		b = &verbBinding{pattern: path, constraints: constraints}
		h.verbBindings[path] = b
	}
	if method == "" {
		b.anyVerb = true
		return
	}
	b.verbs = append(b.verbs, method)
}

// splitVerbPattern separates an optional "METHOD " prefix; found is false
// for plain path-only patterns and (conservatively) host patterns.
func splitVerbPattern(pattern string) (method, path string) {
	first, rest, found := strings.Cut(pattern, " ")
	if !found || !strings.HasPrefix(rest, "/") {
		return "", pattern
	}
	return strings.ToUpper(first), rest
}

// explicitVerbs reports the verbs the request path is explicitly bound to.
// Bindings whose wildcard constraints reject the path values contribute
// nothing (such requests are genuine convention-fallback material). The
// boolean reports anyVerb: a method-less mount accepts every verb.
func (h *Handler) explicitVerbs(r *http.Request) (verbs []string, anyVerb bool) {
	for _, b := range h.verbBindings {
		values, ok := matchPathPattern(b.pattern, r.URL.Path)
		if !ok {
			continue
		}
		if !constraintsMatch(b.constraints, values) {
			continue
		}
		if b.anyVerb {
			return nil, true
		}
		verbs = append(verbs, b.verbs...)
	}
	return verbs, false
}

// constraintsMatch checks the recorded wildcard constraints against the
// values matchPathPattern captured.
func constraintsMatch(constraints []wildcardConstraint, values map[string]string) bool {
	for _, wc := range constraints {
		if !wc.re.MatchString(values[wc.name]) {
			return false
		}
	}
	return true
}

// matchPathPattern reports whether path matches an explicitly mounted path
// pattern and returns the wildcard values it captured. Segments equal
// literally; "{name}" and "{name:regexp}" match one segment; a final
// "{name...}" matches the whole remainder (possibly empty); a final "{$}"
// matches only the exact path without it. Patterns that are not plain paths
// (host patterns) never match. It mirrors ServeMux wildcard matching for
// the syntax the handler accepts, without redirect semantics.
func matchPathPattern(pattern, path string) (map[string]string, bool) {
	if strings.HasSuffix(pattern, "{$}") && path == strings.TrimSuffix(pattern, "{$}") {
		return nil, true
	}
	p := strings.Split(pattern, "/")
	s := strings.Split(path, "/")
	values := map[string]string{}
	for i := 0; i < len(p); i++ {
		seg := p[i]
		if seg == "{$}" {
			// Only valid as the final segment; handled above.
			return nil, false
		}
		if name, ok := trailingWildcardName(seg); ok {
			if i >= len(s) {
				values[name] = ""
			} else {
				values[name] = strings.Join(s[i:], "/")
			}
			return values, true
		}
		if i >= len(s) {
			return nil, false
		}
		if name, ok := wildcardName(seg); ok {
			values[name] = s[i]
			continue
		}
		if seg != s[i] {
			return nil, false
		}
	}
	if len(p) != len(s) {
		return nil, false
	}
	return values, true
}

// wildcardName recognizes a "{name}" or "{name:regexp}" segment and returns
// the wildcard's name.
func wildcardName(seg string) (string, bool) {
	if len(seg) < 3 || seg[0] != '{' || seg[len(seg)-1] != '}' {
		return "", false
	}
	name, _, _ := strings.Cut(seg[1:len(seg)-1], ":")
	if name == "" {
		return "", false
	}
	return name, true
}

// trailingWildcardName recognizes a final "{name...}" segment.
func trailingWildcardName(seg string) (string, bool) {
	if len(seg) < 6 || seg[0] != '{' || !strings.HasSuffix(seg, "...}") {
		return "", false
	}
	name := seg[1 : len(seg)-4]
	if name == "" {
		return "", false
	}
	return name, true
}

// expandWildcards rewrites the "{name:regexp}" wildcard syntax into the
// plain "{name}" form the ServeMux accepts and returns the constraints to
// enforce per request. A value missing its constraint counts as unmatched,
// so such requests fall through to the convention fallback instead of
// reaching the action. Invalid regexps panic at registration, like invalid
// ServeMux patterns.
func expandWildcards(pattern string) (string, []wildcardConstraint) {
	// Separate an optional "METHOD " prefix so only path segments are
	// rewritten.
	prefix, rest := "", pattern
	if i := strings.IndexByte(rest, ' '); i >= 0 {
		prefix, rest = rest[:i+1], rest[i+1:]
	}
	segments := strings.Split(rest, "/")
	var constraints []wildcardConstraint
	for i, seg := range segments {
		name, re, ok := splitRegexpWildcard(seg)
		if !ok {
			continue
		}
		segments[i] = "{" + name + "}"
		constraints = append(constraints, wildcardConstraint{name: name, re: re})
	}
	return prefix + strings.Join(segments, "/"), constraints
}

// splitRegexpWildcard recognizes a "{name:regexp}" path segment and
// compiles the regexp; plain segments, plain wildcards and trailing
// multi-segment wildcards report false.
func splitRegexpWildcard(seg string) (string, *regexp.Regexp, bool) {
	if len(seg) < 2 || seg[0] != '{' || seg[len(seg)-1] != '}' {
		return "", nil, false
	}
	inner := seg[1 : len(seg)-1]
	if strings.HasSuffix(inner, "...") {
		return "", nil, false
	}
	name, expr, found := strings.Cut(inner, ":")
	if !found || name == "" || expr == "" {
		return "", nil, false
	}
	re, err := regexp.Compile(expr)
	if err != nil {
		panic(fmt.Sprintf("web: invalid wildcard constraint in %q: %v", seg, err))
	}
	return name, re, true
}

// controllerSource resolves controller ids, implemented by both the
// application (root scope) and modules.
type controllerSource interface {
	Controller(id string) (any, bool)
}

// mountFallback installs the catch-all finishing every request no explicit
// pattern matched: wrong-verb requests to explicitly bound paths answer
// 405 + Allow, convention routes resolve through the controller registries
// (wrapped with the controller's middleware), the rest render the
// negotiated 404. The catch-all is the least specific ServeMux pattern, so
// explicit routes always win.
func (h *Handler) mountFallback() {
	h.mux.HandleFunc("/", h.serveConvention)
}

// serveConvention resolves requests that matched no explicit route. A path
// explicitly bound under other verbs is a verb mismatch, not a routing
// miss: it answers 405 with an Allow header listing the bound verbs
// instead of silently falling through to the convention resolution. With
// the convention fallback on, /{controller}/{action} — below the longest
// registered module prefix, or at the root — maps onto ActionXxx methods
// of the registered controllers, wrapped with each controller's declared
// middleware; with the fallback off, or for paths it cannot resolve, it
// leaves a not-found error. Like the explicit dispatch it only sets
// Context.err: the pipeline renders it once. Convention routes themselves
// are not method-bound — restricting their verbs is what the VerbFilter
// middleware is for.
func (h *Handler) serveConvention(w http.ResponseWriter, r *http.Request) {
	c := FromContext(r.Context())
	if c == nil {
		// Reached without the pipeline in front (e.g. the mux was used
		// standalone); there is no request Context to run in.
		http.Error(w, "web: action invoked outside the application handler", http.StatusInternalServerError)
		return
	}
	c.r = r

	if verbs, anyVerb := h.explicitVerbs(r); !anyVerb && !containsVerb(verbs, r.Method) {
		if len(verbs) > 0 {
			slices.Sort(verbs)
			c.w.Header().Set("Allow", strings.Join(uniq(verbs), ", "))
			c.err = MethodNotAllowed("method not allowed")
			return
		}
	}

	var action Action
	if h.conventionFallback {
		var controller any
		action, controller, c.err = h.resolveConvention(r.URL.Path)
		if action != nil {
			action = withMiddleware(action, controllerMiddleware(controller))
		}
	} else {
		c.err = NotFound("page not found")
	}
	if action != nil {
		c.err = action(c)
	}
}

// containsVerb reports whether verb is listed.
func containsVerb(verbs []string, verb string) bool {
	return slices.Contains(verbs, verb)
}

// uniq drops adjacent duplicates of a sorted slice.
func uniq(sorted []string) []string {
	out := sorted[:0]
	for i, v := range sorted {
		if i == 0 || v != sorted[i-1] {
			out = append(out, v)
		}
	}
	return out
}

// resolveConvention maps path onto a controller action by naming
// convention. The longest registered module prefix matching path at a
// segment boundary scopes the lookup to that module's registry; anything
// else resolves through the application root registry. Exactly two
// segments (controller/action) resolve; every miss reports not-found. The
// owning controller is returned alongside the action so callers can wrap
// the action with its declared middleware.
func (h *Handler) resolveConvention(path string) (action Action, controller any, err error) {
	rest := path
	lookup := controllerSource(h.app)
	if m := h.moduleFor(path); m != nil {
		rest = path[len(m.Prefix()):]
		lookup = m
	}
	segs := strings.Split(strings.Trim(rest, "/"), "/")
	if len(segs) != 2 || segs[0] == "" || segs[1] == "" {
		return nil, nil, NotFound("page not found")
	}
	controller, ok := lookup.Controller(segs[0])
	if !ok {
		return nil, nil, NotFound("page not found")
	}
	action, err = conventionAction(controller, segs[0], segs[1])
	return action, controller, err
}

// moduleFor returns the module whose prefix is the longest registered
// prefix matching path at a segment boundary, or nil when path belongs to
// the root scope.
func (h *Handler) moduleFor(path string) *base.Module {
	modules := h.app.Modules() // sorted by prefix ascending
	for i := len(modules) - 1; i >= 0; i-- {
		p := modules[i].Prefix()
		if path == p || strings.HasPrefix(path, p+"/") {
			return modules[i]
		}
	}
	return nil
}

// conventionAction resolves the ActionXxx method of controller for the
// action id: "view" -> ActionView with the web.Action signature. An
// unknown action id reports not found; a method with the right name but
// the wrong signature is a setup mistake and reports a server error so it
// is not mistaken for a missing page.
func conventionAction(controller any, controllerID, actionID string) (Action, error) {
	method := reflect.ValueOf(controller).MethodByName("Action" + title(actionID))
	if !method.IsValid() {
		return nil, NotFound("page not found")
	}
	// Reflect hands out the unnamed func type; the defined Action type is
	// not identical to it, so assert the signature and let the return
	// convert it to Action.
	f, ok := method.Interface().(func(*Context) error)
	if !ok {
		return nil, ServerError(fmt.Sprintf("action %s/%s exists but does not match the Action signature", controllerID, actionID))
	}
	return f, nil
}

// title uppercases the first rune, turning an action id into the method
// name suffix: "view" -> "View".
func title(s string) string {
	if s == "" {
		return s
	}
	r, size := utf8.DecodeRuneInString(s)
	if r == utf8.RuneError && size <= 1 {
		return s
	}
	return string(unicode.ToUpper(r)) + s[size:]
}
