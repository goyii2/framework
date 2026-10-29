package commands

import (
	"context"
	"errors"
	"io"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/spf13/cobra"

	"github.com/goyii2/framework/base"
	"github.com/goyii2/framework/config"
	"github.com/goyii2/framework/migrate"
	"github.com/goyii2/framework/queue"
)

// cmdJob is the succeeding test job; failingJob always fails. Observations
// live in a package counter because the worker rebuilds fresh instances from
// the payload.
type cmdJob struct{}

func (*cmdJob) Name() string { return "cmd" }

func (*cmdJob) Execute(context.Context) error {
	cmdRuns.Add(1)
	return nil
}

type failingJob struct{}

func (*failingJob) Name() string { return "failing" }

func (*failingJob) Execute(context.Context) error {
	cmdRuns.Add(1)
	return errBoom
}

var (
	cmdRuns atomic.Int32
	errBoom = errors.New("boom")
)

// newTestConsole builds a "goyii" root command carrying the queue command
// tree over a sqlite-backed application with the queue table applied and the
// test jobs registered.
func newTestConsole(t *testing.T) (*cobra.Command, *base.Application) {
	t.Helper()
	dsn := filepath.Join(t.TempDir(), "queue.db")
	app := base.New(config.Config{
		App:   config.App{Name: "queuedemo"},
		DB:    config.DB{Driver: "sqlite", DSN: dsn},
		Queue: config.Queue{Driver: "db", TableName: "queue", PollIntervalMs: 5},
	})

	conn, err := app.DB()
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	t.Cleanup(func() { _ = conn.Close() }) // release the file before TempDir cleanup
	sqlDB, err := conn.DB.DB()
	if err != nil {
		t.Fatalf("raw handle: %v", err)
	}
	dialect, ok := migrate.DialectByName("sqlite")
	if !ok {
		t.Fatal("sqlite dialect missing")
	}
	if err := queue.NewMigration("").Up(context.Background(), migrate.NewMigrationContext(sqlDB, dialect)); err != nil {
		t.Fatalf("apply queue migration: %v", err)
	}

	for _, job := range []queue.Job{&cmdJob{}, &failingJob{}} {
		if err := app.Queue().Jobs().Register(job); err != nil {
			t.Fatalf("register %s: %v", job.Name(), err)
		}
	}

	root := &cobra.Command{Use: "goyii"}
	root.AddCommand(New(app))
	return root, app
}

// runCommand executes args against root, returning its stdout.
func runCommand(t *testing.T, root *cobra.Command, args ...string) string {
	t.Helper()
	var buf strings.Builder
	root.SetOut(&buf)
	root.SetErr(&buf)
	root.SetArgs(args)
	if err := root.Execute(); err != nil {
		t.Fatalf("%v: %v", args, err)
	}
	return buf.String()
}

// TestCommandTreeShape verifies the three subcommands mount below "queue".
func TestCommandTreeShape(t *testing.T) {
	root, _ := newTestConsole(t)
	queueCmd, _, err := root.Find([]string{"queue"})
	if err != nil {
		t.Fatalf("find queue command: %v", err)
	}
	var names []string
	for _, c := range queueCmd.Commands() {
		names = append(names, c.Name())
	}
	for _, want := range []string{"listen", "run", "retry"} {
		found := false
		for _, n := range names {
			if n == want {
				found = true
			}
		}
		if !found {
			t.Errorf("queue subcommands = %v, missing %s", names, want)
		}
	}
}

// TestRunDrainsDueMessages verifies "queue run" processes due messages once
// and reports the count.
func TestRunDrainsDueMessages(t *testing.T) {
	root, app := newTestConsole(t)
	cmdRuns.Store(0)
	ctx := context.Background()

	for i := 0; i < 2; i++ {
		if _, err := app.Queue().Push(ctx, &cmdJob{}); err != nil {
			t.Fatalf("Push: %v", err)
		}
	}
	out := runCommand(t, root, "queue", "run")
	if cmdRuns.Load() != 2 {
		t.Errorf("job executed %d times, want 2", cmdRuns.Load())
	}
	if !strings.Contains(out, "2") {
		t.Errorf("run output = %q, want the processed count", out)
	}

	// Nothing left: a second run reports zero.
	cmdRuns.Store(0)
	out2 := runCommand(t, root, "queue", "run")
	if cmdRuns.Load() != 0 || strings.Contains(out2, "1") {
		t.Errorf("second run: runs=%d output=%q, want 0 processed", cmdRuns.Load(), out2)
	}
}

// TestRetryRepushesFailed verifies "queue retry" moves failed messages back
// and a following run executes them.
func TestRetryRepushesFailed(t *testing.T) {
	root, app := newTestConsole(t)
	cmdRuns.Store(0)
	ctx := context.Background()

	// One attempt, always failing: parks as failed.
	if _, err := app.Queue().Push(ctx, &failingJob{}); err != nil {
		t.Fatalf("Push: %v", err)
	}
	runCommand(t, root, "queue", "run")
	if cmdRuns.Load() != 1 {
		t.Fatalf("runs = %d, want 1", cmdRuns.Load())
	}

	out := runCommand(t, root, "queue", "retry")
	if !strings.Contains(out, "1") {
		t.Errorf("retry output = %q, want the re-pushed count", out)
	}
	runCommand(t, root, "queue", "run")
	if cmdRuns.Load() != 2 {
		t.Errorf("runs after retry = %d, want 2", cmdRuns.Load())
	}
}

// TestRunUnknownJobFailsTheMessageNotTheCommand verifies a message whose job
// is not registered parks as failed instead of failing the command — an
// unregistered name is queue content, not infrastructure.
func TestRunUnknownJobFailsTheMessageNotTheCommand(t *testing.T) {
	root, app := newTestConsole(t)
	ctx := context.Background()

	// Push a "ghost" message through a second queue over the same table: the
	// application's registry does not know the name.
	conn, err := app.DB()
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	ghostReg, err := queue.NewRegistry(&namedTestJob{name: "ghost"})
	if err != nil {
		t.Fatalf("NewRegistry: %v", err)
	}
	other := queue.New(queue.NewDB(conn.DB, ""), ghostReg)
	if _, err := other.Push(ctx, &namedTestJob{name: "ghost"}); err != nil {
		t.Fatalf("Push ghost: %v", err)
	}

	runCommand(t, root, "queue", "run") // must not fail the command

	// It parked as failed: retry reports it back (still unrunnable here, but
	// the count proves the failed state was reached).
	out := runCommand(t, root, "queue", "retry")
	if !strings.Contains(out, "1") {
		t.Errorf("retry output = %q, want 1 re-pushed failed message", out)
	}
}

// namedTestJob is a job with a fixed name for targeted pushes.
type namedTestJob struct{ name string }

func (j *namedTestJob) Name() string                { return j.name }
func (*namedTestJob) Execute(context.Context) error { return nil }

// TestListenStopsOnCanceledContext verifies "queue listen" keeps working
// until its context goes away (the same path a SIGINT takes in production),
// then exits cleanly having drained the message pushed before it started.
func TestListenStopsOnCanceledContext(t *testing.T) {
	root, app := newTestConsole(t)
	cmdRuns.Store(0)

	if _, err := app.Queue().Push(context.Background(), &cmdJob{}); err != nil {
		t.Fatalf("Push: %v", err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	time.AfterFunc(150*time.Millisecond, cancel)

	root.SetContext(ctx)
	root.SetArgs([]string{"queue", "listen"})
	root.SetOut(io.Discard)
	root.SetErr(io.Discard)
	if err := root.Execute(); err != nil {
		t.Fatalf("queue listen: %v", err)
	}
	if cmdRuns.Load() == 0 {
		t.Error("listen worker never drained the pushed message")
	}
}
