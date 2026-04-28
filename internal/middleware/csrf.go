package middleware

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"net/http"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
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

var ErrCSRFGenerationFailed = errors.New("failed to generate CSRF token")

type CSRFStore struct {
	pool *pgxpool.Pool
}

func NewCSRFStore(pool *pgxpool.Pool) *CSRFStore {
	return &CSRFStore{pool: pool}
}

func (s *CSRFStore) Generate() (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", ErrCSRFGenerationFailed
	}
	token := hex.EncodeToString(b)
	expiresAt := time.Now().Add(2 * time.Hour)

	_, err := s.pool.Exec(context.Background(),
		`INSERT INTO csrf_tokens (token, expires_at) VALUES ($1, $2)`,
		token, expiresAt)
	if err != nil {
		return "", err
	}

	s.cleanup()
	return token, nil
}

func (s *CSRFStore) Validate(token string) bool {
	var expiresAt time.Time
	err := s.pool.QueryRow(context.Background(),
		`SELECT expires_at FROM csrf_tokens WHERE token = $1`,
		token).Scan(&expiresAt)
	if err != nil {
		return false
	}
	if time.Now().After(expiresAt) {
		s.pool.Exec(context.Background(), `DELETE FROM csrf_tokens WHERE token = $1`, token)
		return false
	}

	_, err = s.pool.Exec(context.Background(), `DELETE FROM csrf_tokens WHERE token = $1`, token)
	return err == nil
}

func (s *CSRFStore) cleanup() {
	s.pool.Exec(context.Background(), `DELETE FROM csrf_tokens WHERE expires_at < NOW()`)
}

func CSRFMiddleware(store *CSRFStore) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.Method == http.MethodGet || r.Method == http.MethodHead || r.Method == http.MethodOptions {
				token, err := store.Generate()
				if err != nil {
					apperrors.WriteError(w, http.StatusInternalServerError, apperrors.ErrInternal, GetRequestID(r.Context()))
					return
				}
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

			newToken, err := store.Generate()
			if err != nil {
				apperrors.WriteError(w, http.StatusInternalServerError, apperrors.ErrInternal, GetRequestID(r.Context()))
				return
			}
			w.Header().Set("X-CSRF-Token", newToken)
			next.ServeHTTP(w, r)
		})
	}
}
