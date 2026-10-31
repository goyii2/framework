package url_test

import (
	"strings"
	"testing"

	"github.com/goyii2/framework/url"
)

func TestNamedRouteRendersPattern(t *testing.T) {
	m := url.NewManager()
	m.Add("post/view", "/post/{id:[0-9]+}")

	got, err := m.URL("post/view", url.P{"id": "7"})
	if err != nil {
		t.Fatalf("URL(post/view) error: %v", err)
	}
	if got != "/post/7" {
		t.Errorf("URL = %q, want /post/7", got)
	}
}

func TestNamedRouteMultipleWildcards(t *testing.T) {
	m := url.NewManager()
	m.Add("user/post", "/user/{user}/post/{id:[0-9]+}")

	got, err := m.URL("user/post", url.P{"id": "9", "user": "alice"})
	if err != nil {
		t.Fatalf("URL(user/post) error: %v", err)
	}
	if got != "/user/alice/post/9" {
		t.Errorf("URL = %q, want /user/alice/post/9", got)
	}
}

func TestNamedRouteLeftoverParamsBecomeSortedQuery(t *testing.T) {
	m := url.NewManager()
	m.Add("post/list", "/post/list")

	got, err := m.URL("post/list", url.P{"tag": "go", "page": "2"})
	if err != nil {
		t.Fatalf("URL(post/list) error: %v", err)
	}
	if got != "/post/list?page=2&tag=go" {
		t.Errorf("URL = %q, want /post/list?page=2&tag=go (keys sorted)", got)
	}
}

func TestNamedRouteWithoutParams(t *testing.T) {
	m := url.NewManager()
	m.Add("home", "/")

	got, err := m.URL("home", nil)
	if err != nil {
		t.Fatalf("URL(home) error: %v", err)
	}
	if got != "/" {
		t.Errorf("URL = %q, want /", got)
	}
}

func TestNamedRouteMissingPathParamIsError(t *testing.T) {
	m := url.NewManager()
	m.Add("post/view", "/post/{id:[0-9]+}")

	_, err := m.URL("post/view", url.P{"other": "1"})
	if err == nil {
		t.Fatal("URL with a missing path parameter must error")
	}
	if !strings.Contains(err.Error(), "id") {
		t.Errorf("error = %v, want it to name the missing parameter", err)
	}
}

func TestNamedRouteEscapesValues(t *testing.T) {
	m := url.NewManager()
	m.Add("tag", "/tag/{tag}")
	m.Add("files", "/files/{path...}")

	got, err := m.URL("tag", url.P{"tag": "go/web"})
	if err != nil {
		t.Fatalf("URL(tag) error: %v", err)
	}
	if got != "/tag/go%2Fweb" {
		t.Errorf("URL = %q, want /tag/go%%2Fweb", got)
	}

	got, err = m.URL("files", url.P{"path": "docs/readme.md"})
	if err != nil {
		t.Fatalf("URL(files) error: %v", err)
	}
	if got != "/files/docs%2Freadme.md" {
		t.Errorf("URL = %q, want /files/docs%%2Freadme.md", got)
	}
}

func TestUnnamedRouteAssemblesByConvention(t *testing.T) {
	m := url.NewManager()

	cases := []struct {
		name   string
		params url.P
		want   string
	}{
		{"post/view", url.P{"id": "7"}, "/post/view?id=7"},
		{"post/view", nil, "/post/view"},
		{"admin/user/list", url.P{"page": "3"}, "/admin/user/list?page=3"},
	}
	for _, tc := range cases {
		got, err := m.URL(tc.name, tc.params)
		if err != nil {
			t.Fatalf("URL(%q) error: %v", tc.name, err)
		}
		if got != tc.want {
			t.Errorf("URL(%q) = %q, want %q", tc.name, got, tc.want)
		}
	}
}

func TestHasReportsRegistration(t *testing.T) {
	m := url.NewManager()
	if m.Has("post/view") {
		t.Error("Has on an empty manager must be false")
	}
	m.Add("post/view", "/post/{id}")
	if !m.Has("post/view") {
		t.Error("Has after Add must be true")
	}
}

func TestAddPanicsOnDuplicateName(t *testing.T) {
	defer func() {
		if recover() == nil {
			t.Error("Add with a duplicate name must panic")
		}
	}()
	m := url.NewManager()
	m.Add("post/view", "/post/{id}")
	m.Add("post/view", "/post/other/{id}")
}

func TestToUsesDefaultManager(t *testing.T) {
	url.SetDefault(nil)
	defer url.SetDefault(nil)

	m := url.NewManager()
	m.Add("post/view", "/post/{id:[0-9]+}")
	url.SetDefault(m)

	if got := url.To("post/view", url.P{"id": "5"}); got != "/post/5" {
		t.Errorf("To(named) = %q, want /post/5", got)
	}
	if got := url.To("site/hello", nil); got != "/site/hello" {
		t.Errorf("To(unnamed) = %q, want /site/hello by convention", got)
	}
	if got := url.To("post/view", nil); got != "#" {
		t.Errorf("To with a missing path parameter = %q, want #", got)
	}
}

func TestBuildAbsolutePrefixesHostInfo(t *testing.T) {
	url.SetDefault(nil)
	defer url.SetDefault(nil)

	m := url.NewManager()
	m.SetHostInfo("https://example.com")
	m.Add("post/view", "/post/{id:[0-9]+}")
	url.SetDefault(m)

	if got := url.Build("post/view", url.P{"id": "7"}).Absolute(); got != "https://example.com/post/7" {
		t.Errorf("Build(post/view).Absolute() = %q, want https://example.com/post/7", got)
	}
}

func TestBuildAbsoluteKeepsPort(t *testing.T) {
	url.SetDefault(nil)
	defer url.SetDefault(nil)

	m := url.NewManager()
	m.SetHostInfo("http://localhost:8080")
	m.Add("home", "/")
	url.SetDefault(m)

	if got := url.Build("home", nil).Absolute(); got != "http://localhost:8080/" {
		t.Errorf("Build(home).Absolute() = %q, want http://localhost:8080/", got)
	}
}

func TestSetHostInfoTrailingSlashTolerated(t *testing.T) {
	absolute := func(raw string) string {
		url.SetDefault(nil)
		defer url.SetDefault(nil)

		m := url.NewManager()
		m.SetHostInfo(raw)
		m.Add("post/view", "/post/{id}")
		url.SetDefault(m)
		return url.Build("post/view", url.P{"id": "3"}).Absolute()
	}
	plain := absolute("https://example.com")
	if plain != "https://example.com/post/3" {
		t.Errorf("Absolute() = %q, want https://example.com/post/3", plain)
	}
	if slash := absolute("https://example.com/"); slash != plain {
		t.Errorf("trailing slash = %q, want identical to %q", slash, plain)
	}
}

func TestBuildAbsoluteConventionFallback(t *testing.T) {
	url.SetDefault(nil)
	defer url.SetDefault(nil)

	m := url.NewManager()
	m.SetHostInfo("https://example.com")
	url.SetDefault(m)

	if got := url.Build("site/hello", nil).Absolute(); got != "https://example.com/site/hello" {
		t.Errorf("Build(site/hello).Absolute() = %q, want https://example.com/site/hello by convention", got)
	}
	if got := url.Build("post/view", url.P{"tag": "go", "id": "7"}).Absolute(); got != "https://example.com/post/view?id=7&tag=go" {
		t.Errorf("Build(post/view).Absolute() = %q, want https://example.com/post/view?id=7&tag=go (keys sorted)", got)
	}
}

func TestBuildAbsoluteWithoutHostInfoIsHash(t *testing.T) {
	url.SetDefault(nil)
	defer url.SetDefault(nil)

	m := url.NewManager()
	m.Add("post/view", "/post/{id}")
	url.SetDefault(m)

	if got := url.Build("post/view", url.P{"id": "7"}).Absolute(); got != "#" {
		t.Errorf("Absolute() without host info = %q, want # (no half-configured absolute URL)", got)
	}
}

func TestBuildAbsoluteMissingPathParamIsHash(t *testing.T) {
	url.SetDefault(nil)
	defer url.SetDefault(nil)

	m := url.NewManager()
	m.SetHostInfo("https://example.com")
	m.Add("post/view", "/post/{id:[0-9]+}")
	url.SetDefault(m)

	if got := url.Build("post/view", nil).Absolute(); got != "#" {
		t.Errorf("Absolute() with a missing path parameter = %q, want #", got)
	}
}

func TestSetHostInfoPanicsOnInvalidValues(t *testing.T) {
	for _, raw := range []string{
		"example.com",              // no scheme
		"//example.com",            // authority without a scheme
		"https://",                 // no host
		"https:///post",            // no host, path present
		"://example.com",           // unparseable
		"https://u@example.com",    // userinfo is not part of the format
		"https://example.com/base", // path beyond the host
		"https://example.com/?q=1", // query
	} {
		func() {
			defer func() {
				r := recover()
				if r == nil {
					t.Errorf("SetHostInfo(%q) must panic", raw)
					return
				}
				msg, ok := r.(string)
				if !ok || !strings.Contains(msg, "host info") || !strings.Contains(msg, "wiring") {
					t.Errorf("SetHostInfo(%q) panic = %v, want a wiring-mistake message naming the host info", raw, r)
				}
			}()
			url.NewManager().SetHostInfo(raw)
		}()
	}
}

func TestBuildUsesDefaultManager(t *testing.T) {
	url.SetDefault(nil)
	defer url.SetDefault(nil)

	m := url.NewManager()
	m.SetHostInfo("https://example.com")
	m.Add("post/view", "/post/{id:[0-9]+}")
	url.SetDefault(m)

	if got := url.Build("post/view", url.P{"id": "5"}).Absolute(); got != "https://example.com/post/5" {
		t.Errorf("Build(named).Absolute() = %q, want https://example.com/post/5", got)
	}
	if got := url.Build("site/hello", nil).Absolute(); got != "https://example.com/site/hello" {
		t.Errorf("Build(unnamed).Absolute() = %q, want https://example.com/site/hello by convention", got)
	}
	if got := url.Build("post/view", nil).Absolute(); got != "#" {
		t.Errorf("Build with a missing path parameter .Absolute() = %q, want #", got)
	}

	bare := url.NewManager()
	bare.Add("post/view", "/post/{id:[0-9]+}")
	url.SetDefault(bare)
	if got := url.Build("post/view", url.P{"id": "5"}).Absolute(); got != "#" {
		t.Errorf("Build without host info .Absolute() = %q, want #", got)
	}
}

func TestBuildStringMatchesTo(t *testing.T) {
	url.SetDefault(nil)
	defer url.SetDefault(nil)

	m := url.NewManager()
	m.SetHostInfo("https://example.com")
	m.Add("post/view", "/post/{id:[0-9]+}")
	m.Add("post/list", "/post/list")
	url.SetDefault(m)

	cases := []struct {
		name   string
		params url.P
		want   string
	}{
		{"post/view", url.P{"id": "7"}, "/post/7"},
		{"post/list", url.P{"tag": "go", "page": "2"}, "/post/list?page=2&tag=go"},
		{"site/hello", nil, "/site/hello"},
		{"post/view", nil, "#"},
	}
	for _, tc := range cases {
		b := url.Build(tc.name, tc.params)
		if got := b.String(); got != tc.want {
			t.Errorf("Build(%q).String() = %q, want %q", tc.name, got, tc.want)
		}
		if viaTo := url.To(tc.name, tc.params); viaTo != tc.want {
			t.Errorf("To(%q) = %q, want %q (String and To must agree)", tc.name, viaTo, tc.want)
		}
	}
}

func TestBuildHostOverridesPrefix(t *testing.T) {
	url.SetDefault(nil)
	defer url.SetDefault(nil)

	m := url.NewManager()
	m.SetHostInfo("https://example.com")
	m.Add("post/view", "/post/{id}")
	url.SetDefault(m)

	if got := url.Build("post/view", url.P{"id": "7"}).Host("https://backend.example.com").Absolute(); got != "https://backend.example.com/post/7" {
		t.Errorf("Host override = %q, want https://backend.example.com/post/7", got)
	}
	if got := url.Build("post/view", url.P{"id": "7"}).Host("https://backend.example.com/").Absolute(); got != "https://backend.example.com/post/7" {
		t.Errorf("Host override with trailing slash = %q, want identical to without", got)
	}
	// String ignores the host override: it is a relative rendering.
	if got := url.Build("post/view", url.P{"id": "7"}).Host("https://backend.example.com").String(); got != "/post/7" {
		t.Errorf("Host override then String = %q, want /post/7", got)
	}
	// An explicit host also serves a manager that never got host info.
	bare := url.NewManager()
	bare.Add("post/view", "/post/{id}")
	url.SetDefault(bare)
	if got := url.Build("post/view", url.P{"id": "7"}).Host("http://localhost:8080").Absolute(); got != "http://localhost:8080/post/7" {
		t.Errorf("Host on a manager without host info = %q, want http://localhost:8080/post/7", got)
	}
}

func TestBuildHostInvalidValuesAnswerHash(t *testing.T) {
	url.SetDefault(nil)
	defer url.SetDefault(nil)

	m := url.NewManager()
	m.SetHostInfo("https://example.com")
	m.Add("post/view", "/post/{id}")
	url.SetDefault(m)

	// Same invalid shapes SetHostInfo rejects at boot; here they must only
	// degrade the answer to "#", never panic on the request path.
	for _, h := range []string{
		"example.com",
		"//example.com",
		"https://",
		"https:///post",
		"://example.com",
		"https://u@example.com",
		"https://example.com/base",
		"https://example.com/?q=1",
	} {
		if got := url.Build("post/view", url.P{"id": "7"}).Host(h).Absolute(); got != "#" {
			t.Errorf("Host(%q).Absolute() = %q, want #", h, got)
		}
	}
}

func TestSetHostInfoConcurrentWithReads(t *testing.T) {
	url.SetDefault(nil)
	defer url.SetDefault(nil)

	m := url.NewManager()
	m.Add("post/view", "/post/{id}")
	m.SetHostInfo("https://example.com")
	url.SetDefault(m)

	done := make(chan struct{})
	go func() {
		defer close(done)
		for i := 0; i < 100; i++ {
			m.SetHostInfo("https://example.com/")
		}
	}()
	for i := 0; i < 100; i++ {
		if _, err := m.URL("post/view", url.P{"id": "7"}); err != nil {
			t.Fatalf("URL during concurrent SetHostInfo error: %v", err)
		}
		if got := url.Build("post/view", url.P{"id": "7"}).Absolute(); got != "https://example.com/post/7" {
			t.Fatalf("Build().Absolute() during concurrent SetHostInfo = %q, want https://example.com/post/7", got)
		}
	}
	<-done
}
