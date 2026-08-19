package middleware

import (
	"net"
	"net/http"
	"sync"
	"time"

	"golang.org/x/time/rate"

	"olympiadnext/internal/http/response"
)

// RateLimitByIP throttles auth endpoints per client IP to blunt
// credential-stuffing and brute-force attempts. State is in-process,
// which is sufficient for a single instance; a multi-instance
// deployment should move this to a shared store (e.g. Redis).
func RateLimitByIP(requestsPerMinute int, burst int) func(http.Handler) http.Handler {
	var (
		mu       sync.Mutex
		limiters = make(map[string]*rate.Limiter)
	)

	getLimiter := func(ip string) *rate.Limiter {
		mu.Lock()
		defer mu.Unlock()
		l, ok := limiters[ip]
		if !ok {
			l = rate.NewLimiter(rate.Every(time.Minute/time.Duration(requestsPerMinute)), burst)
			limiters[ip] = l
		}
		return l
	}

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
