package api

import (
	"log/slog"
	"net"
	"net/http"
	"sync"
	"time"

	"golang.org/x/time/rate"
)

// ipRateLimiter gives every client IP its own token bucket. A bucket holds up
// to burst tokens and refills at rps tokens per second; each request spends
// one. That allows short bursts (a page loading a few things at once) while
// capping the sustained rate, so one aggressive client can't starve the rest
// or run up the database.
type ipRateLimiter struct {
	rps   rate.Limit
	burst int

	mu          sync.Mutex
	clients     map[string]*clientBucket
	lastCleanup time.Time
}

type clientBucket struct {
	limiter  *rate.Limiter
	lastSeen time.Time
}

const idleClientTTL = 3 * time.Minute

func newIPRateLimiter(rps float64, burst int) *ipRateLimiter {
	return &ipRateLimiter{
		rps:         rate.Limit(rps),
		burst:       burst,
		clients:     make(map[string]*clientBucket),
		lastCleanup: time.Now(),
	}
}

func (l *ipRateLimiter) allow(ip string) bool {
	l.mu.Lock()
	defer l.mu.Unlock()

	now := time.Now()
	// Forget clients we haven't heard from in a while, or the map would grow
	// by one entry for every IP address that ever made a request. Sweeping
	// here, at most once a minute, avoids needing a background goroutine.
	if now.Sub(l.lastCleanup) > time.Minute {
		for ip, c := range l.clients {
			if now.Sub(c.lastSeen) > idleClientTTL {
				delete(l.clients, ip)
			}
		}
		l.lastCleanup = now
	}

	c, ok := l.clients[ip]
	if !ok {
		c = &clientBucket{limiter: rate.NewLimiter(l.rps, l.burst)}
		l.clients[ip] = c
	}
	c.lastSeen = now
	return c.limiter.Allow()
}

func (l *ipRateLimiter) middleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !l.allow(clientIP(r)) {
			w.Header().Set("Retry-After", "1")
			writeError(w, http.StatusTooManyRequests, "rate limit exceeded, slow down")
			return
		}
		next.ServeHTTP(w, r)
	})
}

// clientIP uses the TCP connection's address. It deliberately ignores the
// X-Forwarded-For header: any client can set that header to anything, so
// trusting it would let an attacker dodge the limit by sending a new fake IP
// with every request. Behind a real load balancer, this should read the
// header that load balancer sets, and only that one.
func clientIP(r *http.Request) string {
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return host
}

// logRequests writes one structured log line per request.
func logRequests(log *slog.Logger, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		rec := &statusRecorder{ResponseWriter: w, status: http.StatusOK}
		next.ServeHTTP(rec, r)
		log.Info("request",
			"method", r.Method,
			"path", r.URL.Path,
			"query", r.URL.RawQuery,
			"status", rec.status,
			"duration", time.Since(start).Round(time.Microsecond),
			"ip", clientIP(r),
		)
	})
}

// statusRecorder remembers the status code, which http.ResponseWriter
// otherwise doesn't let you read back after it's written.
type statusRecorder struct {
	http.ResponseWriter
	status int
}

func (r *statusRecorder) WriteHeader(code int) {
	r.status = code
	r.ResponseWriter.WriteHeader(code)
}
