// Package i18n provides message translation and date/number/currency
// formatting; it corresponds to Yii2's yii\i18n subsystem.
//
// Translations live in JSON files, one per language and category, keyed by
// the message id — the string in the source language, exactly as it appears
// in code:
//
//	messages/zh-CN/app.json:
//	{
//	    "Hello, {name}!": "你好，{name}！",
//	    "You have {n} new message.": {
//	        "one": "您有 {n} 条新消息。",
//	        "other": "您有 {n} 条新消息。"
//	    }
//	}
//
// Applications embed the directory and install the source on the
// application; requests then resolve their language automatically (query
// parameter, session, Accept-Language, application default — in that order):
//
//	//go:embed messages
//	var messages embed.FS
//
//	app := goyii.New(cfg)
//	app.SetI18n(i18n.NewMessageSource(messages))
//
//	func (c *SiteController) ActionHello(ctx *web.Context) error {
//	    msg := i18n.T(ctx.Context(), "app", "Hello, {name}!", map[string]any{"name": "Ada"})
//	    price := i18n.F(ctx.Context()).Currency("USD", 1234.5)
//	    _ = price // "$ 1,234.50" under en-US
//	    return ctx.JSON(msg)
//	}
//
// The same lookup translates validation messages: the message a model's
// rules produce is the id, so messages/zh-CN/app.json translating
// "name cannot be blank" localizes every 422 response for that language.
// Values may instead be an object of plural forms ("one", "other", ...);
// the numeric param "n" selects the form by the language's CLDR cardinal
// rules.
//
// Formatting (dates, numbers, currencies) is based on golang.org/x/text;
// date shapes come with a per-language layout table — English, Chinese,
// Japanese, Korean, German and French out of the box, others fall back to
// English.
package i18n
