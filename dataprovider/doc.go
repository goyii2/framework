// Package dataprovider is the list-scenario data feeding layer (Yii2's
// yii\data subsystem): one sanitized request-parameter parse shared by two
// providers — ActiveDataProvider pushes pagination and sorting down to SQL
// over an ar.Query, ArrayDataProvider pages and sorts an in-memory slice —
// and both answer with the same Result: the current page's items plus the
// paging metadata that API payloads and the view.LinkPager widget consume.
//
// Request parameters follow the Yii2 conventions: page (1-based), per-page
// (default 20, capped at 100) and sort — comma-separated columns with a
// leading "-" meaning descending, e.g. sort=-created_at,id. Sorting is
// whitelist-only: a request may sort exclusively by the columns the action
// declares as sortable, anything else is rejected with an error wrapping
// ErrInvalidSortField before any statement is built, so no request text ever
// reaches the ORDER BY clause. Unusable page or per-page values fall back to
// their defaults; a page beyond the last clamps to the last page (page 1
// with an empty item set when nothing matches).
//
// In an action:
//
//	params, err := dataprovider.FromRequest(c.Request(), "id", "created_at")
//	if err != nil {
//	    return web.BadRequest(err.Error()) // unsortable sort field
//	}
//	res, err := dataprovider.ActiveDataProvider[User]{
//	    Query:        ar.New[User](c.Context()).Where("status = ?", 1),
//	    Params:       params,
//	    DefaultOrder: []dataprovider.Sort{{Column: "id", Desc: true}},
//	}.Prepare()
//	if err != nil {
//	    return err
//	}
//	return c.JSON(res) // API reply: items + totalCount/pageCount/page/perPage
//
// and server-rendered listings feed the same metadata to the pager:
//
//	view.LinkPager{Pagination: res.Pagination()}.Render()
//
// ArrayDataProvider trades the query for a slice and a key extractor:
//
//	res, err := dataprovider.ArrayDataProvider[*Tag]{
//	    Items:  tags,
//	    Params: params,
//	    Key:    func(t *Tag, column string) any { return t.Name },
//	}.Prepare()
package dataprovider
