// Package web provides the request/response context, Controller/Action,
// the application HTTP entry point with explicit routing (method helpers
// with typed path parameters, named routes, module prefixes and the
// /{controller}/{action} convention fallback), the unified error handler,
// the middleware plumbing (application-wide chain, controller-level mounts
// and the filter family: verb control, CORS, content negotiation, CSRF,
// rate limiting, conditional caching, role-based access control, HTTP
// basic/bearer authentication), and the authentication core (Identity,
// request-scoped User with session login/logout, and the session with file
// and redis backends behind the session cookie); it corresponds to Yii2's
// yii\web subsystem.
package web
