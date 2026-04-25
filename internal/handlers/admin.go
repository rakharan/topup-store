package handlers

import (
	"encoding/json"
	"log/slog"
	"net/http"

	"github.com/google/uuid"
	"github.com/topup-store/internal/apperrors"
	"github.com/topup-store/internal/constants"
	"github.com/topup-store/internal/middleware"
	"github.com/topup-store/internal/models"
	"github.com/topup-store/internal/repositories"
	"github.com/topup-store/internal/services"
)

type AdminHandler struct {
	paymentSvc services.PaymentServiceInterface
	topupSvc   services.TopupServiceInterface
	notifySvc  services.NotifyServiceInterface
	productRepo repositories.ProductRepository
	adminPass  string
	logger     *slog.Logger
}

func NewAdminHandler(paymentSvc services.PaymentServiceInterface, topupSvc services.TopupServiceInterface, notifySvc services.NotifyServiceInterface, productRepo repositories.ProductRepository, adminPass string, logger *slog.Logger) *AdminHandler {
	return &AdminHandler{
		paymentSvc:  paymentSvc,
		topupSvc:    topupSvc,
		notifySvc:   notifySvc,
		productRepo: productRepo,
		adminPass:   adminPass,
		logger:      logger,
	}
}

func (h *AdminHandler) ProcessOrder(w http.ResponseWriter, r *http.Request) {
	var req struct {
		OrderID string `json:"order_id"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
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

	orderCopy := *order
	go func() {
		if err := h.topupSvc.ProcessOrder(orderCopy.ID); err != nil {
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

	if order.Status != constants.StatusPaid && order.Status != constants.StatusProcessing {
		apperrors.WriteError(w, http.StatusBadRequest, apperrors.FieldError("status", "order must be in paid or processing status, current: "+order.Status), middleware.GetRequestID(r.Context()))
		return
	}

	orderCopy := *order
	go func() {
		if err := h.topupSvc.ProcessOrder(orderCopy.ID); err != nil {
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
		Game        string `json:"game"`
		Name        string `json:"name"`
		Description string `json:"description"`
		PriceIDR    int    `json:"price_idr"`
		Diamonds    int    `json:"diamonds"`
		SKU         string `json:"sku"`
		IsActive    bool   `json:"is_active"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		apperrors.WriteError(w, http.StatusBadRequest, apperrors.ErrInvalidInput, middleware.GetRequestID(r.Context()))
		return
	}

	if !constants.ValidGames[req.Game] {
		apperrors.WriteError(w, http.StatusBadRequest, apperrors.FieldError("game", "must be free_fire, mobile_legends, or pubg_mobile"), middleware.GetRequestID(r.Context()))
		return
	}
	if req.Name == "" || req.SKU == "" || req.PriceIDR <= 0 {
		apperrors.WriteError(w, http.StatusBadRequest, apperrors.FieldError("input", "name, sku, and price_idr are required"), middleware.GetRequestID(r.Context()))
		return
	}

	exists, _ := h.productRepo.ExistsBySKU(r.Context(), req.SKU, "")
	if exists {
		apperrors.WriteError(w, http.StatusConflict, apperrors.FieldError("sku", "SKU already exists"), middleware.GetRequestID(r.Context()))
		return
	}

	product := &models.Product{
		ID:          uuid.New().String(),
		Game:        req.Game,
		Name:        req.Name,
		Description: req.Description,
		PriceIDR:    req.PriceIDR,
		Diamonds:    req.Diamonds,
		SKU:         req.SKU,
		IsActive:    req.IsActive,
	}

	if err := h.productRepo.Create(r.Context(), product); err != nil {
		h.logger.Error("create product", slog.String("error", err.Error()))
		apperrors.WriteError(w, http.StatusInternalServerError, apperrors.ErrInternal, middleware.GetRequestID(r.Context()))
		return
	}

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
		Game        string `json:"game"`
		Name        string `json:"name"`
		Description string `json:"description"`
		PriceIDR    int    `json:"price_idr"`
		Diamonds    int    `json:"diamonds"`
		SKU         string `json:"sku"`
		IsActive    bool   `json:"is_active"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		apperrors.WriteError(w, http.StatusBadRequest, apperrors.ErrInvalidInput, middleware.GetRequestID(r.Context()))
		return
	}

	if !constants.ValidGames[req.Game] {
		apperrors.WriteError(w, http.StatusBadRequest, apperrors.FieldError("game", "must be free_fire, mobile_legends, or pubg_mobile"), middleware.GetRequestID(r.Context()))
		return
	}
	if req.Name == "" || req.SKU == "" || req.PriceIDR <= 0 {
		apperrors.WriteError(w, http.StatusBadRequest, apperrors.FieldError("input", "name, sku, and price_idr are required"), middleware.GetRequestID(r.Context()))
		return
	}

	exists, _ := h.productRepo.ExistsBySKU(r.Context(), req.SKU, id)
	if exists {
		apperrors.WriteError(w, http.StatusConflict, apperrors.FieldError("sku", "SKU already exists"), middleware.GetRequestID(r.Context()))
		return
	}

	product := &models.Product{
		ID:          existing.ID,
		Game:        req.Game,
		Name:        req.Name,
		Description: req.Description,
		PriceIDR:    req.PriceIDR,
		Diamonds:    req.Diamonds,
		SKU:         req.SKU,
		IsActive:    req.IsActive,
	}

	if err := h.productRepo.Update(r.Context(), product); err != nil {
		h.logger.Error("update product", slog.String("error", err.Error()))
		apperrors.WriteError(w, http.StatusInternalServerError, apperrors.ErrInternal, middleware.GetRequestID(r.Context()))
		return
	}

	apperrors.WriteSuccess(w, http.StatusOK, product, middleware.GetRequestID(r.Context()))
}

func (h *AdminHandler) DeleteProduct(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if err := h.productRepo.Delete(r.Context(), id); err != nil {
		apperrors.WriteError(w, http.StatusNotFound, apperrors.ErrNotFound, middleware.GetRequestID(r.Context()))
		return
	}
	apperrors.WriteSuccess(w, http.StatusOK, map[string]string{"status": "ok", "message": "product deleted"}, middleware.GetRequestID(r.Context()))
}
