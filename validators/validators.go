package validators

import (
	"context"
	"fmt"
	"maps"
	"slices"
	"strings"

	z "github.com/Oudwins/zog"

	"github.com/goyii2/framework/i18n"
)

// Scenario labels the validation context a model runs under: the same model
// validates different rule sets in different situations. Insert and Update
// are the scenarios the save chain triggers automatically; any other string
// (e.g. Scenario("login")) is a custom scenario the application validates
// with explicitly.
type Scenario string

const (
	// ScenarioInsert validates a record about to be created — the save chain
	// selects it when the primary key is zero.
	ScenarioInsert Scenario = "insert"

	// ScenarioUpdate validates a record about to be modified — the save chain
	// selects it when the primary key is set.
	ScenarioUpdate Scenario = "update"
)

// RuleSet maps each scenario to the zog struct schema that validates it. A
// scenario absent from the set is treated as having no rules: validation
// passes for it.
type RuleSet map[Scenario]*z.StructSchema

// Ruled is the contract a model implements to take part in validation. The
// save chain detects it on records being written and refuses to persist ones
// whose schema rejects them; models not implementing Rules are stored without
// any validation overhead.
type Ruled interface {
	Rules() RuleSet
}

// Errors is the validation failure produced by Validate and returned by
// ar.Model.Save when a record's schema rejects it. Fields holds the messages
// keyed by field path; issues attached to the record as a whole rather than a
// single field land under the "$root" key (zog's convention for empty paths).
type Errors struct {
	// Scenario is the scenario that was validated.
	Scenario Scenario
	// Fields maps each failing field to its messages, in report order.
	Fields map[string][]string
}

// Error renders the failure as one line: the scenario followed by each
// field's messages. Fields appear in sorted order so the text is stable.
func (e *Errors) Error() string {
	var sb strings.Builder
	fmt.Fprintf(&sb, "validators: %s scenario rejected:", e.Scenario)
	for _, field := range slices.Sorted(maps.Keys(e.Fields)) {
		fmt.Fprintf(&sb, " %s: %s;", field, strings.Join(e.Fields[field], ", "))
	}
	return sb.String()
}

// MessageCategory is the translation category validation messages are
// looked up under: the message a rule produces (its z.Message text, or the
// rendered zog default) doubles as the message id. Configure before serving;
// reads are unsynchronized.
var MessageCategory = "app"

// Validate checks model — a pointer to a model struct — against the schema
// its Rules method declares for scenario, returning the collected field
// messages. It returns nil when the record is valid, when the model
// implements no Rules, and when the RuleSet carries no schema for the
// scenario (no rules configured means nothing to fail).
//
// Each message is then translated into the language resolved from ctx: the
// message id is looked up in the MessageCategory category of the context's
// i18n source, and a miss keeps the original text. Contexts carrying no
// message source (CLI, standalone tools) leave every message as-is.
func Validate(ctx context.Context, model any, scenario Scenario) *Errors {
	ruled, ok := model.(Ruled)
	if !ok || model == nil {
		return nil
	}
	schema := ruled.Rules()[scenario]
	if schema == nil {
		return nil
	}
	issues := schema.Validate(model)
	if len(issues) == 0 {
		return nil
	}
	fields := z.Issues.Flatten(issues)
	translateMessages(ctx, fields)
	return &Errors{Scenario: scenario, Fields: fields}
}

// translateMessages runs every field message through the i18n lookup for
// the context's language. The message is the id; an id with no translation
// stays verbatim. Without a message source in the context (nil context, CLI,
// standalone tools) nothing changes.
func translateMessages(ctx context.Context, fields map[string][]string) {
	if ctx == nil {
		return
	}
	if _, ok := i18n.SourceFromContext(ctx); !ok {
		return
	}
	for field, messages := range fields {
		translated := make([]string, len(messages))
		for i, msg := range messages {
			translated[i] = i18n.T(ctx, MessageCategory, msg, nil)
		}
		fields[field] = translated
	}
}
