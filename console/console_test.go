package console

import (
	"bytes"
	"context"
	"io"
	"log/slog"
	"net"
	"net/http"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/spf13/cobra"

	"github.com/goyii2/framework/base"
	"github.com/goyii2/framework/config"
	"github.com/goyii2/framework/web"
)

// recordedLog captures log events so tests can observe what serve logged
// (and learn the bound address) without parsing stdout. It stands in for the
// application log component, which also proves serve logs through the shared
// assembly.
type recordedLog struct {
	mu     sync.Mutex
	events []logEvent
}

// logEvent is one captured call: message plus its key/value arguments.
type logEvent struct {
	msg  string
	args []any
}

func (l *recordedLog) Enabled(slog.Level) bool       { return true }
func (l *recordedLog) Debug(msg string, args ...any) { l.record(msg, args) }
func (l *recordedLog) Info(msg string, args ...any)  { l.record(msg, args) }
func (l *recordedLog) Warn(msg string, args ...any)  { l.record(msg, args) }
func (l *recordedLog) Error(msg string, args ...any) { l.record(msg, args) }

func (l *recordedLog) record(msg string, args []any) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.events = append(l.events, logEvent{msg: msg, args: args})
}

// arg returns the value recorded after key on the first event carrying msg.
func (l *recordedLog) arg(msg, key string) (any, bool) {
	l.mu.Lock()
	defer l.mu.Unlock()
	for _, e := range l.events {
		if e.msg != msg {
			continue
		}
		for i := 0; i+1 < len(e.args); i += 2 {
			if k, _ := e.args[i].(string); k == key {
				return e.args[i+1], true
			}
		}
	}
	return nil, false
}

// waitForArg polls until msg was logged with key and returns its value.
func (l *recordedLog) waitForArg(t *testing.T, msg, key string) string {
	t.Helper()
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		if v, ok := l.arg(msg, key); ok {
			s, _ := v.(string)
			return s
		}
		time.Sleep(2 * time.Millisecond)
	}
	t.Fatalf("log event %q with key %q never recorded", msg, key)
	return ""
}

// siteController is resolved through the application's controller registry,
// so serving its action exercises the same assembly the web handler uses.
type siteController struct{ web.Controller }

func (siteController) ActionHello(c *web.Context) error {
	_, err := c.Response().Write([]byte("hello from console-shared app"))
	return err
}

// newTestApp builds an application listening on an ephemeral port with the
// recorded log installed, the shared wiring every serve test starts from.
func newTestApp() (*base.Application, *recordedLog, config.Config) {
	cfg := config.Default()
	cfg.Web.Listen = "127.0.0.1:0"
	app := base.New(cfg)
	log := &recordedLog{}
	app.SetLog(log)
	return app, log, cfg
}

func TestServeServesRequestsThroughSharedApplication(t *testing.T) {
	app, log, _ := newTestApp()
	if err := app.RegisterController("site", siteController{}); err != nil {
		t.Fatalf("register controller: %v", err)
	}
	h := web.NewHandler(app) // one application shared by handler and console

	c := New(app, WithHandler(h))
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	errCh := make(chan error, 1)
	c.Command().SetArgs([]string{"serve"})
	go func() { errCh <- c.ExecuteContext(ctx) }()

	// The listening address is announced through the application's log
	// component — the shared assembly at work.
	addr := log.waitForArg(t, "server listening", "addr")
	resp, err := http.Get("http://" + addr + "/site/hello")
	if err != nil {
		t.Fatalf("get /site/hello: %v", err)
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("read body: %v", err)
	}
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200", resp.StatusCode)
	}
	if string(body) != "hello from console-shared app" {
		t.Fatalf("body = %q", body)
	}

	cancel()
	select {
	case err := <-errCh:
		if err != nil {
			t.Fatalf("serve exited with error: %v", err)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("serve did not return after stop")
	}
	if _, ok := log.arg("server stopped", "addr"); !ok {
		t.Fatal("\"server stopped\" never logged")
	}
}

func TestServeGracefulShutdownWaitsForInFlightRequest(t *testing.T) {
	app, log, cfg := newTestApp()

	h := web.NewHandler(app)
	entered := make(chan struct{}, 1)
	release := make(chan struct{})
	h.Get("/slow", func(c *web.Context) error {
		entered <- struct{}{}
		<-release
		_, err := c.Response().Write([]byte("slow response complete"))
		return err
	})

	c := New(app, WithHandler(h), WithShutdownTimeout(5*time.Second))

	// Capture which address serve asks the listener for; it must be the one
	// from the application configuration.
	var (
		mu        sync.Mutex
		requested string
	)
	c.listenFn = func(addr string) (net.Listener, error) {
		mu.Lock()
		requested = addr
		mu.Unlock()
		return net.Listen("tcp", addr)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	errCh := make(chan error, 1)
	c.Command().SetArgs([]string{"serve"})
	go func() { errCh <- c.ExecuteContext(ctx) }()

	addr := log.waitForArg(t, "server listening", "addr")
	mu.Lock()
	if requested != cfg.Web.Listen {
		mu.Unlock()
		t.Fatalf("listen address = %q, want config value %q", requested, cfg.Web.Listen)
	}
	mu.Unlock()

	type result struct {
		body string
		err  error
	}
	done := make(chan result, 1)
	go func() {
		resp, err := http.Get("http://" + addr + "/slow")
		if err != nil {
			done <- result{err: err}
			return
		}
		defer resp.Body.Close()
		b, err := io.ReadAll(resp.Body)
		done <- result{body: string(b), err: err}
	}()

	select {
	case <-entered:
	case <-time.After(3 * time.Second):
		t.Fatal("request never reached the handler")
	}

	// Stop signal first, response released only after shutdown began: the
	// request must still complete, or shutdown was not graceful.
	cancel()
	log.waitForArg(t, "server shutting down", "addr")
	close(release)

	select {
	case r := <-done:
		if r.err != nil {
			t.Fatalf("in-flight request failed: %v", r.err)
		}
		if r.body != "slow response complete" {
			t.Fatalf("in-flight body = %q", r.body)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("in-flight request never completed")
	}

	select {
	case err := <-errCh:
		if err != nil {
			t.Fatalf("serve exit error: %v", err)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("serve did not return after graceful shutdown")
	}
}

func TestServeShutdownTimeoutFailsTheCommand(t *testing.T) {
	app, log, _ := newTestApp()

	h := web.NewHandler(app)
	entered := make(chan struct{}, 1)
	release := make(chan struct{})
	defer close(release)
	h.Get("/hang", func(c *web.Context) error {
		entered <- struct{}{}
		<-release
		return nil
	})

	c := New(app, WithHandler(h), WithShutdownTimeout(50*time.Millisecond))
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	errCh := make(chan error, 1)
	c.Command().SetArgs([]string{"serve"})
	go func() { errCh <- c.ExecuteContext(ctx) }()

	addr := log.waitForArg(t, "server listening", "addr")
	go func() {
		resp, err := http.Get("http://" + addr + "/hang")
		if err == nil {
			resp.Body.Close()
		}
	}()
	<-entered
	cancel()

	select {
	case err := <-errCh:
		if err == nil {
			t.Fatal("serve exit = nil, want shutdown timeout error")
		}
		if !strings.Contains(err.Error(), "graceful shutdown") {
			t.Fatalf("error = %v, want graceful shutdown failure", err)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("serve did not return after shutdown timeout")
	}
}

func TestUserCommandsRegisterBelowRootAndReachApplication(t *testing.T) {
	cfg := config.Default()
	cfg.Env = "staging"
	app := base.New(cfg)

	var (
		ran     bool
		fromCtx *base.Application
	)
	c := New(app)
	c.AddCommand(&cobra.Command{
		Use: "greet",
		RunE: func(cmd *cobra.Command, _ []string) error {
			ran = true
			fromCtx = base.FromContext(cmd.Context())
			return nil
		},
	})

	c.Command().SetArgs([]string{"greet"})
	c.Command().SetErr(io.Discard)
	if err := c.Execute(); err != nil {
		t.Fatalf("execute greet: %v", err)
	}
	if !ran {
		t.Fatal("custom command did not run")
	}
	if fromCtx != app {
		t.Fatal("command context does not carry the console's application")
	}
	if got := fromCtx.Config().Env; got != "staging" {
		t.Fatalf("env = %q, want the value from the shared config", got)
	}
}

func TestHelpListsBuiltInAndUserCommands(t *testing.T) {
	c := New(base.New(config.Default()))
	c.AddCommand(&cobra.Command{
		Use:   "greet",
		Short: "Greet the user",
		Run:   func(*cobra.Command, []string) {},
	})

	var buf bytes.Buffer
	c.Command().SetOut(&buf)
	c.Command().SetErr(io.Discard)
	c.Command().SetArgs([]string{"--help"})
	if err := c.Execute(); err != nil {
		t.Fatalf("execute --help: %v", err)
	}
	out := buf.String()
	for _, want := range []string{
		"goyii",
		"serve",
		"Start the web server",
		"greet",
		"Available Commands",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("help output missing %q:\n%s", want, out)
		}
	}
}

func TestServeWithoutHandlerReportsSetupError(t *testing.T) {
	c := New(base.New(config.Default()))
	c.Command().SetArgs([]string{"serve"})
	c.Command().SetErr(io.Discard)
	err := c.Execute()
	if err == nil || !strings.Contains(err.Error(), "no HTTP handler") {
		t.Fatalf("error = %v, want missing-handler setup error", err)
	}
}
