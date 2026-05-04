package middleware

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestRateLimiter_AllowsWithinLimit(t *testing.T) {
	rl := NewRateLimiter(5, time.Minute)

	for i := 0; i < 5; i++ {
		req := httptest.NewRequest(http.MethodGet, "/", nil)
		req.RemoteAddr = "127.0.0.1:1234"
		w := httptest.NewRecorder()

		handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusOK)
		})
		rl.Middleware(handler).ServeHTTP(w, req)

		if w.Code != http.StatusOK {
			t.Errorf("request %d: expected status %d, got %d", i+1, http.StatusOK, w.Code)
		}
	}
}

func TestRateLimiter_BlocksOverLimit(t *testing.T) {
	rl := NewRateLimiter(2, time.Minute)

	// First 2 should pass
	for i := 0; i < 2; i++ {
		req := httptest.NewRequest(http.MethodGet, "/", nil)
		req.RemoteAddr = "127.0.0.1:1234"
		w := httptest.NewRecorder()

		handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusOK)
		})
		rl.Middleware(handler).ServeHTTP(w, req)

		if w.Code != http.StatusOK {
			t.Errorf("request %d: expected status %d, got %d", i+1, http.StatusOK, w.Code)
		}
	}

	// 3rd should be blocked
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.RemoteAddr = "127.0.0.1:1234"
	w := httptest.NewRecorder()

	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
	rl.Middleware(handler).ServeHTTP(w, req)

	if w.Code != http.StatusTooManyRequests {
		t.Errorf("expected status %d, got %d", http.StatusTooManyRequests, w.Code)
	}
}

func TestRateLimiter_DifferentIPsNotAffected(t *testing.T) {
	rl := NewRateLimiter(1, time.Minute)

	// IP 1 uses its limit
	req1 := httptest.NewRequest(http.MethodGet, "/", nil)
	req1.RemoteAddr = "192.168.1.1:1111"
	w1 := httptest.NewRecorder()
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
	rl.Middleware(handler).ServeHTTP(w1, req1)

	// IP 2 should still be allowed
	req2 := httptest.NewRequest(http.MethodGet, "/", nil)
	req2.RemoteAddr = "10.0.0.5:2222"
	w2 := httptest.NewRecorder()
	rl.Middleware(handler).ServeHTTP(w2, req2)

	if w2.Code != http.StatusOK {
		t.Errorf("expected status %d for different IP, got %d", http.StatusOK, w2.Code)
	}
}

func TestRateLimiter_XForwardedFor(t *testing.T) {
	rl := NewRateLimiter(1, time.Minute)
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	// Same X-Forwarded-For should share rate limit
	for i := 0; i < 2; i++ {
		req := httptest.NewRequest(http.MethodGet, "/", nil)
		req.RemoteAddr = "10.0.0.1:9999"
		req.Header.Set("X-Forwarded-For", "203.0.113.50")
		w := httptest.NewRecorder()
		rl.Middleware(handler).ServeHTTP(w, req)

		expected := http.StatusOK
		if i > 0 {
			expected = http.StatusTooManyRequests
		}
		if w.Code != expected {
			t.Errorf("request %d: expected %d, got %d", i+1, expected, w.Code)
		}
	}
}

func TestRateLimiter_JSONErrorResponse(t *testing.T) {
	rl := NewRateLimiter(1, time.Minute)
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	// Use limit
	req1 := httptest.NewRequest(http.MethodGet, "/", nil)
	req1.RemoteAddr = "127.0.0.1:1234"
	w1 := httptest.NewRecorder()
	rl.Middleware(handler).ServeHTTP(w1, req1)

	// Exceed limit
	req2 := httptest.NewRequest(http.MethodGet, "/", nil)
	req2.RemoteAddr = "127.0.0.1:1234"
	w2 := httptest.NewRecorder()
	rl.Middleware(handler).ServeHTTP(w2, req2)

	if w2.Header().Get("Content-Type") != "application/json" {
		t.Errorf("expected Content-Type application/json, got %s", w2.Header().Get("Content-Type"))
	}

	body := w2.Body.String()
	if !strings.Contains(body, "rate_limit_exceeded") {
		t.Errorf("expected JSON error body with rate_limit_exceeded, got: %s", body)
	}
	if w2.Header().Get("Retry-After") == "" {
		t.Error("expected Retry-After header")
	}
	if w2.Header().Get("X-RateLimit-Limit") != "1" {
		t.Errorf("expected X-RateLimit-Limit 1, got %s", w2.Header().Get("X-RateLimit-Limit"))
	}
	if w2.Header().Get("X-RateLimit-Remaining") != "0" {
		t.Errorf("expected X-RateLimit-Remaining 0, got %s", w2.Header().Get("X-RateLimit-Remaining"))
	}
}

func TestLogging_CapturesStatusCode(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusCreated)
	})

	req := httptest.NewRequest(http.MethodPost, "/test", nil)
	w := httptest.NewRecorder()

	Logging(handler).ServeHTTP(w, req)

	if w.Code != http.StatusCreated {
		t.Errorf("expected status %d, got %d", http.StatusCreated, w.Code)
	}
}

func TestLogging_DefaultStatusIs200(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Don't call WriteHeader, should default to 200
		w.Write([]byte("ok"))
	})

	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	w := httptest.NewRecorder()

	Logging(handler).ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected status %d, got %d", http.StatusOK, w.Code)
	}
}

func TestCORS_Headers(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	req := httptest.NewRequest(http.MethodGet, "/api/test", nil)
	w := httptest.NewRecorder()

	CORS(nil)(handler).ServeHTTP(w, req)

	if w.Header().Get("Access-Control-Allow-Origin") != "*" {
		t.Errorf("expected CORS origin header, got %s", w.Header().Get("Access-Control-Allow-Origin"))
	}
}

func TestCORS_OptionsRequest(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	req := httptest.NewRequest(http.MethodOptions, "/api/test", nil)
	w := httptest.NewRecorder()

	CORS(nil)(handler).ServeHTTP(w, req)

	if w.Code != http.StatusNoContent {
		t.Errorf("expected status %d for OPTIONS, got %d", http.StatusNoContent, w.Code)
	}
}
