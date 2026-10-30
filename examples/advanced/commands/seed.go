// The seed command of the advanced example: fills the freshly migrated
// database with the demo accounts, the RBAC grant the backend guard reads
// and the demo content the frontend shows. Roles are assignments in the
// authorization tables, never account attributes — this command is the one
// place the demo admin role is granted. Re-running against an
// already-seeded database changes nothing.
package commands

import (
	"context"
	"errors"
	"fmt"
	"io"

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

// demoPosts are the seeded content: published posts the frontend lists
// anonymously and a draft only the backend sees. The seeded admin writes
// them, so every row demonstrates the author association.
var demoPosts = []struct {
	title  string
	body   string
	status string
}{
	{
		title:  "Welcome to the advanced demo",
		body:   "This post was seeded by the seed command. Sign in as admin, open /backend and edit or delete it — the frontend list follows the database.",
		status: models.PostStatusPublished,
	},
	{
		title:  "One process, two applications",
		body:   "The frontend answers at the root scope and the backend lives under /backend as a module behind the admin role — the Yii2 advanced template's shape on one port.",
		status: models.PostStatusPublished,
	},
	{
		title:  "Draft: behind the scenes",
		body:   "Drafts stay invisible on the frontend until the backend publishes them — flip this row's status to published and it appears on the site.",
		status: models.PostStatusDraft,
	},
}

// NewSeedCommand builds the "seed" console command.
func NewSeedCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "seed",
		Short: "Create the demo accounts, grant the admin role and seed demo posts",
		RunE: func(cmd *cobra.Command, _ []string) error {
			ctx := cmd.Context() // carries the application (see console.ExecuteContext)
			out := cmd.OutOrStdout()

			for _, acc := range demoAccounts {
				n, err := ar.New[models.User](ctx).Where("username = ?", acc.username).Count()
				if err != nil {
					return err
				}
				if n > 0 {
					fmt.Fprintf(out, "user %s already exists\n", acc.username)
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
				fmt.Fprintf(out, "created user %s (id %d)\n", user.Username, user.ID)
			}
			if err := seedRbac(ctx); err != nil {
				return err
			}
			return seedPosts(ctx, out)
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

// seedPosts creates the demo content with the seeded admin as its author.
// A post whose title already exists is left as it is — the same
// already-seeded no-op the accounts promise, so an edited demo post
// survives a re-run.
func seedPosts(ctx context.Context, out io.Writer) error {
	admin, err := ar.New[models.User](ctx).Where("username = ?", "admin").One()
	if err != nil {
		return err
	}
	for _, p := range demoPosts {
		n, err := ar.New[models.Post](ctx).Where("title = ?", p.title).Count()
		if err != nil {
			return err
		}
		if n > 0 {
			fmt.Fprintf(out, "post %q already exists\n", p.title)
			continue
		}
		post := ar.New[models.Post](ctx).New()
		post.Title, post.Body, post.Status = p.title, p.body, p.status
		post.AuthorID = admin.ID
		if err := post.Save(); err != nil {
			return fmt.Errorf("seed post %q: %w", p.title, err)
		}
		fmt.Fprintf(out, "created post %q (id %d)\n", post.Title, post.ID)
	}
	return nil
}
