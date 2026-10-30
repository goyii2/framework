// Package commands holds the advanced example's user-defined console
// commands — the counterparts of the Yii2 advanced template's console
// controllers, living in the same binary as serve and migrate. This file
// carries user/create, the account-provisioning command the template ships
// with; seed joins in its own slice.
package commands

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/goyii2/framework/db/ar"
	"github.com/goyii2/framework/examples/advanced/common/models"
)

// NewUserCreateCommand builds the "user/create" console command — named
// after the Yii2 route it maps onto (console controllers there answer as
// controller/action). It runs through the same application assembly as
// serve, so the model's insert validation (username present, long enough
// and unique) and bcrypt hashing apply on the console side too.
func NewUserCreateCommand() *cobra.Command {
	var username, password, role string
	cmd := &cobra.Command{
		Use:   "user/create",
		Short: "Create a user account",
		RunE: func(cmd *cobra.Command, _ []string) error {
			ctx := cmd.Context() // carries the application (see console.ExecuteContext)

			user := ar.New[models.User](ctx).New()
			user.Username, user.Role = username, role
			if err := user.SetPassword(password); err != nil {
				return err
			}
			if err := user.Save(); err != nil {
				return fmt.Errorf("create user %s: %w", username, err)
			}
			fmt.Fprintf(cmd.OutOrStdout(), "created user %s (id %d, role %s)\n", user.Username, user.ID, user.Role)
			return nil
		},
	}
	cmd.Flags().StringVar(&username, "username", "", "login name of the account (required, unique)")
	cmd.Flags().StringVar(&password, "password", "", "plaintext password, stored as a bcrypt hash (required)")
	cmd.Flags().StringVar(&role, "role", "user", "role driving access control (the backend module requires admin)")
	_ = cmd.MarkFlagRequired("username")
	_ = cmd.MarkFlagRequired("password")
	return cmd
}
