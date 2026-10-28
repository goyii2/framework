package web

// Controller is the embed point for application controllers:
//
//	type PostController struct{ web.Controller }
//
// Handler methods named with the Action prefix (ActionView, ActionDelete)
// turn the struct into a controller whose methods are routable actions. A
// controller may optionally declare middleware by implementing
//
//	Middleware() []Middleware
//
// — it then wraps only that controller's actions, mounted explicitly
// through Handler.Ctrl or resolved by the convention fallback, and runs
// after the application-wide chain (see middleware.go).
type Controller struct{}

// Action is the controller action signature: it receives the request
// context, writes the response through it (JSON, redirect, later views) and
// returns an error to hand over to the unified error handler — nil means
// the response is complete.
type Action func(ctx *Context) error
