// Package commands provides the goyii queue command tree: the three worker
// subcommands listen (resident worker until SIGINT/SIGTERM), run (one drain
// pass, for cron-style scheduling) and retry (re-push failed messages).
// Applications attach it below the console root alongside the built-ins:
//
//	console.AddCommand(queuecommands.New(app))
//
// The commands resolve the queue through the application component
// (app.Queue), so the driver and its values are whatever the application
// entry installed, and jobs resolve through the component's registry —
// applications register their jobs before the console runs:
//
//	app.Queue().Jobs().Register(&emailJob{}, &reportJob{})
package commands

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"github.com/spf13/cobra"

	"github.com/goyii2/framework/base"
)

// New returns the "queue" parent command with the three worker subcommands.
func New(app *base.Application) *cobra.Command {
	root := &cobra.Command{
		Use:   "queue",
		Short: "Work the async job queue (listen, run one pass, retry failed)",
	}
	root.AddCommand(
		newListenCommand(app),
		newRunCommand(app),
		newRetryCommand(app),
	)
	return root
}

// workerContext wraps the command context with SIGINT/SIGTERM cancellation —
// the production stop trigger for a resident worker. The surrounding context
// drives the identical graceful path when a test or supervisor cancels it.
func workerContext(ctx context.Context) (context.Context, func()) {
	return signal.NotifyContext(ctx, os.Interrupt, syscall.SIGTERM)
}

// newListenCommand builds "queue listen": drain passes forever at the
// configured poll interval until stopped.
func newListenCommand(app *base.Application) *cobra.Command {
	return &cobra.Command{
		Use:   "listen",
		Short: "Consume the queue until SIGINT/SIGTERM (resident worker)",
		RunE: func(cmd *cobra.Command, _ []string) error {
			ctx, stop := workerContext(cmd.Context())
			defer stop()
			app.Log().Info("queue worker listening", "driver", fmt.Sprintf("%T", app.Queue().Driver()))
			err := app.Queue().Listen(ctx)
			app.Log().Info("queue worker stopped")
			return err
		},
	}
}

// newRunCommand builds "queue run": process every due message once and
// report how many ran.
func newRunCommand(app *base.Application) *cobra.Command {
	return &cobra.Command{
		Use:   "run",
		Short: "Process due messages once (cron-style worker)",
		RunE: func(cmd *cobra.Command, _ []string) error {
			q := app.Queue()
			n, err := q.Run(cmd.Context())
			if err != nil {
				return err
			}
			fmt.Fprintf(cmd.OutOrStdout(), "processed %d message(s)\n", n)
			return nil
		},
	}
}

// newRetryCommand builds "queue retry": move every failed message back to
// waiting with its attempt count reset and report how many were re-pushed.
func newRetryCommand(app *base.Application) *cobra.Command {
	return &cobra.Command{
		Use:   "retry",
		Short: "Re-push failed messages (attempt counts reset)",
		RunE: func(cmd *cobra.Command, _ []string) error {
			q := app.Queue()
			n, err := q.RetryFailed(cmd.Context())
			if err != nil {
				return err
			}
			fmt.Fprintf(cmd.OutOrStdout(), "re-pushed %d failed message(s)\n", n)
			return nil
		},
	}
}
