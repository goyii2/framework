// QueueController: the queue demo page — the live state of the queue table,
// a button that pushes a demo email job, and the worker commands that drain
// it out-of-band.
package controllers

import (
	"net/http"
	"strconv"
	"strings"

	"github.com/goyii2/framework/examples/basic/jobs"
	"github.com/goyii2/framework/examples/basic/views"
	"github.com/goyii2/framework/i18n"
	"github.com/goyii2/framework/url"
	"github.com/goyii2/framework/web"
)

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
	status, err := jobs.ReadQueueStatus(ctx)
	if err != nil {
		return err
	}
	t := func(msgID string) string { return i18n.T(ctx, "app", msgID, nil) }
	c.SetTitle(t("Queue"))
	return c.Render("queue/index", views.QueuePageData{
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
	job := &jobs.EmailJob{
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
