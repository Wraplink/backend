package security

import (
	"net"
	"net/http"
	"sync"
	"time"
)

type rateLimitEntry struct {
	count       int
	windowStart time.Time
}

type RateLimiter struct {
	mu      sync.Mutex
	entries map[string]rateLimitEntry

	limit  int
	window time.Duration
}

func NewRateLimiter(limit int, window time.Duration) *RateLimiter {
	return &RateLimiter{
		entries: make(map[string]rateLimitEntry),
		limit:   limit,
		window:  window,
	}
}

func (l *RateLimiter) Allow(key string) bool {
	now := time.Now()

	l.mu.Lock()
	defer l.mu.Unlock()

	entry, exists := l.entries[key]

	if !exists || now.Sub(entry.windowStart) >= l.window {
		l.entries[key] = rateLimitEntry{
			count:       1,
			windowStart: now,
		}
		return true
	}

	if entry.count >= l.limit {
		return false
	}

	entry.count++
	l.entries[key] = entry

	return true
}

func (l *RateLimiter) Middleware(
	keyFunc func(*http.Request) string,
) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(
			w http.ResponseWriter,
			r *http.Request,
		) {
			key := keyFunc(r)

			if !l.Allow(key) {
				w.Header().Set("Retry-After", "60")
				http.Error(
					w,
					`{"error":"too many requests"}`,
					http.StatusTooManyRequests,
				)
				return
			}

			next.ServeHTTP(w, r)
		})
	}
}

func RemoteIP(r *http.Request) string {
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}

	return host
}
