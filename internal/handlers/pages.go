package handlers

import (
	"context"
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
	"github.com/topup-store/internal/models"
	"github.com/topup-store/internal/repositories"
	"github.com/topup-store/internal/services"
)

func dict(values ...any) map[string]any {
	if len(values)%2 != 0 {
		return nil
	}
	dict := make(map[string]any, len(values)/2)
	for i := 0; i < len(values); i += 2 {
		key, ok := values[i].(string)
		if !ok {
			return nil
		}
		dict[key] = values[i+1]
	}
	return dict
}

var wibLocation = time.FixedZone("WIB", 7*60*60)

func formatWIB(t time.Time) string {
	if t.IsZero() {
		return "-"
	}
	return t.In(wibLocation).Format("2006-01-02 15:04 WIB")
}

func parseTemplates() (*template.Template, error) {
	var templates *template.Template
	funcMap := template.FuncMap{
		"dict":      dict,
		"formatWIB": formatWIB,
	}

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
	orderRepo         repositories.OrderRepository
	topupSvc          services.TopupServiceInterface
	paymentSvc        services.PaymentServiceInterface
	notifySvc         services.NotifyServiceInterface
	templates         *template.Template
	waNumber          string
	adminPass         string
	adminPath         string
	midtransClientKey string
	midtransIsProd    bool
	cookieSecure      bool
	announcementText  string
	announcementLevel string
	logger            *slog.Logger
}

type adminOrderView struct {
	models.Order
	ProductName    string
	ProductGame    string
	CostPriceIDR   int
	MidtransFeeIDR int
	NetProfitIDR   int
}

func NewPageHandler(orderRepo repositories.OrderRepository, topupSvc services.TopupServiceInterface, paymentSvc services.PaymentServiceInterface, notifySvc services.NotifyServiceInterface, waNumber, adminPass, adminPath, midtransClientKey string, midtransIsProd, cookieSecure bool, announcementText, announcementLevel string, logger *slog.Logger) *PageHandler {
	templates, err := parseTemplates()
	if err != nil {
		logger.Error("Failed to parse templates", slog.String("error", err.Error()))
		panic(err)
	}
	return &PageHandler{
		orderRepo:         orderRepo,
		topupSvc:          topupSvc,
		paymentSvc:        paymentSvc,
		notifySvc:         notifySvc,
		templates:         templates,
		waNumber:          waNumber,
		adminPass:         adminPass,
		adminPath:         adminPath,
		midtransClientKey: midtransClientKey,
		midtransIsProd:    midtransIsProd,
		cookieSecure:      cookieSecure,
		announcementText:  announcementText,
		announcementLevel: announcementLevel,
		logger:            logger,
	}
}

func (h *PageHandler) navData(activePage string) map[string]any {
	return map[string]any{
		"ActivePage":        activePage,
		"AnnouncementText":  h.announcementText,
		"AnnouncementLevel": h.announcementLevel,
	}
}

func (h *PageHandler) Home(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 2*time.Second)
	defer cancel()

	successCount := 0
	if h.orderRepo != nil {
		count, err := h.orderRepo.CountSuccessOrders(ctx)
		if err != nil {
			h.logger.Warn("failed to count success orders", slog.String("error", err.Error()))
		} else {
			successCount = count
		}
	}

	if err := h.templates.ExecuteTemplate(w, "index.html", map[string]any{
		"WhatsappNumber": h.waNumber,
		"SuccessCount":   successCount,
		"Nav":            h.navData("home"),
	}); err != nil {
		h.logger.Error("template error (index.html)", slog.String("error", err.Error()))
		http.Error(w, "Internal server error", http.StatusInternalServerError)
	}
}

func (h *PageHandler) OrderForm(w http.ResponseWriter, r *http.Request) {
	if err := h.templates.ExecuteTemplate(w, "order.html", map[string]any{
		"WhatsappNumber":    h.waNumber,
		"CSRFToken":         middleware.GetCSRFToken(r.Context()),
		"MidtransClientKey": h.midtransClientKey,
		"MidtransIsProd":    h.midtransIsProd,
		"Nav":               h.navData("order"),
	}); err != nil {
		h.logger.Error("template error (order.html)", slog.String("error", err.Error()))
		http.Error(w, "Internal server error", http.StatusInternalServerError)
	}
}

func (h *PageHandler) Status(w http.ResponseWriter, r *http.Request) {
	if err := h.templates.ExecuteTemplate(w, "status.html", map[string]any{
		"CSRFToken": middleware.GetCSRFToken(r.Context()),
		"Nav":       h.navData("status"),
	}); err != nil {
		h.logger.Error("template error (status.html)", slog.String("error", err.Error()))
		http.Error(w, "Internal server error", http.StatusInternalServerError)
	}
}

func (h *PageHandler) Terms(w http.ResponseWriter, r *http.Request) {
	if err := h.templates.ExecuteTemplate(w, "terms.html", map[string]any{"Nav": h.navData("terms")}); err != nil {
		h.logger.Error("template error (terms.html)", slog.String("error", err.Error()))
		http.Error(w, "Internal server error", http.StatusInternalServerError)
	}
}

func (h *PageHandler) Refund(w http.ResponseWriter, r *http.Request) {
	if err := h.templates.ExecuteTemplate(w, "refund.html", map[string]any{"Nav": h.navData("refund")}); err != nil {
		h.logger.Error("template error (refund.html)", slog.String("error", err.Error()))
		http.Error(w, "Internal server error", http.StatusInternalServerError)
	}
}

func (h *PageHandler) Admin(w http.ResponseWriter, r *http.Request) {
	data := map[string]any{
		"CSRFToken": middleware.GetCSRFToken(r.Context()),
		"AdminPath": h.adminPath,
		"Nav":       h.navData("admin"),
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
				Path:     h.adminPath,
				HttpOnly: true,
				Secure:   h.cookieSecure,
				SameSite: http.SameSiteStrictMode,
				MaxAge:   3600,
			})
			http.Redirect(w, r, h.adminPath, http.StatusSeeOther)
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

	adminOrders := make([]adminOrderView, 0, len(orders))
	products := make(map[string]*models.Product)
	for _, order := range orders {
		product, ok := products[order.ProductID]
		if !ok {
			fetched, err := h.topupSvc.GetProduct(r.Context(), order.ProductID)
			if err != nil {
				h.logger.Warn("admin: failed to load product cost", slog.String("product_id", order.ProductID), slog.String("error", err.Error()))
			} else {
				product = fetched
			}
			products[order.ProductID] = product
		}
		productName := order.ProductID
		productGame := ""
		cost := 0
		if product != nil {
			productName = product.Name
			productGame = product.Game
			cost = product.CostPriceIDR
		}
		midtransFee := midtransFeeIDR(order.AmountIDR)
		adminOrders = append(adminOrders, adminOrderView{
			Order:          order,
			ProductName:    productName,
			ProductGame:    productGame,
			CostPriceIDR:   cost,
			MidtransFeeIDR: midtransFee,
			NetProfitIDR:   order.AmountIDR - cost - midtransFee,
		})
	}
	data["Orders"] = adminOrders
	if err := h.templates.ExecuteTemplate(w, "admin.html", data); err != nil {
		h.logger.Error("template error (admin.html)", slog.String("error", err.Error()))
		http.Error(w, "Internal server error", http.StatusInternalServerError)
	}
}

func midtransFeeIDR(amount int) int {
	if amount <= 0 {
		return 0
	}
	return (amount*2 + 99) / 100
}

func (h *PageHandler) NotFound(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(http.StatusNotFound)
	if err := h.templates.ExecuteTemplate(w, "404.html", nil); err != nil {
		h.logger.Error("template error (404.html)", slog.String("error", err.Error()))
		http.Error(w, "Not Found", http.StatusNotFound)
	}
}

func (h *PageHandler) ServerError(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(http.StatusInternalServerError)
	if err := h.templates.ExecuteTemplate(w, "500.html", nil); err != nil {
		h.logger.Error("template error (500.html)", slog.String("error", err.Error()))
		http.Error(w, "Internal server error", http.StatusInternalServerError)
	}
}
