// The queue demo's jobs and the queue-state helper its page reads. Jobs are
// registered on the application queue's registry in main before any command
// runs, so serve, seed and the queue workers all resolve the same names.
package main

import (
	"context"

	"github.com/goyii2/framework/base"
	"github.com/goyii2/framework/mail"
)

// EmailJob is the demo job: it composes a notification message through the
// application's mailer. With the log driver the complete message lands in the
// worker's output and the application log — the observable counterpart of an
// SMTP delivery. Its exported fields are the JSON payload that travels
// through the queue table.
type EmailJob struct {
	To      string `json:"to"`
	Subject string `json:"subject"`
	Body    string `json:"body"`
}

// Name is the registry key the worker resolves pushed messages by.
func (j *EmailJob) Name() string { return "email" }

// Execute delivers the message through the application the worker context
// carries.
func (j *EmailJob) Execute(ctx context.Context) error {
	app := base.FromContext(ctx)
	msg := mail.NewMessage().
		SetFrom(app.Config().Mail.From).
		SetTo(j.To).
		SetSubject(j.Subject).
		SetTextBody(j.Body)
	return app.Mailer().Send(ctx, msg)
}

// queueStatus is the state summary the queue page shows: one count per row
// status plus the waiting total.
type queueStatus struct {
	Waiting  int64
	Reserved int64
	Failed   int64
}

// queueTable is the db driver's table, kept in one place so the page query
// and the migration registration cannot drift apart.
const queueTable = "queue"

// The db driver's documented row statuses: waiting rows are runnable when
// due, reserved rows are held by a worker, failed rows are parked after their
// attempts ran out.
const (
	rowStatusWaiting  = 0
	rowStatusReserved = 1
	rowStatusFailed   = 2
)

// readQueueStatus counts the queue rows per status directly from the table.
// The demo page owes its numbers to the same storage the driver works on, so
// what the visitor sees is what a worker would find.
func readQueueStatus(ctx context.Context) (queueStatus, error) {
	var s queueStatus
	app := base.FromContext(ctx)
	conn, err := app.DB()
	if err != nil {
		return s, err
	}
	rows := []struct {
		Status int   `gorm:"column:status"`
		Count  int64 `gorm:"column:count"`
	}{}
	if err := conn.Raw("SELECT status, COUNT(*) AS count FROM " + queueTable + " GROUP BY status").Scan(&rows).Error; err != nil {
		return s, err
	}
	for _, row := range rows {
		switch row.Status {
		case rowStatusWaiting:
			s.Waiting = row.Count
		case rowStatusReserved:
			s.Reserved = row.Count
		case rowStatusFailed:
			s.Failed = row.Count
		}
	}
	return s, nil
}
