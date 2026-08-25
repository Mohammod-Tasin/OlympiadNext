package middleware

import (
	"net"
	"net/http"
	"sync"
	"time"

	"golang.org/x/time/rate"

	"olympiadnext/internal/http/response"
)

const (
	janitorInterval = 10 * time.Minute
	staleAfter      = 15 * time.Minute
)

type limiterEntry struct {
	limiter  *rate.Limiter
	lastSeen time.Time
}

// RateLimitByIP throttles auth endpoints per client IP to blunt
// credential-stuffing and brute-force attempts. State is in-process,
// which is sufficient for a single instance; a multi-instance
// deployment should move this to a shared store (e.g. Redis). A
// background janitor prunes IPs not seen recently so the map doesn't
// grow unbounded for the lifetime of the process.
func RateLimitByIP(requestsPerMinute int, burst int) func(http.Handler) http.Handler {
	var (
		mu       sync.RWMutex
		limiters = make(map[string]*limiterEntry)
	)

	getLimiter := func(ip string) *rate.Limiter {
		mu.Lock()
		defer mu.Unlock()
		entry, ok := limiters[ip]
		if !ok {
			entry = &limiterEntry{
				limiter: rate.NewLimiter(rate.Every(time.Minute/time.Duration(requestsPerMinute)), burst),
			}
			limiters[ip] = entry
		}
		entry.lastSeen = time.Now()
		return entry.limiter
	}

	go func() {
		ticker := time.NewTicker(janitorInterval)
		defer ticker.Stop()
		for range ticker.C {
			cutoff := time.Now().Add(-staleAfter)
			mu.Lock()
			for ip, entry := range limiters {
				if entry.lastSeen.Before(cutoff) {
					delete(limiters, ip)
				}
			}
			mu.Unlock()
		}
	}()

	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			ip := clientIP(r)
			if !getLimiter(ip).Allow() {
				response.Error(w, http.StatusTooManyRequests, "too many requests, please try again later")
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}

func clientIP(r *http.Request) string {
	if host, _, err := net.SplitHostPort(r.RemoteAddr); err == nil {
		return host
	}
	return r.RemoteAddr
}
