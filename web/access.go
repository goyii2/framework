package web

// AccessChecker is the authorization backend of the request pipeline: the
// user component and access control ask it whether an identity holds an
// authorization item (a role or permission name). The method shape matches
// the framework's rbac manager — Params there is an alias of map[string]any
// — so a *rbac.DbManager plugs in as-is, with no adapter:
//
//	auth, _ := rbac.NewDbManager(conn.DB, rules...)
//	handler.SetAuthManager(auth)
//
// CheckAccess returns an error when the check could not run (missing
// schema, misaligned rule registry); false without error is a clean denial.
type AccessChecker interface {
	CheckAccess(userID string, itemName string, params map[string]any) (bool, error)
}

// Can reports whether the current identity holds the named authorization
// item, asking the installed AccessChecker. Guests hold nothing, and so do
// applications without a checker — the plain role callback of identity-based
// access control keeps working in that case. A backend error denies as
// well; the middleware form of the check (AccessControl with Auth) surfaces
// errors as 500s instead, so use that when failures must be loud.
func (u *User) Can(name string) bool {
	return u.CanWithParams(name, nil)
}

// CanWithParams is Can with the params the item's rules evaluate — typically
// the subject of the action ("post" -> the post about to be edited).
func (u *User) CanWithParams(name string, params map[string]any) bool {
	if u == nil || u.checker == nil {
		return false
	}
	identity := u.Identity()
	if identity == nil {
		return false
	}
	ok, err := u.checker.CheckAccess(identity.GetID(), name, params)
	return err == nil && ok
}
