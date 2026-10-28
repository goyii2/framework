package view_test

import (
	"context"
	"io"
	"strings"
	"testing"

	"github.com/a-h/templ"
	"github.com/goyii2/framework/view"
)

// text builds a component writing fixed markup, for asserting composition
// behavior without involving templates under test.
func text(markup string) templ.Component {
	return templ.ComponentFunc(func(_ context.Context, w io.Writer) error {
		_, err := io.WriteString(w, markup)
		return err
	})
}

func render(t *testing.T, c templ.Component) string {
	t.Helper()
	var b strings.Builder
	if err := c.Render(context.Background(), &b); err != nil {
		t.Fatalf("rendering component: %v", err)
	}
	return b.String()
}

func TestRegistryRegisterLookup(t *testing.T) {
	r := view.NewRegistry()
	r.Register("post/view", view.Typed(func(data postViewData) templ.Component {
		return text("<p>" + data.Title + "</p>")
	}))

	f, ok := r.Lookup("post/view")
	if !ok {
		t.Fatal("registered view not found")
	}
	if got := render(t, f(postViewData{Title: "hello"})); got != "<p>hello</p>" {
		t.Errorf("factory output = %q", got)
	}

	if _, ok := r.Lookup("post/missing"); ok {
		t.Error("unknown view reported as found")
	}
	if names := r.Names(); len(names) != 1 || names[0] != "post/view" {
		t.Errorf("Names() = %v, want [post/view]", names)
	}
}

func TestRegistryRegisterValidations(t *testing.T) {
	cases := []struct {
		name string
		call func(r *view.Registry)
	}{
		{"duplicate name", func(r *view.Registry) {
			r.Register("a", func(any) templ.Component { return nil })
			r.Register("a", func(any) templ.Component { return nil })
		}},
		{"empty name", func(r *view.Registry) {
			r.Register("", func(any) templ.Component { return nil })
		}},
		{"nil factory", func(r *view.Registry) {
			r.Register("a", nil)
		}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			defer func() {
				if recover() == nil {
					t.Errorf("%s should panic", tc.name)
				}
			}()
			tc.call(view.NewRegistry())
		})
	}
}

type postViewData struct{ Title string }

func TestTypedFactoryTypeMismatch(t *testing.T) {
	factory := view.Typed(func(data postViewData) templ.Component { return text(data.Title) })

	if got := render(t, factory(postViewData{Title: "ok"})); got != "ok" {
		t.Errorf("matching data rendered %q", got)
	}

	defer func() {
		p := recover()
		msg, _ := p.(string)
		if p == nil || !strings.Contains(msg, "string") || !strings.Contains(msg, "postViewData") {
			t.Errorf("panic = %v, want both type names in message", p)
		}
	}()
	factory("wrong type")
}

func TestTypedAcceptsPointerType(t *testing.T) {
	factory := view.Typed(func(data *postViewData) templ.Component { return text(data.Title) })
	if got := render(t, factory(&postViewData{Title: "ptr"})); got != "ptr" {
		t.Errorf("pointer data rendered %q", got)
	}
}

func TestRegistryConcurrentLookupAfterRegister(t *testing.T) {
	r := view.NewRegistry()
	r.Register("p", view.Typed(func(d postViewData) templ.Component { return text(d.Title) }))

	done := make(chan string, 8)
	for i := 0; i < 8; i++ {
		go func() {
			f, _ := r.Lookup("p")
			done <- render(t, f(postViewData{Title: "x"}))
		}()
	}
	for i := 0; i < 8; i++ {
		if got := <-done; got != "x" {
			t.Fatalf("concurrent render produced %q", got)
		}
	}
}
