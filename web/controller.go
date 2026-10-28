package web

// Controller is the embed point for application controllers:
//
//	type PostController struct{ web.Controller }
//
// Handler methods named with the Action prefix (ActionView, ActionDelete)
// turn the struct into a controller whose methods are routable actions.
// Capabilities grow with the routing and view layers; at this stage the
// type marks controller classes and reserves the embedding slot.
type Controller struct{}

// Action is the controller action signature: it receives the request
// context, writes the response through it (JSON, redirect, later views) and
// returns an error to hand over to the unified error handler — nil means
// the response is complete.
type Action func(ctx *Context) error
