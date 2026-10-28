package helpers

import (
	"testing"

	"github.com/goyii2/framework/url"
)

func TestUrlQuerySortedAndEncoded(t *testing.T) {
	got := UrlQuery(url.P{"b": "2", "a": "1", "q": "hello world"})
	if got != "a=1&b=2&q=hello+world" {
		t.Errorf("got %s", got)
	}
	if got := UrlQuery(url.P{}); got != "" {
		t.Errorf("empty params should give an empty query, got %q", got)
	}
	if got := UrlQuery(nil); got != "" {
		t.Errorf("nil params should give an empty query, got %q", got)
	}
	if got := UrlQuery(url.P{"path": "/a b"}); got != "path=%2Fa+b" {
		t.Errorf("special characters must be percent-encoded, got %s", got)
	}
}

func TestUrlBuild(t *testing.T) {
	if got := UrlBuild("/post", url.P{"id": "7"}); got != "/post?id=7" {
		t.Errorf("got %s", got)
	}
	if got := UrlBuild("/post", nil); got != "/post" {
		t.Errorf("no params must leave the path untouched, got %s", got)
	}
	if got := UrlBuild("/post?page=2", url.P{"id": "7"}); got != "/post?page=2&id=7" {
		t.Errorf("existing query must be extended with &, got %s", got)
	}
	if got := UrlBuild("/post", url.P{"z": "1", "a": "2"}); got != "/post?a=2&z=1" {
		t.Errorf("params must be sorted, got %s", got)
	}
}

func TestUrlAddQuery(t *testing.T) {
	got, err := UrlAddQuery("https://example.com/blog?page=2", url.P{"tag": "go", "page": "3"})
	if err != nil {
		t.Fatal(err)
	}
	if got != "https://example.com/blog?page=3&tag=go" {
		t.Errorf("new keys are added, same-named keys replaced: got %s", got)
	}

	got, err = UrlAddQuery("https://example.com/a/b?x=1#frag", url.P{"y": "2"})
	if err != nil {
		t.Fatal(err)
	}
	if got != "https://example.com/a/b?x=1&y=2#frag" {
		t.Errorf("scheme, path and fragment must survive: got %s", got)
	}

	if _, err := UrlAddQuery("://broken", url.P{"a": "1"}); err == nil {
		t.Error("an unparseable URL must be an error")
	}
}
