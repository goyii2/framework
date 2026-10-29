package queue

import (
	"context"
	"time"
)

// Failing wraps err into a driver whose every operation reports it — the
// shape the application core uses when the configured driver cannot be built
// (a database that will not open, an unknown driver name): the mistake
// surfaces at first use instead of breaking application assembly.
func Failing(err error) Driver { return failingDriver{err} }

// failingDriver reports one fixed error from every operation.
type failingDriver struct{ err error }

func (f failingDriver) Push(context.Context, *Message) error                   { return f.err }
func (f failingDriver) Reserve(context.Context) (*Message, error)              { return nil, f.err }
func (f failingDriver) Complete(context.Context, *Message) error               { return f.err }
func (f failingDriver) Requeue(context.Context, *Message, time.Duration) error { return f.err }
func (f failingDriver) Fail(context.Context, *Message) error                   { return f.err }
func (f failingDriver) RetryFailed(context.Context) (int, error)               { return 0, f.err }
