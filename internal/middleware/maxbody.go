package middleware

import (
	"net/http"

	"github.com/topup-store/internal/apperrors"
)

func MaxBodyMiddleware(limit int64) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.ContentLength > limit {
				apperrors.WriteError(w, http.StatusRequestEntityTooLarge, apperrors.FieldError("body", "request body too large"), GetRequestID(r.Context()))
				return
			}
			r.Body = http.MaxBytesReader(w, r.Body, limit)
			next.ServeHTTP(w, r)
		})
	}
}
