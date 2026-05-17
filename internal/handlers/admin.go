package handlers

import (
	"context"
	"encoding/csv"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/topup-store/internal/apperrors"
	"github.com/topup-store/internal/cache"
	"github.com/topup-store/internal/constants"
	"github.com/topup-store/internal/middleware"
	"github.com/topup-store/internal/models"
	"github.com/topup-store/internal/repositories"
	"github.com/topup-store/internal/services"
)

type AdminHandler struct {
	paymentSvc          services.PaymentServiceInterface
	topupSvc            services.TopupServiceInterface
	notifySvc           services.NotifyServiceInterface
	productRepo         repositories.ProductRepository
	webhookRepo         repositories.WebhookRepository
	orderRepo           repositories.OrderRepository
	blockedIdentityRepo repositories.BlockedIdentityRepository
	referralCodeRepo    repositories.ReferralCodeRepository
	auditRepo           repositories.AuditLogRepository
	cache               *cache.Cache
	retrySvc            *services.WebhookRetryService
	adminPass           string
	logger              *slog.Logger
}

func NewAdminHandler(paymentSvc services.PaymentServiceInterface, topupSvc services.TopupServiceInterface, notifySvc services.NotifyServiceInterface, productRepo repositories.ProductRepository, webhookRepo repositories.WebhookRepository, orderRepo repositories.OrderRepository, blockedIdentityRepo repositories.BlockedIdentityRepository, referralCodeRepo repositories.ReferralCodeRepository, auditRepo repositories.AuditLogRepository, cache *cache.Cache, retrySvc *services.WebhookRetryService, adminPass string, logger *slog.Logger) *AdminHandler {
	return &AdminHandler{
		paymentSvc:          paymentSvc,
		topupSvc:            topupSvc,
		notifySvc:           notifySvc,
		productRepo:         productRepo,
		webhookRepo:         webhookRepo,
		orderRepo:           orderRepo,
		blockedIdentityRepo: blockedIdentityRepo,
		referralCodeRepo:    referralCodeRepo,
		auditRepo:           auditRepo,
		cache:               cache,
		retrySvc:            retrySvc,
		adminPass:           adminPass,
		logger:              logger,
	}
}

func (h *AdminHandler) logAudit(ctx context.Context, r *http.Request, action, entityType, entityID, oldValue, newValue string) {
	if h.auditRepo == nil {
		return
	}
	ip := r.Header.Get("X-Forwarded-For")
	if ip == "" {
		ip = r.RemoteAddr
	}
	if err := h.auditRepo.Log(ctx, &repositories.AuditLogEntry{
		Action:     action,
		EntityType: entityType,
		EntityID:   entityID,
		OldValue:   oldValue,
		NewValue:   newValue,
		AdminIP:    ip,
		AdminUA:    r.UserAgent(),
	}); err != nil {
		h.logger.Error("audit log failed", slog.String("error", err.Error()))
	}
}

func (h *AdminHandler) ProcessOrder(w http.ResponseWriter, r *http.Request) {
	var req struct {
		OrderID string `json:"order_id"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		io.Copy(io.Discard, r.Body)
		r.Body.Close()
		apperrors.WriteError(w, http.StatusBadRequest, apperrors.ErrInvalidInput, middleware.GetRequestID(r.Context()))
		return
	}

	if req.OrderID == "" {
		apperrors.WriteError(w, http.StatusBadRequest, apperrors.FieldError("order_id", "order_id is required"), middleware.GetRequestID(r.Context()))
		return
	}

	order, err := h.paymentSvc.GetOrder(r.Context(), req.OrderID)
	if err != nil {
		apperrors.WriteError(w, http.StatusNotFound, apperrors.ErrNotFound, middleware.GetRequestID(r.Context()))
		return
	}

	if order.Status != constants.StatusPending {
		apperrors.WriteError(w, http.StatusBadRequest, apperrors.FieldError("status", "order must be in pending status, current: "+order.Status), middleware.GetRequestID(r.Context()))
		return
	}

	if err := h.paymentSvc.UpdateOrderStatus(r.Context(), order.ID, constants.StatusPaid); err != nil {
		apperrors.WriteError(w, http.StatusInternalServerError, apperrors.ErrInternal, middleware.GetRequestID(r.Context()))
		return
	}

	h.logAudit(r.Context(), r, "process_order", "order", order.ID, order.Status, constants.StatusPaid)

	orderCopy := *order
	go func() {
		defer func() {
			if r := recover(); r != nil {
				h.logger.Error("admin process order panicked", slog.String("order_id", orderCopy.ID), slog.Any("panic", r))
			}
		}()
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
		defer cancel()
		if err := h.topupSvc.ProcessOrder(ctx, orderCopy.ID); err != nil {
			h.logger.Error("admin process order: topup failed", slog.String("order_id", orderCopy.ID), slog.String("error", err.Error()))
			return
		}
		h.logger.Info("admin process order: completed", slog.String("order_id", orderCopy.ID))
	}()

	apperrors.WriteSuccess(w, http.StatusOK, map[string]string{
		"status":  "ok",
		"message": "order marked as paid and topup processing started",
	}, middleware.GetRequestID(r.Context()))
}

func (h *AdminHandler) RetryOrder(w http.ResponseWriter, r *http.Request) {
	var req struct {
		OrderID string `json:"order_id"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		io.Copy(io.Discard, r.Body)
		r.Body.Close()
		apperrors.WriteError(w, http.StatusBadRequest, apperrors.ErrInvalidInput, middleware.GetRequestID(r.Context()))
		return
	}

	if req.OrderID == "" {
		apperrors.WriteError(w, http.StatusBadRequest, apperrors.FieldError("order_id", "order_id is required"), middleware.GetRequestID(r.Context()))
		return
	}

	order, err := h.paymentSvc.GetOrder(r.Context(), req.OrderID)
	if err != nil {
		apperrors.WriteError(w, http.StatusNotFound, apperrors.ErrNotFound, middleware.GetRequestID(r.Context()))
		return
	}

	if order.Status != constants.StatusPaid && order.Status != constants.StatusProcessing && order.Status != constants.StatusFailed {
		apperrors.WriteError(w, http.StatusBadRequest, apperrors.FieldError("status", "order must be in paid, processing, or failed status, current: "+order.Status), middleware.GetRequestID(r.Context()))
		return
	}

	if order.Status == constants.StatusProcessing || order.Status == constants.StatusFailed {
		oldStatus := order.Status
		updated, err := h.paymentSvc.UpdateOrderStatusIf(r.Context(), order.ID, constants.StatusPaid, oldStatus)
		if err != nil {
			apperrors.WriteError(w, http.StatusInternalServerError, apperrors.ErrInternal, middleware.GetRequestID(r.Context()))
			return
		}
		if !updated {
			apperrors.WriteError(w, http.StatusBadRequest, apperrors.FieldError("status", "order status changed, reload and try again"), middleware.GetRequestID(r.Context()))
			return
		}
		h.paymentSvc.RecordStatusChange(r.Context(), order.ID, oldStatus, constants.StatusPaid, "admin retry reset")
		order.Status = constants.StatusPaid
	}

	h.logAudit(r.Context(), r, "retry_order", "order", order.ID, order.Status, constants.StatusProcessing)

	orderCopy := *order
	go func() {
		defer func() {
			if r := recover(); r != nil {
				h.logger.Error("admin retry order panicked", slog.String("order_id", orderCopy.ID), slog.Any("panic", r))
			}
		}()
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
		defer cancel()
		if err := h.topupSvc.ProcessOrder(ctx, orderCopy.ID); err != nil {
			h.logger.Error("admin retry order: topup failed", slog.String("order_id", orderCopy.ID), slog.String("error", err.Error()))
			return
		}
		h.logger.Info("admin retry order: completed", slog.String("order_id", orderCopy.ID))
	}()

	apperrors.WriteSuccess(w, http.StatusOK, map[string]string{
		"status":  "ok",
		"message": "order retry started",
	}, middleware.GetRequestID(r.Context()))
}

func (h *AdminHandler) ListProducts(w http.ResponseWriter, r *http.Request) {
	products, err := h.productRepo.ListAll(r.Context())
	if err != nil {
		apperrors.WriteError(w, http.StatusInternalServerError, apperrors.ErrInternal, middleware.GetRequestID(r.Context()))
		return
	}
	apperrors.WriteSuccess(w, http.StatusOK, products, middleware.GetRequestID(r.Context()))
}

func (h *AdminHandler) CreateProduct(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Game             string `json:"game"`
		Name             string `json:"name"`
		Description      string `json:"description"`
		PriceIDR         int    `json:"price_idr"`
		CostPriceIDR     int    `json:"cost_price_idr"`
		ItemQty          int    `json:"item_qty"`
		ProductType      string `json:"product_type"`
		SKU              string `json:"sku"`
		CustomerNoFormat string `json:"customer_no_format"`
		CompetitorPrice  int    `json:"competitor_price_idr"`
		BenchmarkNote    string `json:"benchmark_note"`
		IsActive         bool   `json:"is_active"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		io.Copy(io.Discard, r.Body)
		r.Body.Close()
		apperrors.WriteError(w, http.StatusBadRequest, apperrors.ErrInvalidInput, middleware.GetRequestID(r.Context()))
		return
	}

	if !constants.ValidGames[req.Game] {
		apperrors.WriteError(w, http.StatusBadRequest, apperrors.FieldError("game", "unsupported game"), middleware.GetRequestID(r.Context()))
		return
	}
	if req.Name == "" || req.SKU == "" || req.PriceIDR <= 0 {
		apperrors.WriteError(w, http.StatusBadRequest, apperrors.FieldError("input", "name, sku, and price_idr are required"), middleware.GetRequestID(r.Context()))
		return
	}

	if req.ProductType == "" {
		req.ProductType = "diamond"
	}
	if !validProductType(req.ProductType) {
		apperrors.WriteError(w, http.StatusBadRequest, apperrors.FieldError("product_type", "must be diamond, subscription, other, or validation"), middleware.GetRequestID(r.Context()))
		return
	}
	if !validCustomerNoFormat(req.CustomerNoFormat) {
		apperrors.WriteError(w, http.StatusBadRequest, apperrors.FieldError("customer_no_format", "must be uid, uid_server_pipe, uid_server_concat, or uid_server_space"), middleware.GetRequestID(r.Context()))
		return
	}

	exists, err := h.productRepo.ExistsBySKU(r.Context(), req.SKU, "")
	if err != nil {
		h.logger.Error("check SKU existence", slog.String("sku", req.SKU), slog.String("error", err.Error()))
		apperrors.WriteError(w, http.StatusInternalServerError, apperrors.ErrInternal, middleware.GetRequestID(r.Context()))
		return
	}
	if exists {
		apperrors.WriteError(w, http.StatusConflict, apperrors.FieldError("sku", "SKU already exists"), middleware.GetRequestID(r.Context()))
		return
	}

	product := &models.Product{
		ID:               uuid.New().String(),
		Game:             req.Game,
		Name:             req.Name,
		Description:      req.Description,
		PriceIDR:         req.PriceIDR,
		CostPriceIDR:     req.CostPriceIDR,
		ItemQty:          req.ItemQty,
		ProductType:      req.ProductType,
		SKU:              req.SKU,
		CustomerNoFormat: req.CustomerNoFormat,
		CompetitorPrice:  req.CompetitorPrice,
		BenchmarkNote:    req.BenchmarkNote,
		IsActive:         req.IsActive,
		Stock:            -1,
	}

	if err := h.productRepo.Create(r.Context(), product); err != nil {
		h.logger.Error("create product", slog.String("error", err.Error()))
		apperrors.WriteError(w, http.StatusInternalServerError, apperrors.ErrInternal, middleware.GetRequestID(r.Context()))
		return
	}

	h.logAudit(r.Context(), r, "create_product", "product", product.ID, "", product.SKU)
	h.cache.DeleteByPrefix(r.Context(), "product")

	apperrors.WriteSuccess(w, http.StatusCreated, product, middleware.GetRequestID(r.Context()))
}

func (h *AdminHandler) UpdateProduct(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	existing, err := h.productRepo.GetByID(r.Context(), id)
	if err != nil {
		apperrors.WriteError(w, http.StatusNotFound, apperrors.ErrNotFound, middleware.GetRequestID(r.Context()))
		return
	}

	var req struct {
		Game             string `json:"game"`
		Name             string `json:"name"`
		Description      string `json:"description"`
		PriceIDR         int    `json:"price_idr"`
		CostPriceIDR     int    `json:"cost_price_idr"`
		ItemQty          int    `json:"item_qty"`
		ProductType      string `json:"product_type"`
		SKU              string `json:"sku"`
		CustomerNoFormat string `json:"customer_no_format"`
		CompetitorPrice  int    `json:"competitor_price_idr"`
		BenchmarkNote    string `json:"benchmark_note"`
		IsActive         bool   `json:"is_active"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		io.Copy(io.Discard, r.Body)
		r.Body.Close()
		apperrors.WriteError(w, http.StatusBadRequest, apperrors.ErrInvalidInput, middleware.GetRequestID(r.Context()))
		return
	}

	if !constants.ValidGames[req.Game] {
		apperrors.WriteError(w, http.StatusBadRequest, apperrors.FieldError("game", "unsupported game"), middleware.GetRequestID(r.Context()))
		return
	}
	if req.Name == "" || req.SKU == "" || req.PriceIDR <= 0 {
		apperrors.WriteError(w, http.StatusBadRequest, apperrors.FieldError("input", "name, sku, and price_idr are required"), middleware.GetRequestID(r.Context()))
		return
	}

	if req.ProductType == "" {
		req.ProductType = "diamond"
	}
	if !validProductType(req.ProductType) {
		apperrors.WriteError(w, http.StatusBadRequest, apperrors.FieldError("product_type", "must be diamond, subscription, other, or validation"), middleware.GetRequestID(r.Context()))
		return
	}
	if !validCustomerNoFormat(req.CustomerNoFormat) {
		apperrors.WriteError(w, http.StatusBadRequest, apperrors.FieldError("customer_no_format", "must be uid, uid_server_pipe, uid_server_concat, or uid_server_space"), middleware.GetRequestID(r.Context()))
		return
	}

	exists2, err := h.productRepo.ExistsBySKU(r.Context(), req.SKU, id)
	if err != nil {
		h.logger.Error("check SKU existence", slog.String("sku", req.SKU), slog.String("error", err.Error()))
		apperrors.WriteError(w, http.StatusInternalServerError, apperrors.ErrInternal, middleware.GetRequestID(r.Context()))
		return
	}
	if exists2 {
		apperrors.WriteError(w, http.StatusConflict, apperrors.FieldError("sku", "SKU already exists"), middleware.GetRequestID(r.Context()))
		return
	}

	product := &models.Product{
		ID:               existing.ID,
		Game:             req.Game,
		Name:             req.Name,
		Description:      req.Description,
		PriceIDR:         req.PriceIDR,
		CostPriceIDR:     req.CostPriceIDR,
		ItemQty:          req.ItemQty,
		ProductType:      req.ProductType,
		SKU:              req.SKU,
		CustomerNoFormat: req.CustomerNoFormat,
		CompetitorPrice:  req.CompetitorPrice,
		BenchmarkNote:    req.BenchmarkNote,
		IsActive:         req.IsActive,
		Stock:            existing.Stock,
	}

	if err := h.productRepo.Update(r.Context(), product); err != nil {
		h.logger.Error("update product", slog.String("error", err.Error()))
		apperrors.WriteError(w, http.StatusInternalServerError, apperrors.ErrInternal, middleware.GetRequestID(r.Context()))
		return
	}

	h.logAudit(r.Context(), r, "update_product", "product", product.ID, existing.SKU, product.SKU)
	h.cache.DeleteByPrefix(r.Context(), "product")

	apperrors.WriteSuccess(w, http.StatusOK, product, middleware.GetRequestID(r.Context()))
}

func (h *AdminHandler) DeleteProduct(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	product, _ := h.productRepo.GetByID(r.Context(), id)
	if err := h.productRepo.Delete(r.Context(), id); err != nil {
		apperrors.WriteError(w, http.StatusNotFound, apperrors.ErrNotFound, middleware.GetRequestID(r.Context()))
		return
	}
	oldValue := ""
	if product != nil {
		oldValue = product.SKU
	}
	h.logAudit(r.Context(), r, "delete_product", "product", id, oldValue, "")
	h.cache.DeleteByPrefix(r.Context(), "product")
	apperrors.WriteSuccess(w, http.StatusOK, map[string]string{"status": "ok", "message": "product deleted"}, middleware.GetRequestID(r.Context()))
}

func (h *AdminHandler) SyncPrices(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Prices []struct {
			SKU   string `json:"buyer_sku_code"`
			Price int    `json:"price"`
		} `json:"prices"`
		MarginType  string `json:"margin_type"`
		MarginValue int    `json:"margin_value"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		io.Copy(io.Discard, r.Body)
		r.Body.Close()
		apperrors.WriteError(w, http.StatusBadRequest, apperrors.ErrInvalidInput, middleware.GetRequestID(r.Context()))
		return
	}

	if req.MarginType == "" {
		req.MarginType = "tiered"
	}

	type result struct {
		SKU    string `json:"sku"`
		Cost   int    `json:"cost"`
		Price  int    `json:"price"`
		Margin int    `json:"margin"`
		Tier   string `json:"tier"`
	}

	var results []result
	updated := 0
	skipped := 0

	for _, p := range req.Prices {
		if p.Price <= 0 || p.SKU == "" {
			skipped++
			continue
		}

		costPrice := p.Price
		sellingPrice, tier := services.CalcTieredPrice(costPrice, req.MarginType, req.MarginValue)

		if err := h.productRepo.SyncPrice(r.Context(), p.SKU, costPrice, sellingPrice); err != nil {
			h.logger.Warn("sync prices: failed to update", slog.String("sku", p.SKU), slog.String("error", err.Error()))
			skipped++
			continue
		}

		updated++
		results = append(results, result{
			SKU:    p.SKU,
			Cost:   costPrice,
			Price:  sellingPrice,
			Margin: sellingPrice - costPrice,
			Tier:   tier,
		})
	}

	h.logAudit(r.Context(), r, "sync_prices", "product", "bulk", "", fmt.Sprintf("updated=%d skipped=%d total=%d", updated, skipped, len(req.Prices)))
	apperrors.WriteSuccess(w, http.StatusOK, map[string]any{
		"updated":  updated,
		"skipped":  skipped,
		"total":    len(req.Prices),
		"products": results,
	}, middleware.GetRequestID(r.Context()))
}

func (h *AdminHandler) SyncPricesFromDigiflazz(w http.ResponseWriter, r *http.Request) {
	var req struct {
		MarginType        string `json:"margin_type"`
		MarginValue       int    `json:"margin_value"`
		RecalculatePrices bool   `json:"recalculate_prices"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		io.Copy(io.Discard, r.Body)
		r.Body.Close()
		apperrors.WriteError(w, http.StatusBadRequest, apperrors.ErrInvalidInput, middleware.GetRequestID(r.Context()))
		return
	}

	results, updated, created, skipped, err := h.topupSvc.SyncPricesWithAutoCreate(r.Context(), req.MarginType, req.MarginValue, req.RecalculatePrices)
	if err != nil {
		h.logger.Error("sync prices from digiflazz: failed", slog.String("error", err.Error()))
		apperrors.WriteError(w, http.StatusInternalServerError, apperrors.FieldError("digiflazz", err.Error()), middleware.GetRequestID(r.Context()))
		return
	}

	h.cache.DeleteByPrefix(r.Context(), "product")
	h.logAudit(r.Context(), r, "sync_digiflazz", "product", "bulk", "", fmt.Sprintf("updated=%d created=%d skipped=%d", updated, created, skipped))

	apperrors.WriteSuccess(w, http.StatusOK, map[string]any{
		"updated":  updated,
		"created":  created,
		"skipped":  skipped,
		"total":    len(results),
		"products": results,
	}, middleware.GetRequestID(r.Context()))
}

func (h *AdminHandler) GetBalance(w http.ResponseWriter, r *http.Request) {
	balance := h.topupSvc.GetBalance()
	apperrors.WriteSuccess(w, http.StatusOK, map[string]any{
		"balance": balance,
	}, middleware.GetRequestID(r.Context()))
}

func (h *AdminHandler) ExportOrdersCSV(w http.ResponseWriter, r *http.Request) {
	rows, err := h.orderRepo.ListAllForExport(r.Context())
	if err != nil {
		h.logger.Error("export orders: failed to fetch", slog.String("error", err.Error()))
		apperrors.WriteError(w, http.StatusInternalServerError, apperrors.ErrInternal, middleware.GetRequestID(r.Context()))
		return
	}

	w.Header().Set("Content-Type", "text/csv")
	w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=orders_%s.csv", time.Now().Format("2006-01-02")))

	cw := csv.NewWriter(w)
	defer cw.Flush()

	cw.Write([]string{"Order Number", "Order ID", "Game", "Product", "UID", "Server", "Phone", "Amount (IDR)", "Status", "Serial No", "Channel", "Created At"})
	for _, row := range rows {
		server := ""
		if row.GameServer != "" {
			server = row.GameServer
		}
		cw.Write([]string{
			row.OrderNumber,
			row.OrderID,
			row.Game,
			row.ProductName,
			row.GameUID,
			server,
			row.Phone,
			strconv.Itoa(row.AmountIDR),
			row.Status,
			row.SerialNumber,
			row.Channel,
			row.CreatedAt.Format("2006-01-02 15:04:05"),
		})
	}
}

func (h *AdminHandler) GetAnalytics(w http.ResponseWriter, r *http.Request) {
	days := 30
	if d := r.URL.Query().Get("days"); d != "" {
		if n, err := strconv.Atoi(d); err == nil && n > 0 && n <= 365 {
			days = n
		}
	}

	cacheKey := fmt.Sprintf("analytics:%d", days)
	var cached map[string]any
	if h.cache.Get(r.Context(), cacheKey, &cached) {
		apperrors.WriteSuccess(w, http.StatusOK, cached, middleware.GetRequestID(r.Context()))
		return
	}

	endDate := time.Now().Truncate(24 * time.Hour).Add(24 * time.Hour)
	startDate := endDate.AddDate(0, 0, -days)

	dailyRevenue, err := h.orderRepo.GetDailyRevenue(r.Context(), startDate, endDate)
	if err != nil {
		h.logger.Error("analytics: daily revenue failed", slog.String("error", err.Error()))
		apperrors.WriteError(w, http.StatusInternalServerError, apperrors.ErrInternal, middleware.GetRequestID(r.Context()))
		return
	}

	topGames, err := h.orderRepo.GetTopGamesByRevenue(r.Context(), startDate, endDate)
	if err != nil {
		h.logger.Error("analytics: top games failed", slog.String("error", err.Error()))
		apperrors.WriteError(w, http.StatusInternalServerError, apperrors.ErrInternal, middleware.GetRequestID(r.Context()))
		return
	}

	overall, err := h.orderRepo.GetOverallStats(r.Context(), startDate, endDate)
	if err != nil {
		h.logger.Error("analytics: overall stats failed", slog.String("error", err.Error()))
		apperrors.WriteError(w, http.StatusInternalServerError, apperrors.ErrInternal, middleware.GetRequestID(r.Context()))
		return
	}

	result := map[string]any{
		"daily_revenue": dailyRevenue,
		"top_games":     topGames,
		"overall":       overall,
		"period":        map[string]string{"start": startDate.Format("2006-01-02"), "end": endDate.Format("2006-01-02")},
	}
	h.cache.Set(r.Context(), cacheKey, result, 5*time.Minute)
	apperrors.WriteSuccess(w, http.StatusOK, result, middleware.GetRequestID(r.Context()))
}

func (h *AdminHandler) GetRetryQueueStats(w http.ResponseWriter, r *http.Request) {
	if h.retrySvc == nil {
		apperrors.WriteSuccess(w, http.StatusOK, map[string]any{"enabled": false}, middleware.GetRequestID(r.Context()))
		return
	}
	stats, err := h.retrySvc.GetStats(r.Context())
	if err != nil {
		h.logger.Error("retry queue stats: failed", slog.String("error", err.Error()))
		apperrors.WriteError(w, http.StatusInternalServerError, apperrors.ErrInternal, middleware.GetRequestID(r.Context()))
		return
	}
	result := map[string]any{"enabled": true}
	for k, v := range stats {
		result[k] = v
	}
	apperrors.WriteSuccess(w, http.StatusOK, result, middleware.GetRequestID(r.Context()))
}

func (h *AdminHandler) ListDeadItems(w http.ResponseWriter, r *http.Request) {
	if h.retrySvc == nil {
		apperrors.WriteSuccess(w, http.StatusOK, []any{}, middleware.GetRequestID(r.Context()))
		return
	}
	items, err := h.retrySvc.ListDeadItems(r.Context(), 50)
	if err != nil {
		h.logger.Error("list dead items: failed", slog.String("error", err.Error()))
		apperrors.WriteError(w, http.StatusInternalServerError, apperrors.ErrInternal, middleware.GetRequestID(r.Context()))
		return
	}
	apperrors.WriteSuccess(w, http.StatusOK, items, middleware.GetRequestID(r.Context()))
}

func (h *AdminHandler) RetryDeadItem(w http.ResponseWriter, r *http.Request) {
	if h.retrySvc == nil {
		apperrors.WriteError(w, http.StatusBadRequest, apperrors.ErrInternal, middleware.GetRequestID(r.Context()))
		return
	}
	id := r.PathValue("id")
	if err := h.retrySvc.RetryDeadItem(r.Context(), id); err != nil {
		h.logger.Error("retry dead item: failed", slog.String("id", id), slog.String("error", err.Error()))
		apperrors.WriteError(w, http.StatusInternalServerError, apperrors.ErrInternal, middleware.GetRequestID(r.Context()))
		return
	}
	apperrors.WriteSuccess(w, http.StatusOK, map[string]string{"status": "ok"}, middleware.GetRequestID(r.Context()))
}

func (h *AdminHandler) ListWebhookLogs(w http.ResponseWriter, r *http.Request) {
	source := r.URL.Query().Get("source")
	page := 1
	if p := r.URL.Query().Get("page"); p != "" {
		if n, err := strconv.Atoi(p); err == nil && n > 0 {
			page = n
		}
	}

	logs, total, err := h.webhookRepo.List(r.Context(), source, page, 50)
	if err != nil {
		apperrors.WriteError(w, http.StatusInternalServerError, apperrors.ErrInternal, middleware.GetRequestID(r.Context()))
		return
	}

	apperrors.WriteSuccess(w, http.StatusOK, map[string]any{
		"logs":  logs,
		"total": total,
		"page":  page,
	}, middleware.GetRequestID(r.Context()))
}

func (h *AdminHandler) GetOrderDetail(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	order, err := h.paymentSvc.GetOrder(r.Context(), id)
	if err != nil {
		apperrors.WriteError(w, http.StatusNotFound, apperrors.ErrNotFound, middleware.GetRequestID(r.Context()))
		return
	}

	history, _ := h.paymentSvc.GetOrderStatusHistory(r.Context(), id)

	apperrors.WriteSuccess(w, http.StatusOK, map[string]any{
		"order":   order,
		"history": history,
	}, middleware.GetRequestID(r.Context()))
}

func (h *AdminHandler) OverrideOrderStatus(w http.ResponseWriter, r *http.Request) {
	var req struct {
		OrderID string `json:"order_id"`
		Status  string `json:"status"`
		Reason  string `json:"reason"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		io.Copy(io.Discard, r.Body)
		r.Body.Close()
		apperrors.WriteError(w, http.StatusBadRequest, apperrors.ErrInvalidInput, middleware.GetRequestID(r.Context()))
		return
	}

	validStatuses := map[string]bool{"failed": true, "expired": true, "cancelled": true}
	if !validStatuses[req.Status] {
		apperrors.WriteError(w, http.StatusBadRequest, apperrors.FieldError("status", "must be failed, expired, or cancelled"), middleware.GetRequestID(r.Context()))
		return
	}

	order, err := h.paymentSvc.GetOrder(r.Context(), req.OrderID)
	if err != nil {
		apperrors.WriteError(w, http.StatusNotFound, apperrors.ErrNotFound, middleware.GetRequestID(r.Context()))
		return
	}

	if order.Status == "success" {
		apperrors.WriteError(w, http.StatusBadRequest, apperrors.FieldError("status", "cannot override a successful order"), middleware.GetRequestID(r.Context()))
		return
	}

	if err := h.paymentSvc.UpdateOrderStatus(r.Context(), req.OrderID, req.Status); err != nil {
		apperrors.WriteError(w, http.StatusInternalServerError, apperrors.ErrInternal, middleware.GetRequestID(r.Context()))
		return
	}

	reason := req.Reason
	if reason == "" {
		reason = "admin override"
	}
	h.logAudit(r.Context(), r, "override_status", "order", order.ID, order.Status, req.Status)
	h.paymentSvc.RecordStatusChange(r.Context(), req.OrderID, order.Status, req.Status, reason)

	if req.OrderID != "" && order.UserPhone != "" {
		go func() {
			msg := fmt.Sprintf("Order %s telah diubah statusnya menjadi %s. Alasan: %s", req.OrderID, req.Status, reason)
			if err := h.notifySvc.SendNotification(order.UserPhone, msg); err != nil {
				h.logger.Error("failed to notify status override", slog.String("order_id", req.OrderID), slog.String("error", err.Error()))
			}
		}()
	}

	apperrors.WriteSuccess(w, http.StatusOK, map[string]string{
		"status":  "ok",
		"message": "order status overridden",
	}, middleware.GetRequestID(r.Context()))
}

func validCustomerNoFormat(format string) bool {
	switch format {
	case "", "uid", "uid_server_pipe", "uid_server_concat", "uid_server_space":
		return true
	default:
		return false
	}
}

func validProductType(productType string) bool {
	switch productType {
	case constants.ProductTypeDiamond, constants.ProductTypeSubscription, constants.ProductTypeOther, constants.ProductTypeValidation:
		return true
	default:
		return false
	}
}

func (h *AdminHandler) GetSupplierStatus(w http.ResponseWriter, r *http.Request) {
	status := h.topupSvc.GetSupplierStatus()
	apperrors.WriteSuccess(w, http.StatusOK, status, middleware.GetRequestID(r.Context()))
}

func (h *AdminHandler) ListBlockedIdentities(w http.ResponseWriter, r *http.Request) {
	if h.blockedIdentityRepo == nil {
		apperrors.WriteError(w, http.StatusServiceUnavailable, apperrors.ErrInternal, middleware.GetRequestID(r.Context()))
		return
	}
	list, err := h.blockedIdentityRepo.List(r.Context())
	if err != nil {
		h.logger.Error("list blocked identities", slog.String("error", err.Error()))
		apperrors.WriteError(w, http.StatusInternalServerError, apperrors.ErrInternal, middleware.GetRequestID(r.Context()))
		return
	}
	apperrors.WriteSuccess(w, http.StatusOK, list, middleware.GetRequestID(r.Context()))
}

func (h *AdminHandler) CreateBlockedIdentity(w http.ResponseWriter, r *http.Request) {
	if h.blockedIdentityRepo == nil {
		apperrors.WriteError(w, http.StatusServiceUnavailable, apperrors.ErrInternal, middleware.GetRequestID(r.Context()))
		return
	}
	var req struct {
		Phone     *string `json:"phone"`
		GameUID   *string `json:"game_uid"`
		IPAddress *string `json:"ip_address"`
		Reason    string  `json:"reason"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		io.Copy(io.Discard, r.Body)
		r.Body.Close()
		apperrors.WriteError(w, http.StatusBadRequest, apperrors.ErrInvalidInput, middleware.GetRequestID(r.Context()))
		return
	}
	if req.Phone == nil && req.GameUID == nil && req.IPAddress == nil {
		apperrors.WriteError(w, http.StatusBadRequest, apperrors.FieldError("input", "at least one of phone, game_uid, or ip_address is required"), middleware.GetRequestID(r.Context()))
		return
	}
	if req.Reason == "" {
		apperrors.WriteError(w, http.StatusBadRequest, apperrors.FieldError("reason", "reason is required"), middleware.GetRequestID(r.Context()))
		return
	}
	b := &models.BlockedIdentity{
		ID:        uuid.New().String(),
		Phone:     req.Phone,
		GameUID:   req.GameUID,
		IPAddress: req.IPAddress,
		Reason:    req.Reason,
		BlockedBy: "admin",
	}
	if err := h.blockedIdentityRepo.Create(r.Context(), b); err != nil {
		h.logger.Error("create blocked identity", slog.String("error", err.Error()))
		apperrors.WriteError(w, http.StatusInternalServerError, apperrors.ErrInternal, middleware.GetRequestID(r.Context()))
		return
	}
	h.logAudit(r.Context(), r, "block_identity", "blocked_identity", b.ID, "", req.Reason)
	apperrors.WriteSuccess(w, http.StatusCreated, b, middleware.GetRequestID(r.Context()))
}

func (h *AdminHandler) DeleteBlockedIdentity(w http.ResponseWriter, r *http.Request) {
	if h.blockedIdentityRepo == nil {
		apperrors.WriteError(w, http.StatusServiceUnavailable, apperrors.ErrInternal, middleware.GetRequestID(r.Context()))
		return
	}
	id := r.PathValue("id")
	if id == "" {
		apperrors.WriteError(w, http.StatusBadRequest, apperrors.FieldError("id", "id is required"), middleware.GetRequestID(r.Context()))
		return
	}
	if err := h.blockedIdentityRepo.Delete(r.Context(), id); err != nil {
		h.logger.Error("delete blocked identity", slog.String("error", err.Error()))
		apperrors.WriteError(w, http.StatusInternalServerError, apperrors.ErrInternal, middleware.GetRequestID(r.Context()))
		return
	}
	h.logAudit(r.Context(), r, "unblock_identity", "blocked_identity", id, "", "")
	apperrors.WriteSuccess(w, http.StatusOK, map[string]string{"status": "ok"}, middleware.GetRequestID(r.Context()))
}

func (h *AdminHandler) ListReferralCodes(w http.ResponseWriter, r *http.Request) {
	if h.referralCodeRepo == nil {
		apperrors.WriteError(w, http.StatusServiceUnavailable, apperrors.ErrInternal, middleware.GetRequestID(r.Context()))
		return
	}
	codes, err := h.referralCodeRepo.List(r.Context())
	if err != nil {
		h.logger.Error("list referral codes", slog.String("error", err.Error()))
		apperrors.WriteError(w, http.StatusInternalServerError, apperrors.ErrInternal, middleware.GetRequestID(r.Context()))
		return
	}
	apperrors.WriteSuccess(w, http.StatusOK, codes, middleware.GetRequestID(r.Context()))
}

func (h *AdminHandler) CreateReferralCode(w http.ResponseWriter, r *http.Request) {
	if h.referralCodeRepo == nil {
		apperrors.WriteError(w, http.StatusServiceUnavailable, apperrors.ErrInternal, middleware.GetRequestID(r.Context()))
		return
	}
	var req struct {
		Code         string `json:"code"`
		OwnerPhone   string `json:"owner_phone"`
		DiscountIDR  int    `json:"discount_idr"`
		RewardPoints int    `json:"reward_points"`
		MinOrderIDR  int    `json:"min_order_idr"`
		MaxUses      int    `json:"max_uses"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		apperrors.WriteError(w, http.StatusBadRequest, apperrors.FieldError("body", "invalid JSON"), middleware.GetRequestID(r.Context()))
		return
	}
	if req.Code == "" {
		apperrors.WriteError(w, http.StatusBadRequest, apperrors.FieldError("code", "code is required"), middleware.GetRequestID(r.Context()))
		return
	}
	req.Code = strings.ToUpper(strings.TrimSpace(req.Code))
	if req.DiscountIDR <= 0 {
		apperrors.WriteError(w, http.StatusBadRequest, apperrors.FieldError("discount_idr", "discount must be positive"), middleware.GetRequestID(r.Context()))
		return
	}
	req.OwnerPhone = strings.TrimSpace(req.OwnerPhone)
	if req.RewardPoints < 0 {
		apperrors.WriteError(w, http.StatusBadRequest, apperrors.FieldError("reward_points", "reward points cannot be negative"), middleware.GetRequestID(r.Context()))
		return
	}
	if req.MinOrderIDR < 0 {
		apperrors.WriteError(w, http.StatusBadRequest, apperrors.FieldError("min_order_idr", "minimum order cannot be negative"), middleware.GetRequestID(r.Context()))
		return
	}
	if req.MaxUses <= 0 {
		req.MaxUses = 0
	}
	rc := &models.ReferralCode{
		Code:         req.Code,
		OwnerPhone:   req.OwnerPhone,
		DiscountIDR:  req.DiscountIDR,
		RewardPoints: req.RewardPoints,
		MinOrderIDR:  req.MinOrderIDR,
		MaxUses:      req.MaxUses,
		IsActive:     true,
	}
	if err := h.referralCodeRepo.Create(r.Context(), rc); err != nil {
		h.logger.Error("create referral code", slog.String("error", err.Error()))
		apperrors.WriteError(w, http.StatusInternalServerError, apperrors.ErrInternal, middleware.GetRequestID(r.Context()))
		return
	}
	h.logAudit(r.Context(), r, "create_referral_code", "referral_code", rc.ID, "", req.Code)
	apperrors.WriteSuccess(w, http.StatusCreated, rc, middleware.GetRequestID(r.Context()))
}

func (h *AdminHandler) DeleteReferralCode(w http.ResponseWriter, r *http.Request) {
	if h.referralCodeRepo == nil {
		apperrors.WriteError(w, http.StatusServiceUnavailable, apperrors.ErrInternal, middleware.GetRequestID(r.Context()))
		return
	}
	id := r.PathValue("id")
	if id == "" {
		apperrors.WriteError(w, http.StatusBadRequest, apperrors.FieldError("id", "id is required"), middleware.GetRequestID(r.Context()))
		return
	}
	if err := h.referralCodeRepo.Delete(r.Context(), id); err != nil {
		h.logger.Error("delete referral code", slog.String("error", err.Error()))
		apperrors.WriteError(w, http.StatusInternalServerError, apperrors.ErrInternal, middleware.GetRequestID(r.Context()))
		return
	}
	h.logAudit(r.Context(), r, "delete_referral_code", "referral_code", id, "", "")
	apperrors.WriteSuccess(w, http.StatusOK, map[string]string{"status": "ok"}, middleware.GetRequestID(r.Context()))
}

func (h *AdminHandler) ListReferralPointBalances(w http.ResponseWriter, r *http.Request) {
	if h.referralCodeRepo == nil {
		apperrors.WriteError(w, http.StatusServiceUnavailable, apperrors.ErrInternal, middleware.GetRequestID(r.Context()))
		return
	}
	balances, err := h.referralCodeRepo.ListPointBalances(r.Context())
	if err != nil {
		h.logger.Error("list referral point balances", slog.String("error", err.Error()))
		apperrors.WriteError(w, http.StatusInternalServerError, apperrors.ErrInternal, middleware.GetRequestID(r.Context()))
		return
	}
	apperrors.WriteSuccess(w, http.StatusOK, balances, middleware.GetRequestID(r.Context()))
}

func (h *AdminHandler) RedeemReferralPoints(w http.ResponseWriter, r *http.Request) {
	if h.referralCodeRepo == nil {
		apperrors.WriteError(w, http.StatusServiceUnavailable, apperrors.ErrInternal, middleware.GetRequestID(r.Context()))
		return
	}
	var req struct {
		OwnerPhone string `json:"owner_phone"`
		Points     int    `json:"points"`
		CouponCode string `json:"coupon_code"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		apperrors.WriteError(w, http.StatusBadRequest, apperrors.FieldError("body", "invalid JSON"), middleware.GetRequestID(r.Context()))
		return
	}
	req.OwnerPhone = strings.TrimSpace(req.OwnerPhone)
	req.CouponCode = strings.ToUpper(strings.TrimSpace(req.CouponCode))
	if req.OwnerPhone == "" {
		apperrors.WriteError(w, http.StatusBadRequest, apperrors.FieldError("owner_phone", "owner phone is required"), middleware.GetRequestID(r.Context()))
		return
	}
	if req.Points <= 0 {
		apperrors.WriteError(w, http.StatusBadRequest, apperrors.FieldError("points", "points must be positive"), middleware.GetRequestID(r.Context()))
		return
	}
	if req.CouponCode == "" {
		token := strings.ReplaceAll(uuid.NewString(), "-", "")
		req.CouponCode = "REF" + strings.ToUpper(token[:8])
	}
	if err := h.referralCodeRepo.RedeemPoints(r.Context(), req.OwnerPhone, req.CouponCode, req.Points); err != nil {
		h.logger.Error("redeem referral points", slog.String("error", err.Error()), slog.String("owner_phone", req.OwnerPhone))
		apperrors.WriteError(w, http.StatusBadRequest, apperrors.FieldError("points", err.Error()), middleware.GetRequestID(r.Context()))
		return
	}
	h.logAudit(r.Context(), r, "redeem_referral_points", "referral_points", req.OwnerPhone, "", req.CouponCode)
	apperrors.WriteSuccess(w, http.StatusOK, map[string]any{
		"coupon_code": req.CouponCode,
		"points":      req.Points,
	}, middleware.GetRequestID(r.Context()))
}
