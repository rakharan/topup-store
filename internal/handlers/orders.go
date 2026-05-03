package handlers

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"regexp"
	"strconv"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/topup-store/internal/apperrors"
	"github.com/topup-store/internal/constants"
	"github.com/topup-store/internal/middleware"
	"github.com/topup-store/internal/models"
	"github.com/topup-store/internal/services"
)

type OrderHandler struct {
	paymentSvc services.PaymentServiceInterface
	topupSvc   services.TopupServiceInterface
	notifySvc  services.NotifyServiceInterface
	pool       *pgxpool.Pool
	rootCtx    context.Context
	logger     *slog.Logger
}

func NewOrderHandler(paymentSvc services.PaymentServiceInterface, topupSvc services.TopupServiceInterface, notifySvc services.NotifyServiceInterface, pool *pgxpool.Pool, rootCtx context.Context, logger *slog.Logger) *OrderHandler {
	return &OrderHandler{
		paymentSvc: paymentSvc,
		topupSvc:   topupSvc,
		notifySvc:  notifySvc,
		pool:       pool,
		rootCtx:    rootCtx,
		logger:     logger,
	}
}

func (h *OrderHandler) CreateOrder(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Game       string `json:"game"`
		GameUID    string `json:"game_uid"`
		GameServer string `json:"game_server"`
		ProductID  string `json:"product_id"`
		Diamonds   int    `json:"diamonds"`
		Phone      string `json:"phone"`
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		h.logger.Error("create order: decode error", slog.String("error", err.Error()), slog.String("request_id", middleware.GetRequestID(r.Context())))
		apperrors.WriteError(w, http.StatusBadRequest, apperrors.ErrInvalidInput, middleware.GetRequestID(r.Context()))
		return
	}

	h.logger.Info("create order request",
		slog.String("game", req.Game),
		slog.String("uid", req.GameUID),
		slog.String("product_id", req.ProductID),
		slog.String("request_id", middleware.GetRequestID(r.Context())),
	)

	if err := validateOrderInput(req); err != nil {
		h.logger.Warn("create order: validation error", slog.String("error", err.Error()))
		apperrors.WriteError(w, http.StatusBadRequest, apperrors.FieldError("input", err.Error()), middleware.GetRequestID(r.Context()))
		return
	}

	var product *models.Product
	var err error

	if req.ProductID != "" {
		product, err = h.topupSvc.GetProduct(r.Context(), req.ProductID)
	} else {
		product, err = h.topupSvc.GetProductByGameAndDiamonds(r.Context(), req.Game, req.Diamonds)
	}

	if err != nil {
		apperrors.WriteError(w, http.StatusBadRequest, apperrors.FieldError("product_id", "product not found"), middleware.GetRequestID(r.Context()))
		return
	}

	orderID := uuid.New().String()
	var orderNumber string
	if h.pool != nil {
		orderNumber, err = services.GenerateOrderNumber(r.Context(), h.pool)
		if err != nil {
			h.logger.Error("generate order number", slog.String("error", err.Error()))
			apperrors.WriteError(w, http.StatusInternalServerError, apperrors.ErrInternal, middleware.GetRequestID(r.Context()))
			return
		}
	} else {
		orderNumber = "FT-TEST-0001"
	}
	order := &models.Order{
		ID:             orderID,
		OrderNumber:    orderNumber,
		ProductID:      product.ID,
		UserPhone:      req.Phone,
		GameUID:        req.GameUID,
		GameServer:     req.GameServer,
		AmountIDR:      product.PriceIDR,
		Status:         constants.StatusPending,
		Channel:        constants.ChannelWeb,
		DigiflazzRefID: orderID,
	}

	if err := h.paymentSvc.CreateOrder(r.Context(), order); err != nil {
		h.logger.Error("create order", slog.String("error", err.Error()))
		apperrors.WriteError(w, http.StatusInternalServerError, apperrors.ErrInternal, middleware.GetRequestID(r.Context()))
		return
	}

	qrisURL, qrisBase64, err := h.paymentSvc.CreateQRIS(r.Context(), order)
	if err != nil {
		h.logger.Error("create qris", slog.String("error", err.Error()))
		apperrors.WriteError(w, http.StatusInternalServerError, apperrors.ErrInternal, middleware.GetRequestID(r.Context()))
		return
	}

	orderCopy := *order
	productCopy := *product
	qrisURLCopy := qrisURL
	go func() {
		defer func() {
			if r := recover(); r != nil {
				h.logger.Error("order notification panicked", slog.String("order_id", orderCopy.ID), slog.Any("panic", r))
			}
		}()
		ctx, cancel := context.WithTimeout(h.rootCtx, 10*time.Second)
		defer cancel()
		if err := h.notifySvc.SendOrderConfirmation(ctx, &orderCopy, &productCopy, orderCopy.UserPhone, qrisURLCopy); err != nil {
			h.logger.Error("failed to send notification", slog.String("order_id", orderCopy.ID), slog.String("error", err.Error()))
		}
	}()

	apperrors.WriteSuccess(w, http.StatusCreated, map[string]any{
		"order_id":     orderID,
		"order_number": orderNumber,
		"amount_idr":   order.AmountIDR,
		"qris_url":     qrisURL,
		"qris_base64":  qrisBase64,
	}, middleware.GetRequestID(r.Context()))
}

func (h *OrderHandler) GetOrder(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	order, err := h.paymentSvc.GetOrder(r.Context(), id)
	if err != nil {
		apperrors.WriteError(w, http.StatusNotFound, apperrors.ErrNotFound, middleware.GetRequestID(r.Context()))
		return
	}
	apperrors.WriteSuccess(w, http.StatusOK, order, middleware.GetRequestID(r.Context()))
}

func (h *OrderHandler) ListOrders(w http.ResponseWriter, r *http.Request) {
	orders, _, err := h.paymentSvc.ListOrders(r.Context(), 1, 50)
	if err != nil {
		apperrors.WriteError(w, http.StatusInternalServerError, apperrors.ErrInternal, middleware.GetRequestID(r.Context()))
		return
	}
	apperrors.WriteSuccess(w, http.StatusOK, orders, middleware.GetRequestID(r.Context()))
}

func (h *OrderHandler) LookupOrder(w http.ResponseWriter, r *http.Request) {
	var req struct {
		GameUID string `json:"game_uid"`
		Phone   string `json:"phone"`
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		apperrors.WriteError(w, http.StatusBadRequest, apperrors.ErrInvalidInput, middleware.GetRequestID(r.Context()))
		return
	}

	if req.GameUID == "" || req.Phone == "" {
		apperrors.WriteError(w, http.StatusBadRequest, apperrors.FieldError("input", "game_uid and phone are required"), middleware.GetRequestID(r.Context()))
		return
	}

	order, err := h.paymentSvc.GetOrderByUIDAndPhone(r.Context(), req.GameUID, req.Phone)
	if err != nil {
		apperrors.WriteError(w, http.StatusNotFound, apperrors.ErrNotFound, middleware.GetRequestID(r.Context()))
		return
	}

	apperrors.WriteSuccess(w, http.StatusOK, order, middleware.GetRequestID(r.Context()))
}

func (h *OrderHandler) RecentOrders(w http.ResponseWriter, r *http.Request) {
	phone := r.URL.Query().Get("phone")
	if phone == "" {
		apperrors.WriteError(w, http.StatusBadRequest, apperrors.FieldError("phone", "phone is required"), middleware.GetRequestID(r.Context()))
		return
	}

	limit := 3
	if l := r.URL.Query().Get("limit"); l != "" {
		if n, err := strconv.Atoi(l); err == nil && n > 0 && n <= 10 {
			limit = n
		}
	}

	orders, err := h.paymentSvc.GetRecentOrdersByPhone(r.Context(), phone, limit)
	if err != nil {
		apperrors.WriteError(w, http.StatusInternalServerError, apperrors.ErrInternal, middleware.GetRequestID(r.Context()))
		return
	}

	apperrors.WriteSuccess(w, http.StatusOK, orders, middleware.GetRequestID(r.Context()))
}

func (h *OrderHandler) CancelOrder(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	order, err := h.paymentSvc.GetOrder(r.Context(), id)
	if err != nil {
		apperrors.WriteError(w, http.StatusNotFound, apperrors.ErrNotFound, middleware.GetRequestID(r.Context()))
		return
	}

	if order.Status != constants.StatusPending {
		apperrors.WriteError(w, http.StatusBadRequest, apperrors.FieldError("status", "only pending orders can be cancelled"), middleware.GetRequestID(r.Context()))
		return
	}

	if err := h.paymentSvc.CancelTransaction(id); err != nil {
		h.logger.Warn("cancel order: midtrans cancel failed (may be already expired)", slog.String("order_id", id), slog.String("error", err.Error()))
	}

	if err := h.paymentSvc.UpdateOrderStatus(r.Context(), id, constants.StatusCancelled); err != nil {
		h.logger.Error("cancel order: update status failed", slog.String("order_id", id), slog.String("error", err.Error()))
		apperrors.WriteError(w, http.StatusInternalServerError, apperrors.ErrInternal, middleware.GetRequestID(r.Context()))
		return
	}

	h.paymentSvc.RecordStatusChange(r.Context(), id, order.Status, constants.StatusCancelled, "user cancelled")

	orderCopy := *order
	go func() {
		defer func() {
			if r := recover(); r != nil {
				h.logger.Error("cancel notification panicked", slog.String("order_id", orderCopy.ID), slog.Any("panic", r))
			}
		}()
		msg := "Order " + orderCopy.ID + " telah dibatalkan. Jika sudah terlanjur bayar, hubungi admin untuk refund."
		if err := h.notifySvc.SendNotification(orderCopy.UserPhone, msg); err != nil {
			h.logger.Error("cancel order: failed to notify user", slog.String("order_id", orderCopy.ID), slog.String("error", err.Error()))
		}
	}()

	apperrors.WriteSuccess(w, http.StatusOK, map[string]string{
		"status":  "ok",
		"message": "order cancelled",
	}, middleware.GetRequestID(r.Context()))
}

var (
	reNumeric     = regexp.MustCompile(`^\d+$`)
	reNumericPair = regexp.MustCompile(`^\d+\|\d+$`)
	rePhone       = regexp.MustCompile(`^\d{8,20}$`)
)

func validateOrderInput(req struct {
	Game       string `json:"game"`
	GameUID    string `json:"game_uid"`
	GameServer string `json:"game_server"`
	ProductID  string `json:"product_id"`
	Diamonds   int    `json:"diamonds"`
	Phone      string `json:"phone"`
}) error {
	if !constants.ValidGames[req.Game] {
		return &validationError{field: "game", message: "must be free_fire, mobile_legends, or pubg_mobile"}
	}
	if req.GameUID == "" {
		return &validationError{field: "game_uid", message: "is required"}
	}
	if req.Game == constants.GameMobileLegends {
		if req.GameServer == "" {
			return &validationError{field: "game_server", message: "is required for Mobile Legends"}
		}
		combined := req.GameUID + "|" + req.GameServer
		if !reNumericPair.MatchString(combined) {
			return &validationError{field: "game_uid", message: "must be numeric|numeric format (e.g. 12345|1234)"}
		}
	} else {
		if !reNumeric.MatchString(req.GameUID) {
			return &validationError{field: "game_uid", message: "must be numeric"}
		}
	}
	if !rePhone.MatchString(req.Phone) {
		return &validationError{field: "phone", message: "must be numeric (min 8 digits)"}
	}
	if req.ProductID == "" && req.Diamonds <= 0 {
		return &validationError{field: "product_id", message: "either product_id or diamonds is required"}
	}
	return nil
}

type validationError struct {
	field   string
	message string
}

func (e *validationError) Error() string {
	return e.field + ": " + e.message
}
