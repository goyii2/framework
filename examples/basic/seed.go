// The seed command: a user-defined console command filling the freshly
// migrated database with demo data. It runs through the same application
// assembly as serve, so the Active Record layer (and its insert validation)
// applies here too.
package main

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/goyii2/framework/db/ar"
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
				return nil
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
			return nil
		},
	}
}
