// Package validators provides scenario-based model validation on top of the
// zog schema library; it corresponds to Yii2's yii\validators subsystem.
//
// A model participates by implementing Rules — one method returning a RuleSet,
// a scenario-to-schema mapping:
//
//	type User struct {
//	    ar.Model
//	    ID    int64
//	    Name  string
//	    Email string
//	}
//
//	var insertSchema = z.Struct(z.Shape{
//	    "name":  z.String().Required(z.Message("name cannot be blank")),
//	    "email": z.String().Email(z.Message("email is not a valid address")),
//	})
//	var updateSchema = z.Struct(z.Shape{
//	    "email": z.String().Email(z.Message("email is not a valid address")),
//	})
//
//	func (u *User) Rules() validators.RuleSet {
//	    return validators.RuleSet{
//	        validators.ScenarioInsert: insertSchema,
//	        validators.ScenarioUpdate: updateSchema,
//	    }
//	}
//
// Validation returns the per-field message list Yii2 developers expect:
// map[field][]message, empty when the record is valid. Two call paths exist:
// directly through Validate (or the ar.Model.Validate method it powers), and
// implicitly through the save chain — ar.Model.Save validates the record with
// the insert or update schema (decided by the primary key) and refuses to
// write when validation fails. Models without Rules skip validation entirely.
//
// Messages translate through the context's i18n component: Validate takes
// the request context as its first argument, and each produced message is
// the translation id — put "name cannot be blank" into a language's app
// category file and every 422 reply in that language carries the
// translation. Contexts without an i18n source keep the source-language
// text.
//
// Schema keys name exported struct fields (a leading lowercase letter matches
// the corresponding capitalized field, so "name" addresses Name); a key with
// no matching field is a programming error and panics, mirroring zog.
//
// Message templates: z.Message / z.MessageFunc on the individual test is the
// per-rule override point; the global default table lives in
// conf.DefaultIssueMessageMap (github.com/Oudwins/zog/conf) and can be
// replaced wholesale or per issue code. Whatever text a rule produces — its
// own z.Message or the rendered default — is then looked up in the i18n
// app category for the request's language (see Validate); a miss keeps the
// text verbatim.
//
// Wiring a validation failure back to an API client:
//
//	func (c *UserController) ActionCreate(ctx *web.Context) error {
//	    user := ar.New[User](ctx.Context()).New()
//	    // ... fill fields from the request ...
//	    if err := user.Save(); err != nil {
//	        var verrs *validators.Errors
//	        if errors.As(err, &verrs) {
//	            return ctx.JSONStatus(http.StatusUnprocessableEntity, verrs.Fields)
//	        }
//	        return err
//	    }
//	    return ctx.JSON(user)
//	}
package validators
