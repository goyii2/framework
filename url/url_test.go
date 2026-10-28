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
