// Package console provides the CLI command framework around
// github.com/spf13/cobra; it corresponds to Yii2's yii\console subsystem.
//
// A Console wraps the same *base.Application the web handler uses, so CLI
// commands and HTTP requests share one configuration source and one set of
// components. The root command ("goyii") carries the built-in serve
// subcommand; applications attach their own cobra commands with AddCommand:
//
//	app := base.New(goyii.LoadConfig("config/"))
//	h := web.NewHandler(app)
//	c := console.New(app, console.WithHandler(h))
//	c.AddCommand(&cobra.Command{Use: "hello", Short: "Greet", RunE: runHello})
//	if err := c.Execute(); err != nil {
//		os.Exit(1)
//	}
//
// Execute runs the root command on os.Args. Inside any command the
// application is reachable via base.FromContext(cmd.Context()).
package console
