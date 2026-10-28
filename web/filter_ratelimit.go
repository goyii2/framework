package web

import (
	"math"
	"net"
	"net/http"
	"strconv"
	"sync"
	"time"
)

// RateStore counts requests per key. The middleware consults it once per
// request; implementations keep whatever state they need (an in-process
// sliding window here, a shared counter in Redis for multi-instance
// deployments later) and decide both admission and how long to wait.
type RateStore interface {
	// Allow consumes one event for key at now. It reports whether the
	// event fits the budget of limit events per window and, when it does
	// not, how long until the next slot frees up.
	Allow(key string, limit int, window time.Duration, now time.Time) (allowed bool, retryAfter time.Duration)
}

// SlidingWindowStore is an in-process RateStore keeping a sliding window of
// timestamps per key. Keys absent for a full window are pruned on touch so
// the map does not grow without bound.
type SlidingWindowStore struct {
	mu     sync.Mutex
	events map[string][]time.Time
}

// NewSlidingWindowStore creates an empty store.
func NewSlidingWindowStore() *SlidingWindowStore {
	return &SlidingWindowStore{events: map[string][]time.Time{}}
}

// Allow admits the event when fewer than limit events fall inside the
// window ending at now; otherwise it reports the wait until the oldest
// event ages out of the window.
func (s *SlidingWindowStore) Allow(key string, limit int, window time.Duration, now time.Time) (bool, time.Duration) {
	s.mu.Lock()
	defer s.mu.Unlock()

	start := now.Add(-window)
	events := s.events[key]
	kept := events[:0]
	for _, ts := range events {
		if ts.After(start) {
			kept = append(kept, ts)
		}
	}
	if len(kept) >= limit {
		s.events[key] = kept
		wait := kept[0].Add(window).Sub(now)
		if wait < 0 {
			wait = 0
		}
		return false, wait
	}
	s.events[key] = append(kept, now)
	return true, 0
}

// RateLimiter throttles requests per caller: when a caller exceeds Limit
// requests per Window, further requests are rejected 429 with a
// Retry-After header until enough old requests age out of the window. The
// default store is an in-process sliding window shared by every request
// through the middleware; swap in another RateStore (Redis-backed, ...) for
// cross-instance limits.
type RateLimiter struct {
	// Limit is the number of requests per caller per Window; default 60.
	Limit int

	// Window is the sliding window's span; default one minute.
	Window time.Duration

	// Key identifies the caller; default the request's remote IP (port
	// stripped).
	Key func(r *http.Request) string

	// Store counts requests; default one process-local sliding window.
	Store RateStore
}

// Middleware returns the rate limiting as middleware.
func (f RateLimiter) Middleware() Middleware {
	limit := f.Limit
	if limit <= 0 {
		limit = 60
	}
	window := f.Window
	if window <= 0 {
		window = time.Minute
	}
	key := f.Key
	if key == nil {
		key = func(r *http.Request) string {
			host, _, err := net.SplitHostPort(r.RemoteAddr)
			if err != nil {
				return r.RemoteAddr
			}
			return host
		}
	}
	store := f.Store
	if store == nil {
		store = NewSlidingWindowStore()
	}

	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			allowed, retryAfter := store.Allow(key(r), limit, window, time.Now())
			if allowed {
				next.ServeHTTP(w, r)
				return
			}
			w.Header().Set("Retry-After", strconv.Itoa(retrySeconds(retryAfter)))
			writeError(w, r, TooManyRequests("too many requests"))
		})
	}
}

// retrySeconds renders a duration as whole seconds, rounding up so the
// client retries no earlier than the slot frees.
func retrySeconds(d time.Duration) int {
	return int(math.Ceil(d.Seconds()))
}
