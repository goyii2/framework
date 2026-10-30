package console

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"syscall"

	"github.com/spf13/cobra"
)

// newServeCommand builds the built-in serve subcommand: it starts the HTTP
// handler on the injected listen address and runs until SIGINT or SIGTERM
// arrives, then drains in-flight requests before returning.
func newServeCommand(c *Console) *cobra.Command {
	return &cobra.Command{
		Use:   "serve",
		Short: "Start the web server and serve until SIGINT/SIGTERM",
		RunE: func(cmd *cobra.Command, _ []string) error {
			return c.serve(cmd.Context())
		},
	}
}

// serve runs the HTTP server until a stop signal (or the surrounding
// context going away, which is how tests trigger it) arrives, then shuts
// down gracefully: the listener closes immediately, in-flight requests may
// finish within the shutdown timeout, and shutdown failures (such as the
// timeout expiring on requests still open) surface as the command's error.
func (c *Console) serve(ctx context.Context) error {
	if c.handler == nil {
		return errors.New("console: serve has no HTTP handler; construct the console with console.WithHandler (typically web.NewHandler(app))")
	}
	addr := c.listenAddr
	ln, err := c.listenFn(addr)
	if err != nil {
		return fmt.Errorf("console: listen on %s: %w", addr, err)
	}
	defer ln.Close()

	srv := &http.Server{Addr: ln.Addr().String(), Handler: c.handler}
	c.app.Log().Info("server listening", "addr", ln.Addr().String(), "env", c.app.Env())

	serveErr := make(chan error, 1)
	go func() { serveErr <- srv.Serve(ln) }()

	// The surrounding context doubles as the stop trigger: in production it
	// is the process context and only the signals fire; a canceled context
	// drives the identical graceful-shutdown path.
	notifyCtx, stop := signal.NotifyContext(ctx, os.Interrupt, syscall.SIGTERM)
	defer stop()

	select {
	case err := <-serveErr:
		if errors.Is(err, http.ErrServerClosed) {
			return nil
		}
		return fmt.Errorf("console: serve: %w", err)
	case <-notifyCtx.Done():
	}

	c.app.Log().Info("server shutting down", "addr", ln.Addr().String())
	shutdownCtx, cancel := context.WithTimeout(context.Background(), c.shutdownTimeout)
	defer cancel()
	if err := srv.Shutdown(shutdownCtx); err != nil {
		return fmt.Errorf("console: graceful shutdown: %w", err)
	}
	c.app.Log().Info("server stopped", "addr", ln.Addr().String())
	return nil
}
