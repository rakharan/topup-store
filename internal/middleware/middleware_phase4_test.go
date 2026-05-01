package middleware

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

func testPool(t *testing.T) *pgxpool.Pool {
	dsn := os.Getenv("DATABASE_URL")
	if dsn == "" {
		t.Skip("DATABASE_URL not set")
	}
	pool, err := pgxpool.New(context.Background(), dsn)
	if err != nil {
		t.Fatalf("failed to connect to DB: %v", err)
	}
	t.Cleanup(func() { pool.Close() })
	_, _ = pool.Exec(context.Background(), `DELETE FROM csrf_tokens`)
	return pool
}

func TestCSRFStore_GenerateAndValidate(t *testing.T) {
	pool := testPool(t)
	store := NewCSRFStore(pool)

	token, err := store.Generate()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if token == "" {
		t.Error("expected non-empty token")
	}

	if !store.Validate(token) {
		t.Error("expected valid token")
	}

	if store.Validate(token) {
		t.Error("expected token to be consumed after first validation")
	}
}

func TestCSRFStore_InvalidToken(t *testing.T) {
	pool := testPool(t)
	store := NewCSRFStore(pool)

	if store.Validate("nonexistent") {
		t.Error("expected invalid token to fail")
	}
}

func TestCSRFMiddleware_GETReturnsToken(t *testing.T) {
	pool := testPool(t)
	store := NewCSRFStore(pool)
	mw := CSRFMiddleware(store)

	handler := mw(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	w := httptest.NewRecorder()

	handler.ServeHTTP(w, req)

	token := w.Header().Get("X-CSRF-Token")
	if token == "" {
		t.Error("expected CSRF token in response header")
	}
}

func TestCSRFMiddleware_POSTValidToken(t *testing.T) {
	pool := testPool(t)
	store := NewCSRFStore(pool)
	mw := CSRFMiddleware(store)

	token, err := store.Generate()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	handler := mw(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest(http.MethodPost, "/", nil)
	req.Header.Set("X-CSRF-Token", token)
	w := httptest.NewRecorder()

	handler.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", w.Code)
	}
}

func TestCSRFMiddleware_POSTMissingToken(t *testing.T) {
	pool := testPool(t)
	store := NewCSRFStore(pool)
	mw := CSRFMiddleware(store)

	handler := mw(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))

	req := httptest.NewRequest(http.MethodPost, "/", nil)
	w := httptest.NewRecorder()

	handler.ServeHTTP(w, req)

	if w.Code != http.StatusForbidden {
		t.Errorf("expected 403, got %d", w.Code)
	}
}

func TestCSRFMiddleware_POSTExpiredToken(t *testing.T) {
	pool := testPool(t)
	store := NewCSRFStore(pool)
	token, err := store.Generate()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	_, _ = pool.Exec(context.Background(),
		`UPDATE csrf_tokens SET expires_at = $1 WHERE token = $2`,
		time.Now().Add(-1*time.Hour), token)

	mw := CSRFMiddleware(store)
	handler := mw(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))

	req := httptest.NewRequest(http.MethodPost, "/", nil)
	req.Header.Set("X-CSRF-Token", token)
	w := httptest.NewRecorder()

	handler.ServeHTTP(w, req)

	if w.Code != http.StatusForbidden {
		t.Errorf("expected 403, got %d", w.Code)
	}
}

func TestCSRFMiddleware_FormToken(t *testing.T) {
	pool := testPool(t)
	store := NewCSRFStore(pool)
	token, err := store.Generate()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	mw := CSRFMiddleware(store)
	handler := mw(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest(http.MethodPost, "/", strings.NewReader("csrf_token="+token))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	w := httptest.NewRecorder()

	handler.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", w.Code)
	}
}

func TestMaxBodyMiddleware_UnderLimit(t *testing.T) {
	mw := MaxBodyMiddleware(1024)

	handler := mw(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest(http.MethodPost, "/", strings.NewReader("small"))
	w := httptest.NewRecorder()

	handler.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", w.Code)
	}
}

func TestMaxBodyMiddleware_OverContentLength(t *testing.T) {
	mw := MaxBodyMiddleware(10)

	handler := mw(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest(http.MethodPost, "/", strings.NewReader("this is a longer body"))
	req.ContentLength = 23
	w := httptest.NewRecorder()

	handler.ServeHTTP(w, req)

	if w.Code != http.StatusRequestEntityTooLarge {
		t.Errorf("expected 413, got %d", w.Code)
	}
}

func TestTimeoutMiddleware_CompletesInTime(t *testing.T) {
	mw := Timeout(5 * time.Second)

	handler := mw(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	w := httptest.NewRecorder()

	handler.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", w.Code)
	}
}

func TestTimeoutMiddleware_TimesOut(t *testing.T) {
	mw := Timeout(10 * time.Millisecond)

	handler := mw(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(100 * time.Millisecond)
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	w := httptest.NewRecorder()

	handler.ServeHTTP(w, req)

	if w.Code != http.StatusServiceUnavailable {
		t.Errorf("expected 503, got %d", w.Code)
	}
}

func TestSecurityHeaders(t *testing.T) {
	handler := SecurityHeaders(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	w := httptest.NewRecorder()

	handler.ServeHTTP(w, req)

	headers := map[string]string{
		"X-Content-Type-Options": "nosniff",
		"X-Frame-Options":        "SAMEORIGIN",
		"Referrer-Policy":        "strict-origin-when-cross-origin",
	}

	for key, expected := range headers {
		if got := w.Header().Get(key); got != expected {
			t.Errorf("header %q: expected %q, got %q", key, expected, got)
		}
	}
}
