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

// demoAccounts are the seed users. The RBAC hierarchy decides what each of
// them may do: admin inherits the editor role and adds post/delete, editor
// writes (post/create, post/update), demo proves the plain authenticated
// role that holds no extra permission.
var demoAccounts = []struct {
	username string
	password string
	role     string
}{
	{"admin", "admin123", "admin"},
	{"editor", "editor123", "editor"},
	{"demo", "demo123", "user"},
}

// seedPostCount is the number of generated sample posts. Enough rows to make
// the LinkPager step across several pages at the default page size.
const seedPostCount = 24

// newSeedCommand builds the "seed" console command.
func newSeedCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "seed",
		Short: "Create the demo accounts, RBAC data and sample posts",
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
				fmt.Fprintln(cmd.OutOrStdout(), "posts already present, skipping sample posts")
				return seedRbac(ctx)
			}
			admin, err := ar.New[User](ctx).Where("username = ?", "admin").One()
			if err != nil {
				return err
			}
			editor, err := ar.New[User](ctx).Where("username = ?", "editor").One()
			if err != nil {
				return err
			}

			post := ar.New[Post](ctx).New()
			post.Title = "Hello goyii"
			post.Body = "This post was created by the seed command. Create your own through POST /post or the New post page."
			post.Status = PostStatusPublished
			post.AuthorID = admin.ID
			if err := post.Save(); err != nil {
				return fmt.Errorf("seed example post: %w", err)
			}
			fmt.Fprintf(cmd.OutOrStdout(), "created example post %d\n", post.ID)

			for i := 1; i <= seedPostCount; i++ {
				sample := ar.New[Post](ctx).New()
				sample.Title = fmt.Sprintf("Sample post %02d", i)
				sample.Body = fmt.Sprintf("Sample body of post %02d, seeded so pagination and sorting have data to work with.", i)
				sample.Status = PostStatusPublished
				if i%4 == 0 {
					sample.Status = PostStatusDraft
				}
				sample.AuthorID = admin.ID
				if i%2 == 0 {
					sample.AuthorID = editor.ID
				}
				if err := sample.Save(); err != nil {
					return fmt.Errorf("seed sample post %d: %w", i, err)
				}
			}
			fmt.Fprintf(cmd.OutOrStdout(), "created %d sample posts\n", seedPostCount)
			return seedRbac(ctx)
		},
	}
}

// seedRbac fills the authorization tables with a small hierarchy:
//
//	admin -> editor -> post/create, post/update
//	admin -> post/delete
//
// The admin account carries the admin role and inherits everything below it
// (indirect grants through the item graph); the editor account carries the
// editor role. The access rules of PostController and the navigation menu
// ask for these items by name. Re-running against an already-seeded database
// changes nothing.
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
	items := []rbac.Item{
		{Name: "admin", Type: rbac.TypeRole, Description: "may do everything"},
		{Name: "editor", Type: rbac.TypeRole, Description: "may write posts"},
		{Name: "post/create", Type: rbac.TypePermission, Description: "may create posts"},
		{Name: "post/update", Type: rbac.TypePermission, Description: "may edit any post"},
		{Name: "post/delete", Type: rbac.TypePermission, Description: "may delete any post"},
	}
	for _, item := range items {
		if err := ensureItem(item); err != nil {
			return err
		}
	}

	ensureChild := func(parent, child string) error {
		if err := auth.AddChild(parent, child); err != nil {
			// The edge may exist from a previous run; only a hierarchy
			// without it is a real failure.
			kids, listErr := auth.Children(parent)
			if listErr != nil {
				return listErr
			}
			for _, kid := range kids {
				if kid.Name == child {
					return nil
				}
			}
			return fmt.Errorf("seed %s -> %s: %w", parent, child, err)
		}
		return nil
	}
	edges := [][2]string{
		{"admin", "editor"},
		{"admin", "post/delete"},
		{"editor", "post/create"},
		{"editor", "post/update"},
	}
	for _, edge := range edges {
		if err := ensureChild(edge[0], edge[1]); err != nil {
			return err
		}
	}

	assign := func(role, username string) error {
		user, err := ar.New[User](ctx).Where("username = ?", username).One()
		if err != nil {
			return err
		}
		if err := auth.Assign(role, user.GetID()); err != nil && !errors.Is(err, rbac.ErrAlreadyAssigned) {
			return fmt.Errorf("seed %s assignment: %w", role, err)
		}
		return nil
	}
	if err := assign("admin", "admin"); err != nil {
		return err
	}
	if err := assign("editor", "editor"); err != nil {
		return err
	}
	fmt.Println("rbac: admin (with editor) and editor roles assigned")
	return nil
}
