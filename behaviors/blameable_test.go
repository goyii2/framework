package behaviors

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"sync"
	"testing"

	"github.com/goyii2/framework/db/ar"
	"github.com/goyii2/framework/web"
)

func TestBlameableFillsFromContextIdentity(t *testing.T) {
	ctx := asUser(newTestContext(t), 42)

	doc := ar.New[auditDoc](ctx).New()
	doc.Note = "by 42"
	if err := doc.Save(); err != nil {
		t.Fatalf("insert: %v", err)
	}
	if doc.CreatedBy != 42 || doc.UpdatedBy != 42 {
		t.Errorf("stamps = (%d, %d), want (42, 42)", doc.CreatedBy, doc.UpdatedBy)
	}
}

func TestBlameableSwitchesIdentityOnUpdate(t *testing.T) {
	ctx := asUser(newTestContext(t), 7)

	doc := ar.New[auditDoc](ctx).New()
	doc.Note = "by 7"
	if err := doc.Save(); err != nil {
		t.Fatalf("insert: %v", err)
	}

	// Another operator loads and rewrites the row: creation attribution
	// stays with the original author, the modification moves to the new
	// one.
	other := asUser(ctx, 11)
	loaded, err := ar.FindByID[auditDoc](other, doc.ID)
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	loaded.Note = "by 11"
	if err := loaded.Save(); err != nil {
		t.Fatalf("update: %v", err)
	}
	if loaded.CreatedBy != 7 || loaded.UpdatedBy != 11 {
		t.Errorf("stamps = (%d, %d), want (7, 11)", loaded.CreatedBy, loaded.UpdatedBy)
	}
}

func TestBlameableWithoutIdentityFillsZero(t *testing.T) {
	ctx := newTestContext(t) // no identity anywhere

	guest := ar.New[auditDoc](ctx).New()
	guest.Note = "anonymous"
	if err := guest.Save(); err != nil {
		t.Fatalf("insert: %v", err)
	}
	if guest.CreatedBy != 0 || guest.UpdatedBy != 0 {
		t.Errorf("guest insert stamps = (%d, %d), want (0, 0)", guest.CreatedBy, guest.UpdatedBy)
	}

	// An update as a guest sets updated_by back to 0 — the fill is
	// unconditional, writing whatever the current identity resolves to.
	loaded, err := ar.FindByID[auditDoc](ctx, guest.ID)
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	loaded.Note = "still anonymous"
	if err := loaded.Save(); err != nil {
		t.Fatalf("update: %v", err)
	}
	if loaded.CreatedBy != 0 || loaded.UpdatedBy != 0 {
		t.Errorf("guest update stamps = (%d, %d), want (0, 0)", loaded.CreatedBy, loaded.UpdatedBy)
	}
}

func TestBlameableNonNumericIdentityFillsZero(t *testing.T) {
	ctx := webWithIdentity(newTestContext(t), fixedIdentity{"guid-abc"})

	doc := ar.New[auditDoc](ctx).New()
	doc.Note = "string id"
	if err := doc.Save(); err != nil {
		t.Fatalf("insert: %v", err)
	}
	if doc.CreatedBy != 0 {
		t.Errorf("CreatedBy = %d, want 0 for a non-numeric identity id", doc.CreatedBy)
	}
}

// TestBlameableReadsRequestIdentity runs the full pipeline: HTTP basic
// authentication establishes the identity, the action saves through the
// request context, Blameable resolves the identity from there.
func TestBlameableReadsRequestIdentity(t *testing.T) {
	ctx := newTestContext(t)
	handler := newWebApp(t, ctx)

	res, err := postAs(t, handler, "/note?note=hello", "alice")
	if err != nil {
		t.Fatalf("request: %v", err)
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(res.Body)
		t.Fatalf("status = %d, body %s", res.StatusCode, body)
	}
	var created struct {
		ID        int64 `json:"id"`
		CreatedBy int64 `json:"createdBy"`
		UpdatedBy int64 `json:"updatedBy"`
	}
	if err := json.NewDecoder(res.Body).Decode(&created); err != nil {
		t.Fatalf("decode body: %v", err)
	}
	if created.CreatedBy != 7 || created.UpdatedBy != 7 {
		t.Errorf("response stamps = (%d, %d), want (7, 7)", created.CreatedBy, created.UpdatedBy)
	}

	// A different authenticated user updates the same row through the
	// request path: created_by keeps alice, updated_by moves to bob.
	res2, err := postAs(t, handler, fmt.Sprintf("/note/%d", created.ID), "bob")
	if err != nil {
		t.Fatalf("request: %v", err)
	}
	defer res2.Body.Close()
	if res2.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(res2.Body)
		t.Fatalf("status = %d, body %s", res2.StatusCode, body)
	}
	var updated struct {
		CreatedBy int64 `json:"createdBy"`
		UpdatedBy int64 `json:"updatedBy"`
	}
	if err := json.NewDecoder(res2.Body).Decode(&updated); err != nil {
		t.Fatalf("decode body: %v", err)
	}
	if updated.CreatedBy != 7 || updated.UpdatedBy != 11 {
		t.Errorf("response stamps = (%d, %d), want (7, 11)", updated.CreatedBy, updated.UpdatedBy)
	}
}

// TestBlameableConcurrentIdentitySwitching hammers the handler from many
// goroutines at once, each under its own identity, each creating and then
// re-saving its own row. Run with -race: the identity resolution travels
// per-request contexts only, so switching identities concurrently must
// stay clean and every row must end up attributed to exactly its author.
func TestBlameableConcurrentIdentitySwitching(t *testing.T) {
	ctx := newTestContext(t)
	handler := newWebApp(t, ctx)
	srv := serve(t, handler)

	type result struct {
		createdBy int64
		updatedBy int64
		err       error
	}
	users := []struct {
		name string
		id   int64
	}{{"alice", 7}, {"bob", 11}, {"carol", 42}}

	const rounds = 8
	results := make([][]result, len(users))
	var wg sync.WaitGroup
	for i, user := range users {
		results[i] = make([]result, rounds)
		wg.Add(1)
		go func(slot int, name string) {
			defer wg.Done()
			client := &http.Client{}
			for r := 0; r < rounds; r++ {
				var row result
				func() {
					req, err := http.NewRequest(http.MethodPost, srv.URL+"/note?note=x", nil)
					if err != nil {
						row.err = err
						return
					}
					req.SetBasicAuth(name, "pw")
					res, err := client.Do(req)
					if err != nil {
						row.err = err
						return
					}
					defer res.Body.Close()
					var created struct {
						ID        int64 `json:"id"`
						CreatedBy int64 `json:"createdBy"`
					}
					if err := json.NewDecoder(res.Body).Decode(&created); err != nil {
						row.err = err
						return
					}
					row.createdBy = created.CreatedBy

					req2, err := http.NewRequest(http.MethodPost, srv.URL+fmt.Sprintf("/note/%d", created.ID), nil)
					if err != nil {
						row.err = err
						return
					}
					req2.SetBasicAuth(name, "pw")
					res2, err := client.Do(req2)
					if err != nil {
						row.err = err
						return
					}
					defer res2.Body.Close()
					var updated struct {
						UpdatedBy int64 `json:"updatedBy"`
					}
					if err := json.NewDecoder(res2.Body).Decode(&updated); err != nil {
						row.err = err
						return
					}
					row.updatedBy = updated.UpdatedBy
				}()
				results[slot][r] = row
			}
		}(i, user.name)
	}
	wg.Wait()

	for slot, user := range users {
		for r, row := range results[slot] {
			if row.err != nil {
				t.Fatalf("user %s round %d: %v", user.name, r, row.err)
			}
			if row.createdBy != user.id || row.updatedBy != user.id {
				t.Errorf("user %s round %d: stamps = (%d, %d), want (%d, %d)",
					user.name, r, row.createdBy, row.updatedBy, user.id, user.id)
			}
		}
	}

	total, err := ar.New[auditDoc](ctx).Count()
	if err != nil {
		t.Fatalf("count: %v", err)
	}
	if want := int64(len(users) * rounds); total != want {
		t.Errorf("rows = %d, want %d", total, want)
	}
}

// fixedIdentity is an identity whose id does not parse as a number.
type fixedIdentity struct{ id string }

func (u fixedIdentity) GetID() string      { return u.id }
func (u fixedIdentity) GetName() string    { return u.id }
func (u fixedIdentity) GetRoles() []string { return nil }

// webWithIdentity publishes an arbitrary Identity on ctx.
func webWithIdentity(ctx context.Context, identity web.Identity) context.Context {
	return web.WithIdentity(ctx, identity)
}
