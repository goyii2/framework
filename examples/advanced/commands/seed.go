// The seed command of the advanced example: fills the freshly migrated
// database with the demo accounts and the RBAC grant the backend guard
// reads. Roles are assignments in the authorization tables, never account
// attributes — this command is the one place the demo admin role is
// granted. Re-running against an already-seeded database changes nothing.
package commands

import (
	"context"
	"errors"
	"fmt"

	"github.com/spf13/cobra"

	"github.com/goyii2/framework/base"
	"github.com/goyii2/framework/db/ar"
	"github.com/goyii2/framework/examples/advanced/common/models"
	"github.com/goyii2/framework/rbac"
)

// demoAccounts are the seeded users. admin carries the admin role (the
// grant lives in seedRbac); demo holds no role at all — the plain
// authenticated account that demonstrates the backend guard's 403 state.
var demoAccounts = []struct {
	username string
	password string
}{
	{"admin", "admin123"},
	{"demo", "demo123"},
}

// NewSeedCommand builds the "seed" console command.
func NewSeedCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "seed",
		Short: "Create the demo accounts and grant the admin role",
		RunE: func(cmd *cobra.Command, _ []string) error {
			ctx := cmd.Context() // carries the application (see console.ExecuteContext)

			for _, acc := range demoAccounts {
				n, err := ar.New[models.User](ctx).Where("username = ?", acc.username).Count()
				if err != nil {
					return err
				}
				if n > 0 {
					fmt.Fprintf(cmd.OutOrStdout(), "user %s already exists\n", acc.username)
					continue
				}
				user := ar.New[models.User](ctx).New()
				user.Username = acc.username
				if err := user.SetPassword(acc.password); err != nil {
					return err
				}
				if err := user.Save(); err != nil {
					return fmt.Errorf("seed user %s: %w", acc.username, err)
				}
				fmt.Fprintf(cmd.OutOrStdout(), "created user %s (id %d)\n", user.Username, user.ID)
			}
			return seedRbac(ctx)
		},
	}
}

// seedRbac grants the admin role to the seeded admin account: the role
// item is created (idempotently) and the assignment lands in
// auth_assignment — the row the backend module's access rule resolves
// through the auth manager. demo deliberately receives nothing.
func seedRbac(ctx context.Context) error {
	conn := base.FromContext(ctx).DB()
	auth, err := rbac.NewDbManager(conn.DB)
	if err != nil {
		return err
	}

	if err := auth.AddItem(rbac.Item{
		Name:        "admin",
		Type:        rbac.TypeRole,
		Description: "may enter the backend",
	}); err != nil && !errors.Is(err, rbac.ErrItemExists) {
		return fmt.Errorf("seed item admin: %w", err)
	}

	admin, err := ar.New[models.User](ctx).Where("username = ?", "admin").One()
	if err != nil {
		return err
	}
	if err := auth.Assign("admin", admin.GetID()); err != nil && !errors.Is(err, rbac.ErrAlreadyAssigned) {
		return fmt.Errorf("seed admin assignment: %w", err)
	}
	fmt.Println("rbac: admin role granted to admin")
	return nil
}
