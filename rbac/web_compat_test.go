package rbac_test

import (
	"testing"

	"github.com/goyii2/framework/rbac"
	"github.com/goyii2/framework/web"
)

// TestDbManagerSatisfiesWebAccessChecker pins the plug-in compatibility at
// compile time: the database manager's CheckAccess carries exactly the
// signature the web pipeline's authorization seam declares, so
// handler.SetAuthManager(manager) needs no adapter. Params being a type
// alias of map[string]any is what keeps the two in step.
func TestDbManagerSatisfiesWebAccessChecker(t *testing.T) {
	var _ web.AccessChecker = (*rbac.DbManager)(nil)
	var _ web.AccessChecker = rbac.AuthManager(nil)
}
