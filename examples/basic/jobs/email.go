// The queue demo's job and the storage-level state helpers its page reads.
// The job is registered on the application queue's registry in main before
// any command runs, so serve, seed and the queue workers all resolve the
// same names.
package jobs

import (
	"context"

	"github.com/goyii2/framework/base"
	"github.com/goyii2/framework/mail"
)

// Table is the db driver's queue table, kept in one place so the demo page
// query, the seed count and the migration registration cannot drift apart.
const Table = "queue"

// EmailJob is the demo job: it composes a notification message through the
// application's mailer. With the log driver the complete message lands in
// the worker's output and the application log — the observable counterpart of an
// SMTP delivery. Its exported fields are the JSON payload that travels
// through the queue table.
type EmailJob struct {
	To      string `json:"to"`
	Subject string `json:"subject"`
	Body    string `json:"body"`
}

// Name is the registry key the worker resolves pushed messages by.
func (j *EmailJob) Name() string { return "email" }

// defaultFrom is the sender the demo messages carry; it matches the
// project's mail.from value (assembly in main.go).
const defaultFrom = "goyii basic <no-reply@goyii.example>"

// Execute delivers the message through the application the worker context
// carries.
func (j *EmailJob) Execute(ctx context.Context) error {
	app := base.FromContext(ctx)
	msg := mail.NewMessage().
		SetFrom(defaultFrom).
		SetTo(j.To).
		SetSubject(j.Subject).
		SetTextBody(j.Body)
	return app.Mailer().Send(ctx, msg)
}

// Status is the state summary the queue page shows: one count per row
// status plus the waiting total.
type Status struct {
	Waiting  int64
	Reserved int64
	Failed   int64
}

// The db driver's documented row statuses: waiting rows are runnable when
// due, reserved rows are held by a worker, failed rows are parked after their
// attempts ran out.
const (
	rowStatusWaiting  = 0
	rowStatusReserved = 1
	rowStatusFailed   = 2
)

// ReadQueueStatus counts the queue rows per status directly from the table.
// The demo page owes its numbers to the same storage the driver works on, so
// what the visitor sees is what a worker would find.
func ReadQueueStatus(ctx context.Context) (Status, error) {
	var s Status
	conn := base.FromContext(ctx).DB()
	rows := []struct {
		Status int   `gorm:"column:status"`
		Count  int64 `gorm:"column:count"`
	}{}
	if err := conn.Raw("SELECT status, COUNT(*) AS count FROM " + Table + " GROUP BY status").Scan(&rows).Error; err != nil {
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
