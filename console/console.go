package console

import (
	"context"
	"net"
	"net/http"
	"time"

	"github.com/spf13/cobra"

	"github.com/goyii2/framework/base"
)

// DefaultShutdownTimeout bounds how long graceful shutdown waits for
// in-flight requests after a stop signal; beyond it Shutdown fails and the
// command returns the error.
const DefaultShutdownTimeout = 10 * time.Second

// Console is the CLI entry point around one application. It owns the root
// cobra command ("goyii") with the built-in subcommands and accepts
// application-defined commands via AddCommand. Constructing it from the same
// *base.Application as the web handler keeps CLI and web on one config
// source and one component assembly.
type Console struct {
	app  *base.Application
	root *cobra.Command

	// handler is the HTTP handler the serve subcommand starts
	// (see WithHandler).
	handler http.Handler

	// shutdownTimeout bounds serve's graceful shutdown wait.
	shutdownTimeout time.Duration

	// listenFn opens the TCP listener for serve; overridable in tests to
	// capture the bound address.
	listenFn func(addr string) (net.Listener, error)
}

// Option configures a Console at construction time.
type Option func(*Console)

// WithHandler sets the HTTP handler the serve subcommand starts, typically
// the *web.Handler built from the same application.
func WithHandler(h http.Handler) Option {
	return func(c *Console) { c.handler = h }
}

// WithShutdownTimeout overrides how long serve waits for in-flight requests
// during graceful shutdown (default DefaultShutdownTimeout).
func WithShutdownTimeout(d time.Duration) Option {
	return func(c *Console) { c.shutdownTimeout = d }
}

// New creates the console for app: the "goyii" root command with the
// built-in subcommands attached. Options are applied after the built-ins.
func New(app *base.Application, opts ...Option) *Console {
	c := &Console{
		app:             app,
		shutdownTimeout: DefaultShutdownTimeout,
		listenFn: func(addr string) (net.Listener, error) {
			return net.Listen("tcp", addr)
		},
	}
	c.root = &cobra.Command{
		Use:   "goyii",
		Short: "Console entry point of the goyii application",
		// Runtime failures (port in use, shutdown timeout, ...) are not
		// usage mistakes, so the usage text stays out of their output.
		SilenceUsage: true,
	}
	c.root.AddCommand(newServeCommand(c))
	for _, opt := range opts {
		opt(c)
	}
	return c
}

// App returns the application the console shares with the web handler.
func (c *Console) App() *base.Application { return c.app }

// Command returns the root cobra command for wiring beyond AddCommand,
// e.g. attaching persistent flags or command groups.
func (c *Console) Command() *cobra.Command { return c.root }

// AddCommand attaches application-defined commands below the root command,
// where they appear alongside the built-ins in goyii --help. It returns the
// console so construction calls can chain.
func (c *Console) AddCommand(cmds ...*cobra.Command) *Console {
	c.root.AddCommand(cmds...)
	return c
}

// Execute runs the root command on os.Args (see cobra.Command.Execute) and
// returns its error for the caller to map onto the process exit code.
func (c *Console) Execute() error { return c.ExecuteContext(context.Background()) }

// ExecuteContext is Execute with the base context commands run under. The
// application is attached to that context, so inside any command
// base.FromContext(cmd.Context()) reaches the console's application instead
// of the package-level default.
func (c *Console) ExecuteContext(ctx context.Context) error {
	return c.root.ExecuteContext(c.app.WithContext(ctx))
}
