package middleware

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/topup-store/internal/apperrors"
)

func AdminAuth(adminPass string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			cookie, err := r.Cookie("admin_auth")
			if err != nil {
				apperrors.WriteError(w, http.StatusUnauthorized, apperrors.ErrUnauthorized, GetRequestID(r.Context()))
				return
			}

			parts := strings.SplitN(cookie.Value, ":", 2)
			if len(parts) != 2 {
				apperrors.WriteError(w, http.StatusUnauthorized, apperrors.ErrUnauthorized, GetRequestID(r.Context()))
				return
			}

		timestamp := parts[0]
		token := parts[1]

		ts, err := strconv.ParseInt(timestamp, 10, 64)
		if err != nil || time.Since(time.Unix(ts, 0)) > time.Hour {
			apperrors.WriteError(w, http.StatusUnauthorized, apperrors.ErrUnauthorized, GetRequestID(r.Context()))
			return
		}

		mac := hmac.New(sha256.New, []byte(adminPass))
			mac.Write([]byte(timestamp))
			expected := hex.EncodeToString(mac.Sum(nil))

			if !hmac.Equal([]byte(token), []byte(expected)) {
				apperrors.WriteError(w, http.StatusUnauthorized, apperrors.ErrUnauthorized, GetRequestID(r.Context()))
				return
			}

			next.ServeHTTP(w, r)
		})
	}
}
