package api

import (
	"log/slog"
	"net"
	"net/http"
	"strings"
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
	rps      rate.Limit
	burst    int
	clientIP func(*http.Request) string

	mu          sync.Mutex
	clients     map[string]*clientBucket
	lastCleanup time.Time
}

type clientBucket struct {
	limiter  *rate.Limiter
	lastSeen time.Time
}

const idleClientTTL = 3 * time.Minute

func newIPRateLimiter(rps float64, burst int, clientIP func(*http.Request) string) *ipRateLimiter {
	return &ipRateLimiter{
		rps:         rate.Limit(rps),
		burst:       burst,
		clientIP:    clientIP,
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
		if !l.allow(l.clientIP(r)) {
			w.Header().Set("Retry-After", "1")
			writeError(w, http.StatusTooManyRequests, "rate limit exceeded, slow down")
			return
		}
		next.ServeHTTP(w, r)
	})
}

// clientIPFunc returns how to find a request's client IP. By default it's
// the TCP connection's address. Headers like X-Forwarded-For are ignored on
// purpose: any client can set them to anything, so trusting them would let
// an attacker dodge the rate limit by sending a new fake IP every time.
//
// Behind a reverse proxy, though, every connection comes from the proxy,
// and all clients would share one rate limit. Then header names the header
// the proxy sets to the real client IP. Set it only when the proxy is the
// sole way in, so no client can reach the API and set the header itself.
func clientIPFunc(header string) func(*http.Request) string {
	return func(r *http.Request) string {
		if header != "" {
			if ip := strings.TrimSpace(r.Header.Get(header)); ip != "" {
				return ip
			}
		}
		host, _, err := net.SplitHostPort(r.RemoteAddr)
		if err != nil {
			return r.RemoteAddr
		}
		return host
	}
}

// logRequests writes one structured log line per request.
func logRequests(log *slog.Logger, clientIP func(*http.Request) string, next http.Handler) http.Handler {
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
