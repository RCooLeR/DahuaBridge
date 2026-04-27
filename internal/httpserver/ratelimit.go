package httpserver

import (
	"math"
	"net"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"
)

type perClientRateLimiter struct {
	mu         sync.Mutex
	perSecond  float64
	burst      float64
	staleAfter time.Duration
	clients    map[string]*rateBucket
}

type rateBucket struct {
	tokens   float64
	last     time.Time
	lastSeen time.Time
}

func newPerClientRateLimiter(perMinute int, burst int) *perClientRateLimiter {
	if perMinute <= 0 || burst <= 0 {
		return nil
	}
	return &perClientRateLimiter{
		perSecond:  float64(perMinute) / 60.0,
		burst:      float64(burst),
		staleAfter: 15 * time.Minute,
		clients:    make(map[string]*rateBucket),
	}
}

func (l *perClientRateLimiter) Allow(key string) (bool, time.Duration) {
	if l == nil {
		return true, 0
	}

	now := time.Now()

	l.mu.Lock()
	defer l.mu.Unlock()

	for clientKey, bucket := range l.clients {
		if now.Sub(bucket.lastSeen) > l.staleAfter {
			delete(l.clients, clientKey)
		}
	}

	bucket, ok := l.clients[key]
	if !ok {
		bucket = &rateBucket{
			tokens:   l.burst,
			last:     now,
			lastSeen: now,
		}
		l.clients[key] = bucket
	}

	elapsed := now.Sub(bucket.last).Seconds()
	if elapsed > 0 {
		bucket.tokens = math.Min(l.burst, bucket.tokens+(elapsed*l.perSecond))
	}
	bucket.last = now
	bucket.lastSeen = now

	if bucket.tokens >= 1 {
		bucket.tokens -= 1
		return true, 0
	}

	missing := 1 - bucket.tokens
	retryAfter := time.Second
	if l.perSecond > 0 {
		retryAfter = time.Duration(math.Ceil(missing/l.perSecond)) * time.Second
		if retryAfter < time.Second {
			retryAfter = time.Second
		}
	}
	return false, retryAfter
}

func rateLimitMiddleware(limiter *perClientRateLimiter) func(http.Handler) http.Handler {
	if limiter == nil {
		return func(next http.Handler) http.Handler {
			return next
		}
	}

	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			allowed, retryAfter := limiter.Allow(clientIP(r))
			if !allowed {
				retryAfterSeconds := int(retryAfter / time.Second)
				if retryAfterSeconds <= 0 {
					retryAfterSeconds = 1
				}
				w.Header().Set("Retry-After", strconv.Itoa(retryAfterSeconds))
				writeJSON(w, http.StatusTooManyRequests, map[string]any{
					"error":               "rate limit exceeded",
					"retry_after_seconds": retryAfterSeconds,
				})
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}

func clientIP(r *http.Request) string {
	if forwarded := strings.TrimSpace(r.Header.Get("X-Forwarded-For")); forwarded != "" {
		parts := strings.Split(forwarded, ",")
		if len(parts) > 0 && strings.TrimSpace(parts[0]) != "" {
			return strings.TrimSpace(parts[0])
		}
	}

	if realIP := strings.TrimSpace(r.Header.Get("X-Real-IP")); realIP != "" {
		return realIP
	}

	host, _, err := net.SplitHostPort(strings.TrimSpace(r.RemoteAddr))
	if err == nil && host != "" {
		return host
	}

	if remoteAddr := strings.TrimSpace(r.RemoteAddr); remoteAddr != "" {
		return remoteAddr
	}

	return "unknown"
}
