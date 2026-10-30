package web

import "github.com/goyii2/framework/session"

// Identity is the contract every authenticated party fulfills. It lives in
// the session package (where the stored login state is resolved) and is
// re-exported here — the web plane's authentication middlewares, the User
// component and access control all speak it.
type Identity = session.Identity
