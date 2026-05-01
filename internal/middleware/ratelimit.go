package middleware

import (
	"encoding/json"
	"log/slog"
	"net"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"
)

type RateLimiter struct {
	visitors       map[string]*visitor
	mu             sync.Mutex
	rate           int
	window         time.Duration
	trustForwarded bool
	allowedIPs     map[string]bool
	logger         *slog.Logger
}

type visitor struct {
	count    int
	lastSeen time.Time
}

func NewRateLimiter(rate int, window time.Duration) *RateLimiter {
	rl := &RateLimiter{
		visitors:   make(map[string]*visitor),
		rate:       rate,
		window:     window,
		allowedIPs: make(map[string]bool),
	}
	go rl.cleanup()
	return rl
}

func (rl *RateLimiter) WithAllowedIPs(ips []string) *RateLimiter {
	for _, ip := range ips {
		rl.allowedIPs[strings.TrimSpace(ip)] = true
	}
	return rl
}

func (rl *RateLimiter) WithLogger(logger *slog.Logger) *RateLimiter {
	rl.logger = logger
	return rl
}

func (rl *RateLimiter) Cleanup() {
	rl.mu.Lock()
	defer rl.mu.Unlock()
	for ip, v := range rl.visitors {
		if time.Since(v.lastSeen) > rl.window {
			delete(rl.visitors, ip)
		}
	}
}

func (rl *RateLimiter) cleanup() {
	ticker := time.NewTicker(rl.window)
	defer ticker.Stop()
	for range ticker.C {
		rl.Cleanup()
	}
}

func (rl *RateLimiter) Middleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/health" || r.URL.Path == "/metrics" {
			next.ServeHTTP(w, r)
			return
		}

		ip := extractIP(r)

		if rl.allowedIPs[ip] {
			next.ServeHTTP(w, r)
			return
		}

		rl.mu.Lock()
		v, exists := rl.visitors[ip]
		if !exists || time.Since(v.lastSeen) > rl.window {
			rl.visitors[ip] = &visitor{count: 1, lastSeen: time.Now()}
			remaining := rl.rate - 1
			rl.mu.Unlock()
			w.Header().Set("X-RateLimit-Limit", strconv.Itoa(rl.rate))
			w.Header().Set("X-RateLimit-Remaining", strconv.Itoa(remaining))
			next.ServeHTTP(w, r)
			return
		}

		v.count++
		v.lastSeen = time.Now()
		remaining := rl.rate - v.count
		if remaining < 0 {
			remaining = 0
		}
		w.Header().Set("X-RateLimit-Limit", strconv.Itoa(rl.rate))
		w.Header().Set("X-RateLimit-Remaining", strconv.Itoa(remaining))

		if v.count > rl.rate {
			retryAfter := int(rl.window.Seconds())
			w.Header().Set("Retry-After", strconv.Itoa(retryAfter))
			rl.mu.Unlock()
			if rl.logger != nil {
				rl.logger.Warn("rate limit exceeded", slog.String("ip", ip), slog.String("path", r.URL.Path))
			}
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusTooManyRequests)
			json.NewEncoder(w).Encode(map[string]any{
				"success": false,
				"error": map[string]string{
					"code":    "rate_limit_exceeded",
					"message": "Terlalu banyak permintaan. Silakan coba lagi nanti.",
				},
			})
			return
		}
		rl.mu.Unlock()

		next.ServeHTTP(w, r)
	})
}

func extractIP(r *http.Request) string {
	ip := r.RemoteAddr
	if host, _, err := net.SplitHostPort(ip); err == nil {
		ip = host
	}
	if ip == "127.0.0.1" || ip == "::1" {
		if forwarded := r.Header.Get("X-Forwarded-For"); forwarded != "" {
			first := strings.Split(forwarded, ",")[0]
			if parsed := net.ParseIP(strings.TrimSpace(first)); parsed != nil {
				return parsed.String()
			}
		}
	}
	return ip
}
