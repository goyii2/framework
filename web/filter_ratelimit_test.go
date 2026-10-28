package web_test

import (
	"net/http"
	"net/http/httptest"
	"strconv"
	"sync"
	"testing"
	"time"

	"github.com/goyii2/framework/base"
	"github.com/goyii2/framework/config"
	"github.com/goyii2/framework/web"
)

func TestRateLimiterAdmitsUpToLimitThenRejects429(t *testing.T) {
	app := base.New(config.Config{Web: config.Web{DisableCsrf: true}})
	h := web.NewHandler(app)
	h.Use(web.RateLimiter{Limit: 2, Window: time.Minute}.Middleware())
	h.Get("/hit", func(c *web.Context) error { return c.JSON(map[string]string{"ok": "yes"}) })

	for i := 1; i <= 2; i++ {
		if res := req(t, h, http.MethodGet, "/hit"); res.StatusCode != http.StatusOK {
			t.Fatalf("request %d status = %d, want 200 (within budget)", i, res.StatusCode)
		}
	}
	res := req(t, h, http.MethodGet, "/hit")
	if res.StatusCode != http.StatusTooManyRequests {
		t.Fatalf("status = %d, want 429 beyond the budget", res.StatusCode)
	}
	retry, err := strconv.Atoi(res.Header.Get("Retry-After"))
	if err != nil || retry <= 0 || retry > 60 {
		t.Errorf("Retry-After = %q (err %v), want 1..60 seconds", res.Header.Get("Retry-After"), err)
	}
}

func TestRateLimiterKeysSeparateCallers(t *testing.T) {
	app := base.New(config.Config{Web: config.Web{DisableCsrf: true}})
	h := web.NewHandler(app)
	h.Use(web.RateLimiter{
		Limit:  1,
		Window: time.Minute,
		Key:    func(r *http.Request) string { return r.Header.Get("X-Key") },
	}.Middleware())
	h.Get("/hit", func(c *web.Context) error { return c.JSON(map[string]string{"ok": "yes"}) })

	for _, key := range []string{"alice", "bob"} {
		r := httptest.NewRequest(http.MethodGet, "/hit", nil)
		r.Header.Set("X-Key", key)
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, r)
		if rec.Code != http.StatusOK {
			t.Fatalf("first request of %s status = %d, want 200 (budgets are per caller)", key, rec.Code)
		}
	}
}

func TestSlidingWindowStoreAdmissionAndExpiry(t *testing.T) {
	store := web.NewSlidingWindowStore()
	now := time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)

	for i := 1; i <= 3; i++ {
		ok, _ := store.Allow("k", 3, time.Minute, now)
		if !ok {
			t.Fatalf("event %d denied, want admitted (limit 3)", i)
		}
	}
	ok, retry := store.Allow("k", 3, time.Minute, now.Add(10*time.Second))
	if ok || retry != 50*time.Second {
		t.Fatalf("fourth event = %v, retry %v; want denied with 50s wait", ok, retry)
	}
	// Sliding: the window ends at now+60s; all three events aged out by
	// now+61s, so the budget resets.
	ok, _ = store.Allow("k", 3, time.Minute, now.Add(61*time.Second))
	if !ok {
		t.Fatal("event after the window slid past all events was denied")
	}
}

func TestSlidingWindowStoreSlidingOverlap(t *testing.T) {
	store := web.NewSlidingWindowStore()
	t0 := time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)

	// Fill the budget at t0.
	store.Allow("k", 2, time.Minute, t0)
	store.Allow("k", 2, time.Minute, t0)
	if ok, _ := store.Allow("k", 2, time.Minute, t0.Add(30*time.Second)); ok {
		t.Fatal("admitted at +30s although both t0 events are inside the window")
	}
	// At +61s both t0 events slid out: the budget is back.
	if ok, _ := store.Allow("k", 2, time.Minute, t0.Add(61*time.Second)); !ok {
		t.Fatal("denied at +61s although the window slid past the t0 events")
	}
	// One slot stays consumed by the +61s event.
	if ok, _ := store.Allow("k", 2, time.Minute, t0.Add(62*time.Second)); !ok {
		t.Fatal("denied although one slot remained after the slide")
	}
	if ok, _ := store.Allow("k", 2, time.Minute, t0.Add(62*time.Second)); ok {
		t.Fatal("admitted beyond limit after the slide")
	}
}

func TestSlidingWindowStoreConcurrentAccess(t *testing.T) {
	store := web.NewSlidingWindowStore()
	now := time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)

	const goroutines, limit = 200, 10
	var mu sync.Mutex
	admitted := 0
	var wg sync.WaitGroup
	for i := 0; i < goroutines; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			ok, _ := store.Allow("shared", limit, time.Minute, now)
			if ok {
				mu.Lock()
				admitted++
				mu.Unlock()
			}
		}()
	}
	wg.Wait()
	if admitted != limit {
		t.Errorf("admitted %d of %d concurrent events, want exactly the limit %d", admitted, goroutines, limit)
	}
}
