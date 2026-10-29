// The browser-facing actions: pages built on the templ view layer, the
// data provider, the widgets and the RBAC manager. They live beside the
// JSON API actions of controllers.go and share the same models and access
// rules; the write pages additionally gate on the RBAC permissions the seed
// grants through the role hierarchy (editor writes, admin also deletes).
// The P2 face rides here too: the listing renders through the GridView
// widget (sortable headers + filter row), the create/edit forms through
// ActiveForm, and the queue and page-cache demo pages close the loop with
// the worker and cache components.
package main

import (
	"context"
	"errors"
	"net/http"
	"strconv"
	"strings"
	"time"

	"gorm.io/gorm"

	"github.com/goyii2/framework/dataprovider"
	"github.com/goyii2/framework/db/ar"
	"github.com/goyii2/framework/i18n"
	"github.com/goyii2/framework/mail"
	"github.com/goyii2/framework/url"
	"github.com/goyii2/framework/validators"
	"github.com/goyii2/framework/view"
	"github.com/goyii2/framework/web"
)

// sortablePostFields is the whitelist the listing accepts sort requests
// for. A request naming anything else answers 400 before any SQL is built.
var sortablePostFields = []string{"id", "title", "status", "created_at"}

// listPerPage is the page size of the browser listing.
const listPerPage = 10

// translateIssues maps a field -> messages set through the message source,
// so messages an inline zog schema produced localize the same way model
// rules do (the automatic translation sits in the model save pipeline).
func translateIssues(ctx context.Context, fields map[string][]string) map[string][]string {
	out := make(map[string][]string, len(fields))
	for field, msgs := range fields {
		translated := make([]string, len(msgs))
		for i, msg := range msgs {
			translated[i] = i18n.T(ctx, "app", msg, nil)
		}
		out[field] = translated
	}
	return out
}

// ActionHome is the browser front page: an introduction, the entry links
// and the mailer demonstration button.
func (site *SiteController) ActionHome(c *web.Context) error {
	c.SetTitle(i18n.T(c.Context(), "app", "Home", nil))
	return c.Render("site/home", homeData{
		SignedIn:  c.User() != nil && c.User().Identity() != nil,
		PostsURL:  url.To("post/list", nil),
		LoginURL:  url.To("site/login", nil),
		NotifyURL: url.To("site/notify", nil),
		CSRF:      c.CsrfToken(),
	})
}

// ActionLoginForm renders the login page. The form posts back to /login,
// carrying the CSRF token in the hidden field the middleware verifies.
func (site *SiteController) ActionLoginForm(c *web.Context) error {
	c.SetTitle(i18n.T(c.Context(), "app", "Login", nil))
	return c.Render("site/login", loginData{
		Action: url.To("site/login", nil),
		CSRF:   c.CsrfToken(),
	})
}

// ActionNotify demonstrates the mail component: it composes a message with
// the chain builder and hands it to the application's mailer. The default
// log driver records the complete message in the application log — the
// development counterpart of an SMTP delivery.
func (site *SiteController) ActionNotify(c *web.Context) error {
	recipient := "guest@example.com"
	if identity := c.User().Identity(); identity != nil {
		recipient = identity.GetName() + "@example.com"
	}
	subject := i18n.T(c.Context(), "app", "A test email from {app}",
		map[string]any{"app": c.App().Config().App.Name})
	body := i18n.T(c.Context(), "app", "This message was composed by the mail example of the basic application.", nil)
	msg := mail.NewMessage().
		SetFrom("goyii basic <no-reply@goyii.example>").
		SetTo(recipient).
		SetSubject(subject).
		SetTextBody(body)
	if err := c.App().Mailer().Send(c.Context(), msg); err != nil {
		return err
	}
	c.SetTitle(i18n.T(c.Context(), "app", "Mail sent", nil))
	return c.Render("site/notify", notifyData{Recipient: recipient, HomeURL: url.To("site/home", nil)})
}

// requirePermission gates a page action on an RBAC item: guests are sent to
// the login page (the redirect answered the request, so the returned error
// is nil), signed-in requesters without the item get the 403 error page.
// Actions return the result directly — an action's own return value is what
// the pipeline renders.
func requirePermission(c *web.Context, item string) error {
	if c.User() == nil || c.User().IsGuest() {
		return c.Redirect(http.StatusFound, url.To("site/login", nil))
	}
	if !c.User().Can(item) {
		return web.Forbidden("you are not allowed to perform this action")
	}
	return nil
}

// can reports whether the current user holds an RBAC item (false for
// guests); for menu and button visibility.
func can(c *web.Context, item string) bool {
	return c.User() != nil && c.User().Can(item)
}

// postID formats a row id for URL parameters.
func postID(post *Post) string { return strconv.FormatInt(post.ID, 10) }

// likeEscapeClause is the escape declaration every LIKE of the listing
// search carries; the pattern builder below escapes with the same character.
const likeEscapeClause = ` ESCAPE '\'`

// likeEscaper neutralizes the LIKE wildcards (and the escape character
// itself) in one pass, so each character the user typed keeps its literal
// meaning inside the pattern and cannot reshape the match.
var likeEscaper = strings.NewReplacer(`\`, `\\`, `%`, `\%`, `_`, `\_`)

// likePattern wraps raw input into a contains-match pattern whose interior
// is fully escaped: "%"/"_" match only themselves, exactly like the rest of
// the text. Bound as a statement parameter — never concatenated.
func likePattern(raw string) string {
	return "%" + likeEscaper.Replace(raw) + "%"
}

// postFilters is the filter state of the listing, read from the query
// string the GridView filter row submits: a contains-match on the title and
// an exact match on the status.
type postFilters struct {
	title  string
	status string
}

// active reports whether any filter narrows the listing.
func (f postFilters) active() bool { return f.title != "" || f.status != "" }

// readPostFilters collects the filter inputs of one request.
func readPostFilters(r *http.Request) postFilters {
	return postFilters{
		title:  strings.TrimSpace(r.URL.Query().Get("title")),
		status: r.URL.Query().Get("status"),
	}
}

// apply narrows the listing query by the filters: the title pattern is fully
// escaped (no user wildcard reshapes the match), the status compares
// exactly. Both ride the base query so the count, the page fetch and every
// link stay consistent with them.
func (f postFilters) apply(query *ar.Query[Post]) *ar.Query[Post] {
	if f.title != "" {
		query = query.Where("title LIKE ?"+likeEscapeClause, likePattern(f.title))
	}
	if f.status != "" {
		query = query.Where("status = ?", f.status)
	}
	return query
}

// ActionList is the browser listing: an ActiveDataProvider pushes the
// pagination and the whitelisted sort down to SQL, the GridView widget
// renders the table — sortable column headers and the filter row — and the
// LinkPager renders the paging bar. Filtering splits the widget's way: the
// grid renders the inputs and echoes their values, the action assembles the
// query they describe before Prepare.
func (pc *PostController) ActionList(c *web.Context) error {
	params, err := dataprovider.FromRequest(c.Request(), sortablePostFields...)
	if err != nil {
		return web.BadRequest(err.Error())
	}
	if c.Request().URL.Query().Get("per-page") == "" {
		// The page fixes its own page size; an explicit per-page from the
		// query string still wins (the parser already clamped it).
		params.PerPage = listPerPage
	}

	filters := readPostFilters(c.Request())
	res, err := dataprovider.ActiveDataProvider[Post]{
		Query:        filters.apply(ar.New[Post](c.Context()).With("Author")),
		Params:       params,
		DefaultOrder: []dataprovider.Sort{{Column: "id", Desc: true}},
	}.Prepare()
	if err != nil {
		return err
	}

	ctx := c.Context()
	t := func(msgID string) string { return i18n.T(ctx, "app", msgID, nil) }
	format := i18n.F(ctx)
	canUpdate := can(c, "post/update")

	// keep returns the query-string parameters that must survive a sort
	// toggle or a pager click: the active filters.
	keep := func(p url.P) url.P {
		if filters.title != "" {
			p["title"] = filters.title
		}
		if filters.status != "" {
			p["status"] = filters.status
		}
		return p
	}
	sortParam := c.Request().URL.Query().Get("sort")
	viewURL := func(post *Post) string {
		return url.To("post/show", url.P{"id": postID(post)})
	}

	columns := []view.DataColumn{
		{
			Attr: "id", Label: t("ID"), Sortable: true,
			Value: func(item any) any { return postID(item.(*Post)) },
		},
		{
			Attr: "title", Label: t("Title"), Sortable: true,
			Filter: &view.ColumnFilter{},
			Value: func(item any) any {
				post := item.(*Post)
				return linkCell(viewURL(post), post.Title)
			},
		},
		{
			Attr: "status", Label: t("Status"), Sortable: true,
			Filter: &view.ColumnFilter{Type: view.FieldSelect, Options: []view.FieldOption{
				{Value: "", Label: t("All")},
				{Value: PostStatusDraft, Label: t(PostStatusDraft)},
				{Value: PostStatusPublished, Label: t(PostStatusPublished)},
			}},
			Value: func(item any) any { return t(item.(*Post).Status) },
		},
		{
			Attr: "author", Label: t("Author"),
			Value: func(item any) any {
				if post := item.(*Post); post.Author != nil {
					return post.Author.Username
				}
				return nil
			},
		},
		{
			Attr: "created_at", Label: t("Created at"), Sortable: true,
			Value: func(item any) any { return format.DateTime(item.(*Post).CreatedAt) },
		},
		{
			Attr: "actions", Label: t("Actions"),
			Value: func(item any) any {
				if !canUpdate {
					return nil
				}
				post := item.(*Post)
				return linkCell(url.To("post/edit", url.P{"id": postID(post)}), t("Edit"))
			},
		},
	}
	emptyText := t("No posts yet.")
	if filters.active() {
		emptyText = t("No posts match your filters.")
	}
	grid := view.GridView{
		Columns:      columns,
		ItemCount:    len(res.Items),
		Item:         func(i int) any { return res.Items[i] },
		Sort:         sortParam,
		SortURL:      func(sortValue string) string { return url.To("post/list", keep(url.P{"sort": sortValue})) },
		FilterValues: view.QueryValues(c.Request(), "title", "status"),
		EmptyText:    emptyText,
	}.Render()

	// The pager keeps the active sort and filters across pages; the numbered
	// buttons address pages, the sort toggles restart from page one.
	pager := view.LinkPager{
		Pagination:     res.Pagination(),
		MaxButtonCount: 5,
		URL: func(page int) string {
			p := url.P{"page": strconv.Itoa(page)}
			if sortParam != "" {
				p["sort"] = sortParam
			}
			return url.To("post/list", keep(p))
		},
	}.Render()

	c.SetTitle(i18n.T(ctx, "app", "Posts", nil))
	return c.Render("post/list", postsListData{
		Grid:      grid,
		Pager:     pager,
		Total:     res.TotalCount,
		CanCreate: can(c, "post/create"),
		CreateURL: url.To("post/new", nil),
	})
}

// findPost loads the post addressed by the id path parameter with its
// author eager-loaded; a missing row answers the negotiated 404.
func (pc *PostController) findPost(c *web.Context) (*Post, error) {
	id, err := c.ParamInt64("id")
	if err != nil {
		return nil, web.NotFound("post not found")
	}
	post, err := ar.New[Post](c.Context()).With("Author").Where("id = ?", id).One()
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, web.NotFound("post not found")
	}
	if err != nil {
		return nil, err
	}
	return post, nil
}

// ActionShow renders the detail page: the DetailView widget over explicit
// rows, the breadcrumbs trail, and the action buttons the RBAC manager
// allows (edit for post/update holders, delete for admin through the
// hierarchy).
func (pc *PostController) ActionShow(c *web.Context) error {
	post, err := pc.findPost(c)
	if err != nil {
		return err
	}
	ctx := c.Context()
	t := func(msgID string) string { return i18n.T(ctx, "app", msgID, nil) }
	format := i18n.F(ctx)
	author := ""
	if post.Author != nil {
		author = post.Author.Username
	}
	detail := view.DetailView{Rows: []view.DetailRow{
		{Label: t("ID"), Value: postID(post)},
		{Label: t("Title"), Value: post.Title},
		{Label: t("Status"), Value: t(post.Status)},
		{Label: t("Author"), Value: author},
		{Label: t("Created at"), Value: format.DateTime(post.CreatedAt)},
		{Label: t("Updated at"), Value: format.DateTime(post.UpdatedAt)},
	}}.Render()

	c.SetTitle(post.Title)
	return c.Render("post/show", postShowData{
		Breadcrumbs: breadcrumbs(ctx, post.Title),
		Detail:      detail,
		Title:       post.Title,
		CanUpdate:   can(c, "post/update"),
		CanDelete:   can(c, "post/delete"),
		EditURL:     url.To("post/edit", url.P{"id": postID(post)}),
		DeleteURL:   url.To("post/destroy", url.P{"id": postID(post)}),
		BackURL:     url.To("post/list", nil),
		CSRF:        c.CsrfToken(),
	})
}

// ActionNew renders the create form; guests go to the login page, accounts
// without post/create see the 403 page.
func (pc *PostController) ActionNew(c *web.Context) error {
	if err := requirePermission(c, "post/create"); err != nil {
		return err
	}
	return renderPostForm(c, false, http.StatusOK, postFormInput{status: PostStatusDraft}, nil)
}

// ActionStore receives the create form. The CSRF middleware verified the
// submission; the save pipeline runs the insert scenario and a rejected
// payload re-renders the form under 422 with the messages and the entered
// values echoed back.
func (pc *PostController) ActionStore(c *web.Context) error {
	if err := requirePermission(c, "post/create"); err != nil {
		return err
	}
	author, ok := c.User().Identity().(*User)
	if !ok || author == nil {
		return web.Unauthorized("authentication required")
	}
	input := readPostForm(c)
	post := ar.New[Post](c.Context()).New()
	post.Title, post.Body, post.Status = input.title, input.body, input.status
	post.AuthorID = author.ID
	if err := post.Save(); err != nil {
		return replayPostForm(c, false, input, err)
	}
	return c.Redirect(http.StatusFound, url.To("post/show", url.P{"id": postID(post)}))
}

// ActionEdit renders the update form with the record's current values.
func (pc *PostController) ActionEdit(c *web.Context) error {
	if err := requirePermission(c, "post/update"); err != nil {
		return err
	}
	post, err := pc.findPost(c)
	if err != nil {
		return err
	}
	return renderPostForm(c, true, http.StatusOK, postFormInput{title: post.Title, body: post.Body, status: post.Status}, nil)
}

// ActionSave receives the update form; like the create form it re-renders
// with 422 and the entered values when the update scenario rejects the
// payload.
func (pc *PostController) ActionSave(c *web.Context) error {
	if err := requirePermission(c, "post/update"); err != nil {
		return err
	}
	post, err := pc.findPost(c)
	if err != nil {
		return err
	}
	input := readPostForm(c)
	post.Title, post.Body, post.Status = input.title, input.body, input.status
	if err := post.Save(); err != nil {
		return replayPostForm(c, true, input, err)
	}
	return c.Redirect(http.StatusFound, url.To("post/show", url.P{"id": postID(post)}))
}

// ActionDestroy receives the delete button of the detail page. Only the
// admin hierarchy reaches it; everyone else sees the 403 page.
func (pc *PostController) ActionDestroy(c *web.Context) error {
	if err := requirePermission(c, "post/delete"); err != nil {
		return err
	}
	post, err := pc.findPost(c)
	if err != nil {
		return err
	}
	if err := post.Delete(); err != nil {
		return err
	}
	return c.Redirect(http.StatusFound, url.To("post/list", nil))
}

// postFormInput carries the form field values between reading, saving and
// re-rendering.
type postFormInput struct {
	title  string
	body   string
	status string
}

// readPostForm reads the submitted form fields.
func readPostForm(c *web.Context) postFormInput {
	r := c.Request()
	return postFormInput{
		title:  r.PostFormValue("title"),
		body:   r.PostFormValue("body"),
		status: r.PostFormValue("status"),
	}
}

// renderPostForm renders the shared form for create and edit; editing picks
// the heading and targets the row's save route instead of the create route.
// status is 200 for a fresh form and 422 when a rejected submission
// re-renders with its messages. The ActiveForm widget does the field
// rendering: labels, controls, and — on a 422 — the validators messages
// under the failing fields with the submitted values refilled.
func renderPostForm(c *web.Context, editing bool, status int, input postFormInput, verrs *validators.Errors) error {
	ctx := c.Context()
	t := func(msgID string) string { return i18n.T(ctx, "app", msgID, nil) }
	heading := t("New post")
	action := url.To("post/store", nil)
	if editing {
		heading = t("Edit post")
		action = url.To("post/save", url.P{"id": c.Param("id")})
	}
	form := view.ActiveForm{
		ID:     "post-form",
		Action: action,
		Fields: []view.ActiveField{
			{Name: "title", Label: t("Title"), Type: view.FieldText, Required: true, MaxLength: 200},
			{Name: "status", Label: t("Status"), Type: view.FieldSelect, Options: []view.FieldOption{
				{Value: PostStatusDraft, Label: t(PostStatusDraft)},
				{Value: PostStatusPublished, Label: t(PostStatusPublished)},
			}},
			{Name: "body", Label: t("Body"), Type: view.FieldTextarea},
		},
		Errors: verrs,
		Values: map[string]string{
			"title":  input.title,
			"status": input.status,
			"body":   input.body,
		},
		CSRF: c.CsrfToken(),
	}
	c.SetTitle(heading)
	return c.RenderStatus(status, "post/form", postFormData{
		Breadcrumbs: breadcrumbs(ctx, heading),
		Heading:     heading,
		Form:        form.Render(),
		CancelURL:   url.To("post/list", nil),
	})
}

// replayPostForm re-renders the form after a rejected save: validation
// failures come back as the 422 error set (messages already translated by
// the save pipeline's flatten boundary), anything else is a real error.
func replayPostForm(c *web.Context, editing bool, input postFormInput, err error) error {
	var verrs *validators.Errors
	if !errors.As(err, &verrs) {
		return err
	}
	return renderPostForm(c, editing, http.StatusUnprocessableEntity, input, verrs)
}

// QueueController serves the queue demo page: the live state of the queue
// table, a button that pushes a demo email job, and the worker commands that
// drain it. The push is the producer side of the demo; the consumption runs
// out-of-band through `goyii queue run` or `queue listen` — the page links
// to the records those runs leave in the application log.
type QueueController struct {
	web.Controller
}

// ActionIndex renders the queue page. A just-pushed job id arrives through
// the pushed query parameter (the push action redirects back post/redirect/
// get) and confirms the enqueue.
func (qc *QueueController) ActionIndex(c *web.Context) error {
	ctx := c.Context()
	status, err := readQueueStatus(ctx)
	if err != nil {
		return err
	}
	t := func(msgID string) string { return i18n.T(ctx, "app", msgID, nil) }
	c.SetTitle(t("Queue"))
	return c.Render("queue/index", queuePageData{
		Waiting:  strconv.FormatInt(status.Waiting, 10),
		Reserved: strconv.FormatInt(status.Reserved, 10),
		Failed:   strconv.FormatInt(status.Failed, 10),
		PushedID: c.Request().URL.Query().Get("pushed"),
		PushURL:  url.To("queue/push", nil),
		CSRF:     c.CsrfToken(),
	})
}

// ActionPush enqueues one demo email job. The recipient is optional (the
// signed-in account's address is the default); the push redirects back to
// the queue page carrying the message id, so the waiting count ticks up
// visibly before a worker drains it.
func (qc *QueueController) ActionPush(c *web.Context) error {
	recipient := strings.TrimSpace(c.Request().PostFormValue("to"))
	if recipient == "" {
		recipient = "guest@example.com"
		if identity := c.User().Identity(); identity != nil {
			recipient = identity.GetName() + "@example.com"
		}
	}
	job := &EmailJob{
		To:      recipient,
		Subject: "A queued notification from the basic demo",
		Body:    "This message was pushed from the queue page and delivered by a queue worker.",
	}
	id, err := c.App().Queue().Push(c.Context(), job)
	if err != nil {
		return err
	}
	c.App().Log().Info("demo job pushed", "job", job.Name(), "id", id, "to", recipient)
	return c.Redirect(http.StatusFound,
		url.To("queue/index", url.P{"pushed": strconv.FormatInt(id, 10)}))
}

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
	return c.Render("site/pagecache", pageCacheData{
		GeneratedAt: generated,
		TTL:         pageCacheTTL.String(),
	})
}
