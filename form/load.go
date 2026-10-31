package form

import (
	"context"
	"net/http"

	"github.com/goyii2/framework/validators"
)

// Load is the one-step form model entry: it binds the request body into dst
// exactly like Bind, then validates the model under scenario when its type
// implements validators.Ruled, and reports both stages as one
// *validators.Errors — nil when every submitted value parsed and every rule
// declared for the scenario passed. Validation runs on the model as bound,
// so a field whose value failed to parse still faces its rules: within one
// field the parse messages come first and the rule messages after, a fixed
// order tests can rely on. A model without Rules only binds; a scenario with
// no schema in the RuleSet has nothing to fail. The returned failure carries
// the scenario Load was asked for, and record-level failures — a body that
// cannot be decoded, a struct-level rule — keep the "$root" key.
func Load(ctx context.Context, r *http.Request, dst any, scenario validators.Scenario) *validators.Errors {
	bindErrs := Bind(r, dst)
	validateErrs := validators.Validate(ctx, dst, scenario)
	fields := mergeFields(bindErrs, validateErrs)
	if len(fields) == 0 {
		return nil
	}
	return &validators.Errors{Scenario: scenario, Fields: fields}
}

// mergeFields unions the Fields of the binding and validation failures into
// one map: a field present in both keeps its parse messages ahead of its
// rule messages, and a stage that passed contributes nothing.
func mergeFields(bind, validate *validators.Errors) map[string][]string {
	var fields map[string][]string
	for _, failure := range [...](*validators.Errors){bind, validate} {
		if failure == nil {
			continue
		}
		if fields == nil {
			fields = make(map[string][]string, len(failure.Fields))
		}
		for field, messages := range failure.Fields {
			fields[field] = append(fields[field], messages...)
		}
	}
	return fields
}
