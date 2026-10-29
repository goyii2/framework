// PostController: the business CRUD. The JSON API actions exercise the
// AccessControl rules, scenario validation on save and the dependency-cached
// list endpoint; the browser pages render through the P2 face — the GridView
// listing (sortable headers + filter row) and the ActiveForm create/edit
// forms. The write pages gate on the RBAC permissions the seed grants
// through the role hierarchy (editor writes, admin also deletes).
package main

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"strconv"
	"strings"
	"time"

	"gorm.io/gorm"

	"github.com/goyii2/framework/base"
	"github.com/goyii2/framework/cache"
	"github.com/goyii2/framework/dataprovider"
	"github.com/goyii2/framework/db/ar"
	"github.com/goyii2/framework/i18n"
	"github.com/goyii2/framework/url"
	"github.com/goyii2/framework/validators"
	"github.com/goyii2/framework/view"
	"github.com/goyii2/framework/web"
)

// PostController serves the business CRUD. Its access rules are the
// AccessControl demonstration: reading is public (guest "?" and logged-in
// "@" alike), writing needs a login, deleting asks the RBAC manager for the
// post/delete permission (granted through the admin role's hierarchy, not
// through the identity's own role column), and anything no rule covers is
// denied.
type PostController struct {
	web.Controller

	// Auth is the RBAC manager deciding the concrete role names below.
	Auth web.AccessChecker
}

func (pc *PostController) Middleware() []web.Middleware {
	return []web.Middleware{
		web.AccessControl{Auth: pc.Auth, Rules: []web.AccessRule{
			{Allow: true, Roles: []string{"?", "@"}, Verbs: []string{http.MethodGet, http.MethodHead}},
			{Allow: true, Roles: []string{"@"}, Verbs: []string{http.MethodPost, http.MethodPut}},
			{Allow: true, Roles: []string{"post/delete"}, Verbs: []string{http.MethodDelete}},
		}}.Middleware(),
	}
}

// sortablePostFields is the whitelist the listing accepts sort requests
// for. A request naming anything else answers 400 before any SQL is built.
var sortablePostFields = []string{"id", "title", "status", "created_at"}

// listPerPage is the page size of the browser listing.
const listPerPage = 10

// postListCacheKey is the cache key of the list endpoint.
const postListCacheKey = "post-list"

// ActionIndex lists the newest posts. The list is cached for one minute
// behind a value dependency on the table's max id: creating or deleting a
// post changes the dependency, the entry is judged stale and rebuilt on the
// next read. The X-Cache response header and the cache-category log record
// make the hit/miss outcome observable.
func (pc *PostController) ActionIndex(c *web.Context) error {
	ctx := c.Context()
	dep := cache.NewValueDependency(postMaxID(ctx))

	rebuilt := false
	var posts []Post
	err := c.App().Cache().GetOrSet(ctx, postListCacheKey, &posts,
		func(ctx context.Context) (any, error) {
			rebuilt = true
			rows, err := ar.New[Post](ctx).OrderBy("id DESC").Limit(50).All()
			if err != nil {
				return nil, err
			}
			return rows, nil
		},
		time.Minute, dep)
	if err != nil {
		return err
	}

	state := "HIT"
	if rebuilt {
		state = "MISS"
	}
	c.App().Logger().Log(slog.LevelDebug, "cache", "post list served", "state", state, "dependency", postMaxID(ctx))
	c.Response().Header().Set("X-Cache", state)
	return c.JSON(posts)
}

// ActionView fetches one post with its author eager-loaded (With), the
// Active Record association preload.
func (pc *PostController) ActionView(c *web.Context) error {
	id, err := c.ParamInt("id")
	if err != nil {
		return web.NotFound("post not found")
	}
	post, err := ar.New[Post](c.Context()).With("Author").Where("id = ?", id).One()
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return web.NotFound("post not found")
	}
	if err != nil {
		return err
	}
	return c.JSON(post)
}

// postPayload is the request body of the write actions.
type postPayload struct {
	Title  string `json:"title"`
	Body   string `json:"body"`
	Status string `json:"status"`
}

// ActionCreate inserts a post. Save runs the insert scenario first: a body
// the rules reject never reaches the database and comes back as 422 with the
// field -> messages map.
func (pc *PostController) ActionCreate(c *web.Context) error {
	var payload postPayload
	if err := json.NewDecoder(c.Request().Body).Decode(&payload); err != nil {
		return web.BadRequest("malformed JSON body")
	}
	author, ok := c.User().Identity().(*User)
	if !ok || author == nil {
		return web.Unauthorized("authentication required")
	}
	post := ar.New[Post](c.Context()).New()
	post.Title, post.Body, post.Status = payload.Title, payload.Body, payload.Status
	post.AuthorID = author.ID
	if err := post.Save(); err != nil {
		var verrs *validators.Errors
		if errors.As(err, &verrs) {
			return c.JSONStatus(http.StatusUnprocessableEntity, verrs.Fields)
		}
		return err
	}
	// Reverse URL generation: the named route fills the path parameter.
	c.Response().Header().Set("Location", url.To("post/view", url.P{"id": strconv.FormatInt(post.ID, 10)}))
	return c.JSONStatus(http.StatusCreated, post)
}

// ActionUpdate rewrites a post. The update scenario validates the payload;
// a row that vanished between read and write answers 404.
func (pc *PostController) ActionUpdate(c *web.Context) error {
	id, err := c.ParamInt("id")
	if err != nil {
		return web.NotFound("post not found")
	}
	post, err := ar.FindByID[Post](c.Context(), id)
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return web.NotFound("post not found")
	}
	if err != nil {
		return err
	}
	var payload postPayload
	if err := json.NewDecoder(c.Request().Body).Decode(&payload); err != nil {
		return web.BadRequest("malformed JSON body")
	}
	post.Title, post.Body, post.Status = payload.Title, payload.Body, payload.Status
	if err := post.Save(); err != nil {
		var verrs *validators.Errors
		if errors.As(err, &verrs) {
			return c.JSONStatus(http.StatusUnprocessableEntity, verrs.Fields)
		}
		return err
	}
	return c.JSON(post)
}

// ActionDelete removes a post. Only holders of the post/delete permission
// get here — the access rules already rejected everyone else with 403, and
// the seed grants it through the admin role's hierarchy.
func (pc *PostController) ActionDelete(c *web.Context) error {
	id, err := c.ParamInt("id")
	if err != nil {
		return web.NotFound("post not found")
	}
	post, err := ar.FindByID[Post](c.Context(), id)
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return web.NotFound("post not found")
	}
	if err != nil {
		return err
	}
	if err := post.Delete(); err != nil {
		return err
	}
	c.Response().WriteHeader(http.StatusNoContent)
	return nil
}

// postMaxID reports the highest post id, the value the list cache depends
// on. A production table would use a version counter or max(updated_at)
// instead — anything that moves when the data does.
func postMaxID(ctx context.Context) int64 {
	conn, err := base.FromContext(ctx).DB()
	if err != nil {
		return -1
	}
	var maxID int64
	if err := conn.Raw("SELECT COALESCE(MAX(id), 0) FROM post").Scan(&maxID).Error; err != nil {
		return -1
	}
	return maxID
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
