package handlers

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"html/template"
	"io/fs"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/topup-store/internal/middleware"
	"github.com/topup-store/internal/services"
)

func dict(values ...interface{}) map[string]interface{} {
	if len(values)%2 != 0 {
		return nil
	}
	dict := make(map[string]interface{}, len(values)/2)
	for i := 0; i < len(values); i += 2 {
		key, ok := values[i].(string)
		if !ok {
			return nil
		}
		dict[key] = values[i+1]
	}
	return dict
}

func parseTemplates() (*template.Template, error) {
	var templates *template.Template
	funcMap := template.FuncMap{"dict": dict}

	err := filepath.WalkDir("web/templates", func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() || !strings.HasSuffix(path, ".html") {
			return nil
		}

		content, err := os.ReadFile(path)
		if err != nil {
			return err
		}

		name := d.Name()
		if templates == nil {
			templates = template.New("").Funcs(funcMap)
		}
		_, err = templates.New(name).Parse(string(content))
		return err
	})

	return templates, err
}

type PageHandler struct {
	topupSvc   services.TopupServiceInterface
	paymentSvc services.PaymentServiceInterface
	notifySvc  services.NotifyServiceInterface
	templates  *template.Template
	waNumber   string
	adminPass  string
	logger     *slog.Logger
}

func NewPageHandler(topupSvc services.TopupServiceInterface, paymentSvc services.PaymentServiceInterface, notifySvc services.NotifyServiceInterface, waNumber, adminPass string, logger *slog.Logger) *PageHandler {
	templates, err := parseTemplates()
	if err != nil {
		logger.Error("Failed to parse templates", slog.String("error", err.Error()))
		panic(err)
	}
	return &PageHandler{
		topupSvc:   topupSvc,
		paymentSvc: paymentSvc,
		notifySvc:  notifySvc,
		templates:  templates,
		waNumber:   waNumber,
		adminPass:  adminPass,
		logger:     logger,
	}
}

func (h *PageHandler) Home(w http.ResponseWriter, r *http.Request) {
	if err := h.templates.ExecuteTemplate(w, "index.html", map[string]any{
		"WhatsappNumber": h.waNumber,
	}); err != nil {
		h.logger.Error("template error (index.html)", slog.String("error", err.Error()))
		http.Error(w, "Internal server error", http.StatusInternalServerError)
	}
}

func (h *PageHandler) OrderForm(w http.ResponseWriter, r *http.Request) {
	if err := h.templates.ExecuteTemplate(w, "order.html", map[string]any{
		"WhatsappNumber": h.waNumber,
	}); err != nil {
		h.logger.Error("template error (order.html)", slog.String("error", err.Error()))
		http.Error(w, "Internal server error", http.StatusInternalServerError)
	}
}

func (h *PageHandler) Status(w http.ResponseWriter, r *http.Request) {
	if err := h.templates.ExecuteTemplate(w, "status.html", nil); err != nil {
		h.logger.Error("template error (status.html)", slog.String("error", err.Error()))
		http.Error(w, "Internal server error", http.StatusInternalServerError)
	}
}

func (h *PageHandler) Admin(w http.ResponseWriter, r *http.Request) {
	data := map[string]any{
		"CSRFToken": middleware.GetCSRFToken(r.Context()),
	}

	if r.Method == http.MethodPost {
		password := r.FormValue("password")
		passHash := sha256.Sum256([]byte(password))
		expectedHash := sha256.Sum256([]byte(h.adminPass))
		if hmac.Equal(passHash[:], expectedHash[:]) {
			timestamp := strconv.FormatInt(time.Now().Unix(), 10)
			mac := hmac.New(sha256.New, []byte(h.adminPass))
			mac.Write([]byte(timestamp))
			token := hex.EncodeToString(mac.Sum(nil))

			http.SetCookie(w, &http.Cookie{
				Name:     "admin_auth",
				Value:    timestamp + ":" + token,
				Path:     "/admin",
				HttpOnly: true,
				Secure:   true,
				SameSite: http.SameSiteStrictMode,
				MaxAge:   3600,
			})
			http.Redirect(w, r, "/admin", http.StatusSeeOther)
			return
		}
		data["LoginFailed"] = true
		if err := h.templates.ExecuteTemplate(w, "admin.html", data); err != nil {
			h.logger.Error("template error (admin.html)", slog.String("error", err.Error()))
			http.Error(w, "Internal server error", http.StatusInternalServerError)
		}
		return
	}

	cookie, err := r.Cookie("admin_auth")
	if err != nil {
		if err := h.templates.ExecuteTemplate(w, "admin.html", data); err != nil {
			h.logger.Error("template error (admin.html)", slog.String("error", err.Error()))
			http.Error(w, "Internal server error", http.StatusInternalServerError)
		}
		return
	}

	parts := strings.SplitN(cookie.Value, ":", 2)
	if len(parts) != 2 {
		if err := h.templates.ExecuteTemplate(w, "admin.html", data); err != nil {
			h.logger.Error("template error (admin.html)", slog.String("error", err.Error()))
			http.Error(w, "Internal server error", http.StatusInternalServerError)
		}
		return
	}

	timestamp := parts[0]
	token := parts[1]

	ts, err := strconv.ParseInt(timestamp, 10, 64)
	if err != nil || time.Since(time.Unix(ts, 0)) > time.Hour {
		if err := h.templates.ExecuteTemplate(w, "admin.html", data); err != nil {
			h.logger.Error("template error (admin.html)", slog.String("error", err.Error()))
			http.Error(w, "Internal server error", http.StatusInternalServerError)
		}
		return
	}

	mac := hmac.New(sha256.New, []byte(h.adminPass))
	mac.Write([]byte(timestamp))
	expectedToken := hex.EncodeToString(mac.Sum(nil))

	if !hmac.Equal([]byte(token), []byte(expectedToken)) {
		if err := h.templates.ExecuteTemplate(w, "admin.html", data); err != nil {
			h.logger.Error("template error (admin.html)", slog.String("error", err.Error()))
			http.Error(w, "Internal server error", http.StatusInternalServerError)
		}
		return
	}

	data["Authenticated"] = true
	orders, _, err := h.paymentSvc.ListOrders(r.Context(), 1, 100)
	if err != nil {
		h.logger.Error("admin: failed to list orders", slog.String("error", err.Error()))
	}
	data["Orders"] = orders
	if err := h.templates.ExecuteTemplate(w, "admin.html", data); err != nil {
		h.logger.Error("template error (admin.html)", slog.String("error", err.Error()))
		http.Error(w, "Internal server error", http.StatusInternalServerError)
	}
}
