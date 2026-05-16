package middleware

import (
	"encoding/json"
	"html"
	"net/http"
	"strings"
)

func MaintenanceMode(enabled bool, adminPath, message string) func(http.Handler) http.Handler {
	if message == "" {
		message = "Layanan sedang maintenance. Silakan coba lagi sebentar lagi."
	}
	return func(next http.Handler) http.Handler {
		if !enabled {
			return next
		}
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if maintenanceAllowedPath(r.URL.Path, adminPath) {
				next.ServeHTTP(w, r)
				return
			}

			w.Header().Set("Retry-After", "300")
			if strings.HasPrefix(r.URL.Path, "/api/") {
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusServiceUnavailable)
				_ = json.NewEncoder(w).Encode(map[string]any{
					"success": false,
					"error": map[string]string{
						"code":    "MAINTENANCE",
						"message": message,
					},
					"request_id": GetRequestID(r.Context()),
				})
				return
			}
			w.Header().Set("Content-Type", "text/html; charset=utf-8")
			w.WriteHeader(http.StatusServiceUnavailable)
			_, _ = w.Write([]byte(`<!doctype html><html lang="id"><head><meta charset="utf-8"><meta name="viewport" content="width=device-width,initial-scale=1"><title>Maintenance</title><style>body{margin:0;min-height:100vh;display:grid;place-items:center;background:#0b0b0f;color:#f8fafc;font-family:system-ui,sans-serif}.box{max-width:520px;padding:32px;text-align:center}h1{color:#f97316}</style></head><body><main class="box"><h1>Maintenance</h1><p>` + html.EscapeString(message) + `</p></main></body></html>`))
		})
	}
}

func maintenanceAllowedPath(path, adminPath string) bool {
	if path == "/health" || path == "/ready" || path == "/metrics" {
		return true
	}
	if strings.HasPrefix(path, "/static/") || strings.HasPrefix(path, "/webhook/") {
		return true
	}
	if adminPath != "" && strings.HasPrefix(path, adminPath) {
		return true
	}
	return false
}
