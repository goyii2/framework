// Package commands provides the goyii gii command tree: the model,
// controller and crud generators of the framework's gii package, attached
// below the goyii console root command the way the migrate commands are.
package commands

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/goyii2/framework/base"
	"github.com/goyii2/framework/db"
	"github.com/goyii2/framework/gii"
)

// New returns the "gii" parent command with the model, controller and crud
// subcommands. The database commands resolve their connection from the
// application's db component; --dsn overrides the configured DSN.
func New(app *base.Application) *cobra.Command {
	root := &cobra.Command{
		Use:   "gii",
		Short: "Generate models, controllers and CRUD scaffolding",
	}
	root.AddCommand(newModelCommand(app), newControllerCommand(), newCrudCommand(app))
	return root
}

// newModelCommand builds "gii model": the Active Record model of one table.
func newModelCommand(app *base.Application) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "model",
		Short: "Generate the Active Record model of a table",
		RunE: func(cmd *cobra.Command, args []string) error {
			table, err := cmd.Flags().GetString("table")
			if err != nil {
				return err
			}
			conn, cleanup, err := openConn(app, cmd)
			if err != nil {
				return err
			}
			if cleanup != nil {
				defer cleanup()
			}
			result, err := gii.GenerateModel(conn.DB, table, generationOptions(cmd))
			if err != nil {
				return err
			}
			report(cmd, result)
			return nil
		},
	}
	addFlags(cmd)
	cmd.Flags().String("table", "", "table to generate the model from (required)")
	return cmd
}

// newControllerCommand builds "gii controller": an empty controller
// skeleton for one controller id. It needs no database.
func newControllerCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "controller",
		Short: "Generate a controller skeleton",
		RunE: func(cmd *cobra.Command, args []string) error {
			id, err := cmd.Flags().GetString("id")
			if err != nil {
				return err
			}
			result, err := gii.GenerateController(id, generationOptions(cmd))
			if err != nil {
				return err
			}
			report(cmd, result)
			return nil
		},
	}
	addFlags(cmd)
	cmd.Flags().String("id", "", "controller id, e.g. post (required)")
	return cmd
}

// newCrudCommand builds "gii crud": the model, controller and templ views
// of one table's full create/read/update/delete set.
func newCrudCommand(app *base.Application) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "crud",
		Short: "Generate a model, controller and views for a table",
		RunE: func(cmd *cobra.Command, args []string) error {
			table, err := cmd.Flags().GetString("table")
			if err != nil {
				return err
			}
			conn, cleanup, err := openConn(app, cmd)
			if err != nil {
				return err
			}
			if cleanup != nil {
				defer cleanup()
			}
			result, err := gii.GenerateCrud(conn.DB, table, generationOptions(cmd))
			if err != nil {
				return err
			}
			report(cmd, result)
			return nil
		},
	}
	addFlags(cmd)
	cmd.Flags().String("table", "", "table to generate the CRUD set for (required)")
	return cmd
}

// addFlags registers the flags every generator shares: --dsn (connection
// override), --out, --package and --templates.
func addFlags(cmd *cobra.Command) {
	cmd.Flags().String("dsn", "", "database DSN; overrides the application's db.dsn configuration")
	cmd.Flags().String("out", ".", "output directory (created when missing)")
	cmd.Flags().String("package", "", "package name of the generated files (default: the output directory's base name)")
	cmd.Flags().String("templates", "", "directory of template files overriding the built-in set by name")
}

// generationOptions collects the shared flags into generator options.
func generationOptions(cmd *cobra.Command) gii.Options {
	opts := gii.Options{}
	opts.Out, _ = cmd.Flags().GetString("out")
	opts.Package, _ = cmd.Flags().GetString("package")
	opts.TemplateDir, _ = cmd.Flags().GetString("templates")
	return opts
}

// openConn resolves the database connection: the application's db component
// by default, a fresh one when --dsn overrides the DSN (the driver follows
// the application's connection). The returned cleanup closes the fresh
// connection (nil when the component's connection was reused — its lifetime
// belongs to the application).
func openConn(app *base.Application, cmd *cobra.Command) (*db.Conn, func(), error) {
	dsn, err := cmd.Flags().GetString("dsn")
	if err != nil {
		return nil, nil, err
	}
	if dsn == "" {
		return app.DB(), nil, nil
	}
	conn, err := db.New(db.Config{Driver: app.DB().Driver(), DSN: dsn})
	if err != nil {
		return nil, nil, err
	}
	return conn, func() { _ = conn.Close() }, nil
}

// report prints the written files and, separated by blank lines, the
// wiring snippets (route registration, view registration, follow-up
// hints).
func report(cmd *cobra.Command, result *gii.Result) {
	out := cmd.OutOrStdout()
	for _, file := range result.Files {
		fmt.Fprintf(out, "created %s\n", file)
	}
	for _, snippet := range result.Snippets {
		fmt.Fprintln(out)
		fmt.Fprintln(out, snippet)
	}
}
