// CacheController: the PageCache filter demonstration — its one page
// renders a generation timestamp, and the whole response (status,
// Content-Type, body) is replayed from the cache while the entry lives.
package controllers

import (
	"net/http"
	"time"

	"github.com/goyii2/framework/examples/basic/views"
	"github.com/goyii2/framework/i18n"
	"github.com/goyii2/framework/web"
)

// CacheController demonstrates the PageCache filter: its one page renders a
// generation timestamp, and the whole response — status, Content-Type, body
// — is replayed from the cache while the entry lives.
type CacheController struct {
	web.Controller
}

// pageCacheTTL bounds how long a rendered demo page stays cached. Short on
// purpose: the second request inside the window replays the very same
// timestamp, and once the entry expires the page renders fresh again.
const pageCacheTTL = 3 * time.Second

// Middleware caches the controller's pages with web.PageCache. VaryBy keys
// the entry by the two request-dependent parts the page frame carries — the
// display language and the signed-in account — so a copy cached for one
// visitor never replays to another.
func (cc *CacheController) Middleware() []web.Middleware {
	return []web.Middleware{web.PageCache{
		Duration: pageCacheTTL,
		VaryBy:   pageCacheVary,
	}.Middleware()}
}

// pageCacheVary derives the cache-key segment of one request: language plus
// user id (guests share one variant). It runs inside the pipeline, so the
// request Context carries both.
func pageCacheVary(r *http.Request) string {
	c := web.FromContext(r.Context())
	if c == nil {
		return ""
	}
	lang, _ := i18n.LanguageFromContext(c.Context())
	user := "guest"
	if c.User() != nil {
		if id := c.User().Identity(); id != nil {
			user = id.GetID()
		}
	}
	return lang + "|" + user
}

// ActionDemo renders the page-cache demonstration page. The rendered-at
// timestamp is the observable: it freezes while a cached copy is replayed
// and moves again once the entry expired.
func (cc *CacheController) ActionDemo(c *web.Context) error {
	generated := time.Now().Format(time.RFC3339Nano)
	ctx := c.Context()
	c.SetTitle(i18n.T(ctx, "app", "Page cache", nil))
	return c.Render("site/pagecache", views.PageCacheData{
		GeneratedAt: generated,
		TTL:         pageCacheTTL.String(),
	})
}
