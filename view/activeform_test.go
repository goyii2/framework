package view_test

import (
	"strings"
	"testing"

	"github.com/goyii2/framework/validators"
	"github.com/goyii2/framework/view"
)

func TestActiveFormTextDefaults(t *testing.T) {
	got := render(t, view.ActiveForm{
		Fields: []view.ActiveField{
			{Name: "title", Label: "Title", Required: true, MaxLength: 128, Pattern: "[A-Za-z]+", Placeholder: "Enter title"},
		},
	}.Render())

	// An empty Type means text; Action submits to the current URL and the
	// method defaults to post; the native attributes render in the group
	// class and on the input.
	for _, want := range []string{
		`<form id="form" action="" method="post">`,
		`<div class="form-group field-title required">`,
		`<label class="control-label" for="form-title">Title</label>`,
		`<input type="text" id="form-title" class="form-control" name="title" value="" required placeholder="Enter title" maxlength="128" pattern="[A-Za-z]+">`,
		"</form>",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("form missing %q:\n%s", want, got)
		}
	}
	// No validation failure and no hint configured: no state markup.
	for _, unwanted := range []string{"has-error", "help-block", "hint-block"} {
		if strings.Contains(got, unwanted) {
			t.Errorf("clean form renders %s markup:\n%s", unwanted, got)
		}
	}
}

func TestActiveFormActionMethodID(t *testing.T) {
	got := render(t, view.ActiveForm{
		ID:     "post",
		Action: "/post/create",
		Method: "get",
		Fields: []view.ActiveField{{Name: "title", Label: "Title"}},
	}.Render())
	for _, want := range []string{
		`<form id="post" action="/post/create" method="get">`,
		`<label class="control-label" for="post-title">Title</label>`,
		`<input type="text" id="post-title"`,
	} {
		if !strings.Contains(got, want) {
			t.Errorf("form missing %q:\n%s", want, got)
		}
	}
}

func TestActiveFormEchoesSubmittedValues(t *testing.T) {
	got := render(t, view.ActiveForm{
		Fields: []view.ActiveField{
			{Name: "title", Label: "Title"},
		},
		Values: map[string]string{"title": "Hello again"},
	}.Render())
	if !strings.Contains(got, `value="Hello again"`) {
		t.Errorf("submitted value not refilled:\n%s", got)
	}
}

func TestActiveFormEchoesValidationErrors(t *testing.T) {
	errs := &validators.Errors{
		Scenario: validators.ScenarioInsert,
		Fields: map[string][]string{
			"title": {"Title cannot be blank.", "Title should contain at least 3 characters."},
			// record-level issues have no field to attach to and stay unseen
			"$root": {"the record is inconsistent"},
		},
	}
	got := render(t, view.ActiveForm{
		Fields: []view.ActiveField{
			{Name: "title", Label: "Title"},
			{Name: "body", Label: "Body", Type: view.FieldTextarea},
		},
		Errors: errs,
	}.Render())

	for _, want := range []string{
		// the failing field carries every message under the has-error state
		`<div class="form-group field-title has-error">`,
		`<div class="help-block">Title cannot be blank.</div>`,
		`<div class="help-block">Title should contain at least 3 characters.</div>`,
		// the passing field stays clean
		`<div class="form-group field-body">`,
	} {
		if !strings.Contains(got, want) {
			t.Errorf("form missing %q:\n%s", want, got)
		}
	}
	if n := strings.Count(got, "help-block"); n != 2 {
		t.Errorf("rendered %d error blocks, want 2:\n%s", n, got)
	}
}

func TestActiveFormHint(t *testing.T) {
	got := render(t, view.ActiveForm{
		Fields: []view.ActiveField{
			{Name: "slug", Label: "Slug", Hint: "Lowercase letters and dashes only."},
		},
	}.Render())
	if !strings.Contains(got, `<div class="hint-block">Lowercase letters and dashes only.</div>`) {
		t.Errorf("hint missing:\n%s", got)
	}
}

func TestActiveFormFieldTypes(t *testing.T) {
	got := render(t, view.ActiveForm{
		Fields: []view.ActiveField{
			{Name: "pwd", Label: "Password", Type: view.FieldPassword, Placeholder: "secret"},
			{Name: "body", Label: "Body", Type: view.FieldTextarea, Required: true, MaxLength: 500, Placeholder: "Tell more"},
			{Name: "status", Label: "Status", Type: view.FieldSelect, Required: true,
				Options: []view.FieldOption{{Value: "1", Label: "Active"}, {Value: "0", Label: "Inactive"}}},
			{Name: "agree", Label: "Agree", Type: view.FieldCheckbox, Required: true},
			{Name: "color", Label: "Color", Type: view.FieldRadio,
				Options: []view.FieldOption{{Value: "r", Label: "Red"}, {Value: "g", Label: "Green"}}},
			{Name: "owner", Type: view.FieldHidden},
			{Name: "age", Label: "Age", Type: view.FieldNumber},
			{Name: "at", Label: "At", Type: view.FieldDatetimeLocal},
			{Name: "att", Label: "Attachment", Type: view.FieldFile, Required: true},
		},
		Values: map[string]string{
			"pwd": "p@ss", "body": "some text", "status": "0",
			"agree": "1", "color": "g", "owner": "7", "age": "30", "at": "2026-10-29T10:00",
		},
	}.Render())

	for _, want := range []string{
		// password echoes its value; maxlength/pattern would not apply and are dropped
		`<input type="password" id="form-pwd" class="form-control" name="pwd" value="p@ss" placeholder="secret">`,
		// textarea echoes as content and keeps its native attributes
		`<textarea id="form-body" class="form-control" name="body" required maxlength="500" placeholder="Tell more">some text</textarea>`,
		// select marks the option whose Value was submitted
		`<select id="form-status" class="form-control" name="status" required><option value="1">Active</option><option value="0" selected>Inactive</option></select>`,
		// checkbox renders checked for the value it submits back
		`<input type="checkbox" id="form-agree" name="agree" value="1" checked required>`,
		// radio marks the matching option, one group name across the inputs
		`<input type="radio" name="color" value="r">`,
		`<label class="radio"><input type="radio" name="color" value="g" checked> Green</label>`,
		// hidden renders bare, without the group wrapper
		`<input type="hidden" name="owner" value="7">`,
		// number and datetime-local echo as values
		`<input type="number" id="form-age" class="form-control" name="age" value="30">`,
		`<input type="datetime-local" id="form-at" class="form-control" name="at" value="2026-10-29T10:00">`,
		// a file input cannot carry a value attribute
		`<input type="file" id="form-att" class="form-control" name="att" required>`,
	} {
		if !strings.Contains(got, want) {
			t.Errorf("form missing %q:\n%s", want, got)
		}
	}
	if strings.Contains(got, "field-owner") {
		t.Errorf("hidden field wrapped in a group:\n%s", got)
	}
}

func TestActiveFormCheckboxRefillStates(t *testing.T) {
	field := view.ActiveField{Name: "active", Label: "Active", Type: view.FieldCheckbox}
	cases := []struct {
		value   string
		checked bool
	}{
		// The value this widget's own checkbox submits back.
		{"1", true},
		// The spellings a boolean model refills through form.Values.
		{"true", true},
		{"false", false},
		{"", false},
	}
	for _, c := range cases {
		got := render(t, view.ActiveForm{
			Fields: []view.ActiveField{field},
			Values: map[string]string{"active": c.value},
		}.Render())
		want := `<input type="checkbox" id="form-active" name="active" value="1"`
		if c.checked {
			want += " checked"
		}
		if !strings.Contains(got, want+">") {
			t.Errorf("checkbox refill %q should render checked=%v:\n%s", c.value, c.checked, got)
		}
	}
}

func TestActiveFormUnselectedControlsWithoutValue(t *testing.T) {
	got := render(t, view.ActiveForm{
		Fields: []view.ActiveField{
			{Name: "status", Label: "Status", Type: view.FieldSelect,
				Options: []view.FieldOption{{Value: "1", Label: "Active"}}},
			{Name: "color", Label: "Color", Type: view.FieldRadio,
				Options: []view.FieldOption{{Value: "r", Label: "Red"}}},
			{Name: "agree", Label: "Agree", Type: view.FieldCheckbox},
		},
	}.Render())
	for _, unwanted := range []string{" selected", " checked"} {
		if strings.Contains(got, unwanted) {
			t.Errorf("no value submitted but a control renders %q:\n%s", unwanted, got)
		}
	}
}

func TestActiveFormCSRFHiddenField(t *testing.T) {
	fields := []view.ActiveField{{Name: "title", Label: "Title"}}

	with := render(t, view.ActiveForm{Fields: fields, CSRF: "token-1"}.Render())
	if !strings.Contains(with, `<input type="hidden" name="_csrf" value="token-1">`) {
		t.Errorf("default CSRF field missing:\n%s", with)
	}

	custom := render(t, view.ActiveForm{Fields: fields, CSRF: "token-2", CSRFName: "token"}.Render())
	if !strings.Contains(custom, `<input type="hidden" name="token" value="token-2">`) {
		t.Errorf("custom-named CSRF field missing:\n%s", custom)
	}

	none := render(t, view.ActiveForm{Fields: fields}.Render())
	if strings.Contains(none, "_csrf") {
		t.Errorf("empty token still renders the CSRF field:\n%s", none)
	}
}

func TestActiveFormEscapesDynamicValues(t *testing.T) {
	got := render(t, view.ActiveForm{
		Fields: []view.ActiveField{
			{Name: "title", Label: `<b>Title</b>`, Hint: `a "hint" & <i>`},
			{Name: "status", Label: "Status", Type: view.FieldSelect,
				Options: []view.FieldOption{{Value: `x">y`, Label: `<script>alert(1)</script>`}}},
		},
		Errors: &validators.Errors{Fields: map[string][]string{
			"title": {"<script>bad()</script>"},
		}},
		Values: map[string]string{"title": `<script>alert("x")</script>`, "status": `x">y`},
	}.Render())

	if strings.Contains(got, "<script>") {
		t.Fatalf("unescaped script tag in output:\n%s", got)
	}
	for _, want := range []string{
		`<label class="control-label" for="form-title">&lt;b&gt;Title&lt;/b&gt;</label>`,
		`value="&lt;script&gt;alert(&#34;x&#34;)&lt;/script&gt;"`,
		`<div class="hint-block">a &#34;hint&#34; &amp; &lt;i&gt;</div>`,
		`<div class="help-block">&lt;script&gt;bad()&lt;/script&gt;</div>`,
		`<option value="x&#34;&gt;y" selected>&lt;script&gt;alert(1)&lt;/script&gt;</option>`,
	} {
		if !strings.Contains(got, want) {
			t.Errorf("form missing escaped %q:\n%s", want, got)
		}
	}
}

func TestActiveFormCustomInputTypePassesThrough(t *testing.T) {
	got := render(t, view.ActiveForm{
		Fields: []view.ActiveField{{Name: "email", Label: "Email", Type: view.FieldType("email")}},
	}.Render())
	if !strings.Contains(got, `<input type="email" id="form-email" class="form-control" name="email" value="">`) {
		t.Errorf("custom type not passed through:\n%s", got)
	}
}
