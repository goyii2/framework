// The seed command: a user-defined console command filling the freshly
// migrated database with demo data. It runs through the same application
// assembly as serve, so the Active Record layer (and its insert validation)
// applies here too.
package main

import (
	"context"
	"errors"
	"fmt"

	"github.com/spf13/cobra"

	"github.com/goyii2/framework/base"
	"github.com/goyii2/framework/db/ar"
	"github.com/goyii2/framework/rbac"
)

// demoAccounts are the seed users: admin may delete posts (access rule),
// demo proves the plain authenticated role.
var demoAccounts = []struct {
	username string
	password string
	role     string
}{
	{"admin", "admin123", "admin"},
	{"demo", "demo123", "user"},
}

// newSeedCommand builds the "seed" console command.
func newSeedCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "seed",
		Short: "Create the demo accounts and an example post",
		RunE: func(cmd *cobra.Command, _ []string) error {
			ctx := cmd.Context() // carries the application (see console.ExecuteContext)

			for _, acc := range demoAccounts {
				n, err := ar.New[User](ctx).Where("username = ?", acc.username).Count()
				if err != nil {
					return err
				}
				if n > 0 {
					fmt.Fprintf(cmd.OutOrStdout(), "user %s already exists\n", acc.username)
					continue
				}
				user := ar.New[User](ctx).New()
				user.Username, user.Role = acc.username, acc.role
				if err := user.SetPassword(acc.password); err != nil {
					return err
				}
				if err := user.Save(); err != nil {
					return fmt.Errorf("seed user %s: %w", acc.username, err)
				}
				fmt.Fprintf(cmd.OutOrStdout(), "created user %s (%s)\n", acc.username, acc.role)
			}

			if n, err := ar.New[Post](ctx).Count(); err != nil {
				return err
			} else if n > 0 {
				fmt.Fprintln(cmd.OutOrStdout(), "posts already present, skipping example post")
				return seedRbac(ctx)
			}
			admin, err := ar.New[User](ctx).Where("username = ?", "admin").One()
			if err != nil {
				return err
			}
			post := ar.New[Post](ctx).New()
			post.Title = "Hello goyii"
			post.Body = "This post was created by the seed command. Create your own through POST /post."
			post.Status = PostStatusPublished
			post.AuthorID = admin.ID
			if err := post.Save(); err != nil {
				return fmt.Errorf("seed example post: %w", err)
			}
			fmt.Fprintf(cmd.OutOrStdout(), "created example post %d\n", post.ID)
			return seedRbac(ctx)
		},
	}
}

// seedRbac fills the authorization tables: an admin role containing the
// post/delete permission, and the admin account carrying that role. The
// delete rule of PostController asks for the permission and receives it
// through the hierarchy — the demo of indirect grants. Re-running against an
// already-seeded database changes nothing.
func seedRbac(ctx context.Context) error {
	app := base.FromContext(ctx)
	conn, err := app.DB()
	if err != nil {
		return err
	}
	auth, err := rbac.NewDbManager(conn.DB)
	if err != nil {
		return err
	}

	ensureItem := func(item rbac.Item) error {
		err := auth.AddItem(item)
		switch {
		case err == nil, errors.Is(err, rbac.ErrItemExists):
			return nil
		default:
			return fmt.Errorf("seed item %s: %w", item.Name, err)
		}
	}
	if err := ensureItem(rbac.Item{Name: "admin", Type: rbac.TypeRole, Description: "may do everything"}); err != nil {
		return err
	}
	if err := ensureItem(rbac.Item{Name: "post/delete", Type: rbac.TypePermission, Description: "may delete any post"}); err != nil {
		return err
	}
	if err := auth.AddChild("admin", "post/delete"); err != nil {
		// The edge may exist from a previous run; only a hierarchy without
		// it is a real failure.
		kids, listErr := auth.Children("admin")
		if listErr != nil {
			return listErr
		}
		present := false
		for _, kid := range kids {
			if kid.Name == "post/delete" {
				present = true
			}
		}
		if !present {
			return fmt.Errorf("seed admin -> post/delete: %w", err)
		}
	}

	admin, err := ar.New[User](ctx).Where("username = ?", "admin").One()
	if err != nil {
		return err
	}
	if err := auth.Assign("admin", admin.GetID()); err != nil && !errors.Is(err, rbac.ErrAlreadyAssigned) {
		return fmt.Errorf("seed admin assignment: %w", err)
	}
	fmt.Println("rbac: role admin with post/delete assigned to admin")
	return nil
}
