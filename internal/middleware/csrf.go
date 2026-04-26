package middleware

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"net/http"
	"sync"
	"time"

	"github.com/topup-store/internal/apperrors"
)

type contextKey struct{}

var csrfTokenKey = contextKey{}

func GetCSRFToken(ctx context.Context) string {
	if v, ok := ctx.Value(csrfTokenKey).(string); ok {
		return v
	}
	return ""
}

type CSRFStore struct {
	mu     sync.Mutex
	tokens map[string]time.Time
}

func NewCSRFStore() *CSRFStore {
	return &CSRFStore{tokens: make(map[string]time.Time)}
}

func (s *CSRFStore) Generate() string {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return ""
	}
	token := hex.EncodeToString(b)
	s.mu.Lock()
	s.tokens[token] = time.Now().Add(2 * time.Hour)
	s.cleanup()
	s.mu.Unlock()
	return token
}

func (s *CSRFStore) Validate(token string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	exp, ok := s.tokens[token]
	if !ok || time.Now().After(exp) {
		return false
	}
	delete(s.tokens, token)
	return true
}

func (s *CSRFStore) cleanup() {
	now := time.Now()
	for t, exp := range s.tokens {
		if now.After(exp) {
			delete(s.tokens, t)
		}
	}
}

func CSRFMiddleware(store *CSRFStore) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.Method == http.MethodGet || r.Method == http.MethodHead || r.Method == http.MethodOptions {
				token := store.Generate()
				w.Header().Set("X-CSRF-Token", token)
				r = r.WithContext(context.WithValue(r.Context(), csrfTokenKey, token))
				next.ServeHTTP(w, r)
				return
			}

			token := r.Header.Get("X-CSRF-Token")
			if token == "" {
				token = r.FormValue("csrf_token")
			}

			if !store.Validate(token) {
				apperrors.WriteError(w, http.StatusForbidden, apperrors.FieldError("csrf", "invalid or expired CSRF token"), GetRequestID(r.Context()))
				return
			}

			newToken := store.Generate()
			w.Header().Set("X-CSRF-Token", newToken)
			next.ServeHTTP(w, r)
		})
	}
}
