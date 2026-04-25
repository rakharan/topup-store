package middleware

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"net/http"
	"strings"

	"github.com/topup-store/internal/apperrors"
)

func BasicAuth(username, password string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			auth := r.Header.Get("Authorization")
			if auth == "" {
				w.Header().Set("WWW-Authenticate", `Basic realm="Restricted"`)
				http.Error(w, "Unauthorized", http.StatusUnauthorized)
				return
			}

			creds := strings.TrimPrefix(auth, "Basic ")
			decoded, err := base64.StdEncoding.DecodeString(creds)
			if err != nil {
				http.Error(w, "Unauthorized", http.StatusUnauthorized)
				return
			}

			parts := strings.SplitN(string(decoded), ":", 2)
			if len(parts) != 2 || parts[0] != username || parts[1] != password {
				http.Error(w, "Forbidden", http.StatusForbidden)
				return
			}

			next.ServeHTTP(w, r)
		})
	}
}

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
