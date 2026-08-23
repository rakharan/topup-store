package handlers

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/topup-store/internal/apperrors"
	"github.com/topup-store/internal/constants"
	"github.com/topup-store/internal/middleware"
	"github.com/topup-store/internal/models"
	"github.com/topup-store/internal/repositories"
	"github.com/topup-store/internal/services"
)

type OrderHandler struct {
	paymentSvc          services.PaymentServiceInterface
	topupSvc            services.TopupServiceInterface
	notifySvc           services.NotifyServiceInterface
	blockedIdentityRepo repositories.BlockedIdentityRepository
	referralCodeRepo    repositories.ReferralCodeRepository
	pool                *pgxpool.Pool
	rootCtx             context.Context
	logger              *slog.Logger
}

func NewOrderHandler(paymentSvc services.PaymentServiceInterface, topupSvc services.TopupServiceInterface, notifySvc services.NotifyServiceInterface, blockedIdentityRepo repositories.BlockedIdentityRepository, referralCodeRepo repositories.ReferralCodeRepository, pool *pgxpool.Pool, rootCtx context.Context, logger *slog.Logger) *OrderHandler {
	return &OrderHandler{
		paymentSvc:          paymentSvc,
		topupSvc:            topupSvc,
		notifySvc:           notifySvc,
		blockedIdentityRepo: blockedIdentityRepo,
		referralCodeRepo:    referralCodeRepo,
		pool:                pool,
		rootCtx:             rootCtx,
		logger:              logger,
	}
}

func (h *OrderHandler) CreateOrder(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Game       string `json:"game"`
		GameUID    string `json:"game_uid"`
		GameServer string `json:"game_server"`
		ProductID  string `json:"product_id"`
		ItemQty    int    `json:"item_qty"`
		Phone      string `json:"phone"`
		CouponCode string `json:"coupon_code"`
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		drainBody(r)
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

	if h.blockedIdentityRepo != nil {
		blocked, reason, err := h.blockedIdentityRepo.IsBlocked(r.Context(), req.Phone, req.GameUID, r.RemoteAddr)
		if err != nil {
			h.logger.Error("blocked identity check failed", slog.String("error", err.Error()))
		} else if blocked {
			h.logger.Warn("blocked identity attempted order",
				slog.String("phone", req.Phone),
				slog.String("game_uid", req.GameUID),
				slog.String("ip", r.RemoteAddr),
				slog.String("reason", reason),
			)
			apperrors.WriteError(w, http.StatusForbidden, apperrors.FieldError("account", "account blocked: "+reason), middleware.GetRequestID(r.Context()))
			return
		}
	}

	if err := validateOrderInput(struct {
		Game       string `json:"game"`
		GameUID    string `json:"game_uid"`
		GameServer string `json:"game_server"`
		ProductID  string `json:"product_id"`
		ItemQty    int    `json:"item_qty"`
		Phone      string `json:"phone"`
	}{Game: req.Game, GameUID: req.GameUID, GameServer: req.GameServer, ProductID: req.ProductID, ItemQty: req.ItemQty, Phone: req.Phone}); err != nil {
		h.logger.Warn("create order: validation error", slog.String("error", err.Error()))
		apperrors.WriteError(w, http.StatusBadRequest, apperrors.FieldError("input", err.Error()), middleware.GetRequestID(r.Context()))
		return
	}

	var product *models.Product
	var err error

	if req.ProductID != "" {
		product, err = h.topupSvc.GetProduct(r.Context(), req.ProductID)
		if err != nil && req.ItemQty > 0 {
			product, err = h.topupSvc.GetProductByGameAndDiamonds(r.Context(), req.Game, req.ItemQty)
		}
	} else {
		product, err = h.topupSvc.GetProductByGameAndDiamonds(r.Context(), req.Game, req.ItemQty)
	}

	if err != nil {
		apperrors.WriteError(w, http.StatusBadRequest, apperrors.FieldError("product_id", "product not found"), middleware.GetRequestID(r.Context()))
		return
	}
	if product.ProductType == constants.ProductTypeValidation {
		apperrors.WriteError(w, http.StatusBadRequest, apperrors.FieldError("product_id", "product not available for purchase"), middleware.GetRequestID(r.Context()))
		return
	}

	if product.Stock == 0 {
		h.logger.Warn("create order: product out of stock", slog.String("product_id", product.ID))
		apperrors.WriteError(w, http.StatusBadRequest, apperrors.FieldError("product", "product is out of stock"), middleware.GetRequestID(r.Context()))
		return
	}

	subtotalIDR := product.PriceIDR
	discountIDR, couponCode, err := h.calculateCouponDiscount(r.Context(), req.CouponCode, req.Game, subtotalIDR)
	if err != nil {
		apperrors.WriteError(w, http.StatusBadRequest, apperrors.FieldError("coupon_code", err.Error()), middleware.GetRequestID(r.Context()))
		return
	}
	amountIDR := subtotalIDR - discountIDR
	if amountIDR < 1000 {
		apperrors.WriteError(w, http.StatusBadRequest, apperrors.FieldError("coupon_code", "discount makes payment amount too low"), middleware.GetRequestID(r.Context()))
		return
	}

	validation, err := h.topupSvc.ValidateCustomer(r.Context(), req.Game, req.GameUID, req.GameServer)
	if err != nil {
		h.logger.Warn("create order: customer validation failed",
			slog.String("game", req.Game),
			slog.String("uid", req.GameUID),
			slog.String("server", req.GameServer),
			slog.String("error", err.Error()),
		)
		apperrors.WriteError(w, http.StatusBadRequest, apperrors.FieldError("game_uid", "UID/server tidak valid"), middleware.GetRequestID(r.Context()))
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
	stockReserved := false
	if product.Stock > 0 {
		ok, err := h.topupSvc.DecrementProductStock(r.Context(), product.ID)
		if err != nil {
			h.logger.Error("create order: failed to decrement stock", slog.String("product_id", product.ID), slog.String("error", err.Error()))
			apperrors.WriteError(w, http.StatusInternalServerError, apperrors.ErrInternal, middleware.GetRequestID(r.Context()))
			return
		}
		if !ok {
			h.logger.Warn("create order: product out of stock (race)", slog.String("product_id", product.ID))
			apperrors.WriteError(w, http.StatusBadRequest, apperrors.FieldError("product", "product is out of stock"), middleware.GetRequestID(r.Context()))
			return
		}
		stockReserved = true
	}

	order := &models.Order{
		ID:             orderID,
		OrderNumber:    orderNumber,
		ProductID:      product.ID,
		UserPhone:      req.Phone,
		GameUID:        req.GameUID,
		GameServer:     req.GameServer,
		AmountIDR:      amountIDR,
		Status:         constants.StatusPending,
		Channel:        constants.ChannelWeb,
		DigiflazzRefID: orderID,
		StockReserved:  stockReserved,
	}

	if err := h.paymentSvc.CreateOrder(r.Context(), order); err != nil {
		h.logger.Error("create order", slog.String("error", err.Error()))
		apperrors.WriteError(w, http.StatusInternalServerError, apperrors.ErrInternal, middleware.GetRequestID(r.Context()))
		return
	}
	if couponCode != "" {
		if err := h.recordCouponUsage(r.Context(), order.ID, couponCode, subtotalIDR, discountIDR); err != nil {
			h.logger.Warn("create order: failed to record coupon usage", slog.String("order_id", order.ID), slog.String("coupon_code", couponCode), slog.String("error", err.Error()))
		}
	}

	// Try Snap first (snap token does not conflict with Core API)
	snapToken, snapRedirectURL, snapErr := h.paymentSvc.CreateSnapPayment(r.Context(), order)
	if snapErr != nil {
		h.logger.Warn("create snap token failed", slog.String("order_id", orderID), slog.String("error", snapErr.Error()))
	} else if snapRedirectURL != "" {
		if err := h.paymentSvc.SaveOrderSnap(r.Context(), order.ID, snapToken, snapRedirectURL); err != nil {
			h.logger.Warn("save snap checkout failed", slog.String("order_id", orderID), slog.String("error", err.Error()))
		}
	}

	qrString, qrisURL, expiryTime, err := h.paymentSvc.CreateQRIS(r.Context(), order)
	if err != nil {
		h.logger.Error("create qris failed", slog.String("order_id", orderID), slog.String("error", err.Error()))
		if snapToken == "" {
			// Neither Snap nor QRIS available — cannot proceed
			apperrors.WriteError(w, http.StatusInternalServerError, apperrors.ErrInternal, middleware.GetRequestID(r.Context()))
			return
		}
		// Snap succeeded but QRIS failed; continue with Snap only
		expiryTime = time.Now().Add(30 * time.Minute).Format(time.RFC3339)
	}

	orderCopy := *order
	productCopy := *product
	paymentLink := qrisURL
	if paymentLink == "" {
		paymentLink = snapRedirectURL
	}
	if paymentLink == "" {
		paymentLink = "https://sagameda.com/status?id=" + orderNumber
	}
	if qrString != "" {
		paymentLink = "https://sagameda.com/status?id=" + orderNumber
	}
	go func() {
		defer func() {
			if r := recover(); r != nil {
				h.logger.Error("order notification panicked", slog.String("order_id", orderCopy.ID), slog.Any("panic", r))
			}
		}()
		ctx, cancel := context.WithTimeout(h.rootCtx, 10*time.Second)
		defer cancel()
		if err := h.notifySvc.SendOrderConfirmation(ctx, &orderCopy, &productCopy, orderCopy.UserPhone, paymentLink); err != nil {
			h.logger.Error("failed to send notification", slog.String("order_id", orderCopy.ID), slog.String("error", err.Error()))
		}
	}()

	apperrors.WriteSuccess(w, http.StatusCreated, map[string]any{
		"order_id":          orderID,
		"order_number":      orderNumber,
		"amount_idr":        order.AmountIDR,
		"subtotal_idr":      subtotalIDR,
		"discount_idr":      discountIDR,
		"coupon_code":       couponCode,
		"qr_string":         qrString,
		"qris_url":          qrisURL,
		"snap_token":        snapToken,
		"snap_redirect_url": snapRedirectURL,
		"expiry_time":       expiryTime,
		"validation":        validation,
	}, middleware.GetRequestID(r.Context()))
}

func (h *OrderHandler) GetOrder(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	order, err := h.paymentSvc.GetOrder(r.Context(), id)
	if err != nil {
		apperrors.WriteError(w, http.StatusNotFound, apperrors.ErrNotFound, middleware.GetRequestID(r.Context()))
		return
	}
	h.expirePendingOrderIfNeeded(r.Context(), order)

	response := map[string]any{
		"id":                 order.ID,
		"order_number":       order.OrderNumber,
		"product_id":         order.ProductID,
		"user_phone":         order.UserPhone,
		"game_uid":           order.GameUID,
		"game_server":        order.GameServer,
		"amount_idr":         order.AmountIDR,
		"status":             order.Status,
		"midtrans_order_id":  order.MidtransOrderID,
		"channel":            order.Channel,
		"serial_number":      order.SerialNumber,
		"digiflazz_ref_id":   order.DigiflazzRefID,
		"stock_reserved":     order.StockReserved,
		"created_at":         order.CreatedAt,
		"updated_at":         order.UpdatedAt,
		"payment_expires_at": nil,
		"snap_token":         "",
		"snap_redirect_url":  "",
	}
	if order.Status == constants.StatusPending {
		response["payment_expires_at"] = order.CreatedAt.Add(30 * time.Minute)
		if snapData, err := h.paymentSvc.GetOrderSnap(r.Context(), order.ID); err == nil && snapData != nil {
			response["snap_token"] = snapData.SnapToken
			response["snap_redirect_url"] = snapData.SnapRedirectURL
		}
	}
	apperrors.WriteSuccess(w, http.StatusOK, response, middleware.GetRequestID(r.Context()))
}

func (h *OrderHandler) ListOrders(w http.ResponseWriter, r *http.Request) {
	orders, _, err := h.paymentSvc.ListOrders(r.Context(), 1, 50)
	if err != nil {
		apperrors.WriteError(w, http.StatusInternalServerError, apperrors.ErrInternal, middleware.GetRequestID(r.Context()))
		return
	}
	apperrors.WriteSuccess(w, http.StatusOK, orders, middleware.GetRequestID(r.Context()))
}

func (h *OrderHandler) calculateCouponDiscount(ctx context.Context, code, game string, subtotal int) (int, string, error) {
	code = strings.ToUpper(strings.TrimSpace(code))
	if code == "" {
		return 0, "", nil
	}
	if h.pool == nil {
		return 0, "", nil
	}

	var discountType, couponGame string
	var discountValue, minOrder, maxDiscount, maxUses, usedCount int
	err := h.pool.QueryRow(ctx, `
		SELECT discount_type, discount_value, min_order_idr, max_discount_idr, max_uses, used_count, game
		FROM coupons
		WHERE code = $1 AND is_active = true
		  AND (starts_at IS NULL OR starts_at <= NOW())
		  AND (expires_at IS NULL OR expires_at >= NOW())
	`, code).Scan(&discountType, &discountValue, &minOrder, &maxDiscount, &maxUses, &usedCount, &couponGame)
	if err != nil {
		if err == pgx.ErrNoRows {
			return h.calculateReferralDiscount(ctx, code, subtotal)
		}
		return 0, "", fmt.Errorf("coupon check failed")
	}
	if couponGame != "" && couponGame != game {
		return 0, "", fmt.Errorf("coupon is not valid for this game")
	}
	if subtotal < minOrder {
		return 0, "", fmt.Errorf("minimum order for coupon is Rp %s", formatIDR(minOrder))
	}
	if maxUses > 0 && usedCount >= maxUses {
		return 0, "", fmt.Errorf("coupon usage limit reached")
	}

	discount := discountValue
	if discountType == "percent" {
		discount = subtotal * discountValue / 100
		if maxDiscount > 0 && discount > maxDiscount {
			discount = maxDiscount
		}
	}
	if discount <= 0 {
		return 0, "", fmt.Errorf("coupon discount is invalid")
	}
	if discount >= subtotal {
		discount = subtotal - 1000
	}
	return discount, code, nil
}

func (h *OrderHandler) calculateReferralDiscount(ctx context.Context, code string, subtotal int) (int, string, error) {
	if h.referralCodeRepo == nil {
		return 0, "", fmt.Errorf("coupon not found or inactive")
	}
	referral, err := h.referralCodeRepo.GetByCode(ctx, code)
	if err != nil {
		return 0, "", fmt.Errorf("referral code check failed")
	}
	if referral == nil {
		return 0, "", fmt.Errorf("coupon not found or inactive")
	}
	if referral.MaxUses > 0 && referral.UsedCount >= referral.MaxUses {
		return 0, "", fmt.Errorf("referral code usage limit reached")
	}
	if subtotal < referral.MinOrderIDR {
		return 0, "", fmt.Errorf("minimum order for referral code is Rp %s", formatIDR(referral.MinOrderIDR))
	}
	discount := referral.DiscountIDR
	if discount <= 0 {
		return 0, "", fmt.Errorf("referral code discount is invalid")
	}
	if discount >= subtotal {
		discount = subtotal - 1000
	}
	return discount, code, nil
}

func (h *OrderHandler) recordCouponUsage(ctx context.Context, orderID, code string, subtotal, discount int) error {
	if h.pool == nil || code == "" {
		return nil
	}
	_, err := h.pool.Exec(ctx, `
		UPDATE orders SET subtotal_idr = $1, discount_idr = $2, coupon_code = $3 WHERE id = $4
	`, subtotal, discount, code, orderID)
	if err != nil {
		return err
	}
	if h.referralCodeRepo != nil {
		referral, err := h.referralCodeRepo.GetByCode(ctx, code)
		if err != nil {
			return err
		}
		if referral != nil {
			if err := h.referralCodeRepo.ApplyToOrder(ctx, orderID, referral.ID, discount); err != nil {
				return err
			}
			return h.referralCodeRepo.IncrementUsage(ctx, referral.ID)
		}
	}
	_, err = h.pool.Exec(ctx, `UPDATE coupons SET used_count = used_count + 1, updated_at = NOW() WHERE code = $1`, code)
	return err
}

func formatIDR(amount int) string {
	return strconv.FormatInt(int64(amount), 10)
}

func (h *OrderHandler) LookupOrder(w http.ResponseWriter, r *http.Request) {
	var req struct {
		GameUID string `json:"game_uid"`
		Phone   string `json:"phone"`
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		drainBody(r)
		apperrors.WriteError(w, http.StatusBadRequest, apperrors.ErrInvalidInput, middleware.GetRequestID(r.Context()))
		return
	}

	if req.GameUID == "" || req.Phone == "" {
		drainBody(r)
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

	if err := h.paymentSvc.CancelTransaction(order.ID); err != nil {
		h.logger.Warn("cancel order: midtrans cancel failed (may be already expired)", slog.String("order_id", order.ID), slog.String("error", err.Error()))
	}

	if err := h.paymentSvc.UpdateOrderStatus(r.Context(), order.ID, constants.StatusCancelled); err != nil {
		h.logger.Error("cancel order: update status failed", slog.String("order_id", order.ID), slog.String("error", err.Error()))
		apperrors.WriteError(w, http.StatusInternalServerError, apperrors.ErrInternal, middleware.GetRequestID(r.Context()))
		return
	}

	if order.StockReserved && h.topupSvc != nil {
		if err := h.topupSvc.IncrementProductStock(r.Context(), order.ProductID); err != nil {
			h.logger.Warn("cancel order: failed to restore stock", slog.String("order_id", id), slog.String("product_id", order.ProductID), slog.String("error", err.Error()))
		}
	}

	h.paymentSvc.RecordStatusChange(r.Context(), order.ID, order.Status, constants.StatusCancelled, "user cancelled")

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

func (h *OrderHandler) expirePendingOrderIfNeeded(ctx context.Context, order *models.Order) {
	if order == nil || order.Status != constants.StatusPending || order.CreatedAt.IsZero() {
		return
	}
	if time.Since(order.CreatedAt) < 30*time.Minute {
		return
	}

	updated, err := h.paymentSvc.UpdateOrderStatusIf(ctx, order.ID, constants.StatusExpired, constants.StatusPending)
	if err != nil {
		h.logger.Warn("get order: failed to expire stale pending order", slog.String("order_id", order.ID), slog.String("error", err.Error()))
		return
	}
	if !updated {
		return
	}

	if order.StockReserved {
		if err := h.topupSvc.IncrementProductStock(ctx, order.ProductID); err != nil {
			h.logger.Warn("get order: failed to restore stock for expired order", slog.String("order_id", order.ID), slog.String("product_id", order.ProductID), slog.String("error", err.Error()))
		}
	}
	if err := h.paymentSvc.CancelTransaction(order.ID); err != nil {
		h.logger.Warn("get order: midtrans cancel failed for expired order", slog.String("order_id", order.ID), slog.String("error", err.Error()))
	}
	h.paymentSvc.RecordStatusChange(ctx, order.ID, constants.StatusPending, constants.StatusExpired, "expired during status check")
	order.Status = constants.StatusExpired
}

var (
	reNumeric     = regexp.MustCompile(`^\d{4,20}$`)
	reNumericPair = regexp.MustCompile(`^\d+\|\d+$`)
	reServerCode  = regexp.MustCompile(`^[A-Za-z0-9_-]{2,32}$`)
	rePhone       = regexp.MustCompile(`^(0|62)\d{8,13}$`)
)

func validateOrderInput(req struct {
	Game       string `json:"game"`
	GameUID    string `json:"game_uid"`
	GameServer string `json:"game_server"`
	ProductID  string `json:"product_id"`
	ItemQty    int    `json:"item_qty"`
	Phone      string `json:"phone"`
}) error {
	if !constants.ValidGames[req.Game] {
		return &validationError{field: "game", message: "unsupported game"}
	}
	if req.GameUID == "" {
		return &validationError{field: "game_uid", message: "is required"}
	}
	if constants.ServerRequiredGames[req.Game] {
		if req.GameServer == "" {
			return &validationError{field: "game_server", message: "is required for this game"}
		}
		if req.Game == constants.GameMobileLegends {
			combined := req.GameUID + "|" + req.GameServer
			if !reNumericPair.MatchString(combined) {
				return &validationError{field: "game_uid", message: "must be numeric|numeric format (e.g. 12345|1234)"}
			}
		} else {
			if !reNumeric.MatchString(req.GameUID) {
				return &validationError{field: "game_uid", message: "must be numeric"}
			}
			if !reServerCode.MatchString(req.GameServer) {
				return &validationError{field: "game_server", message: "must be 2-32 letters, numbers, underscore, or dash"}
			}
		}
	} else if req.Game == constants.GamePulsa || req.Game == constants.GameGopay {
		if !rePhone.MatchString(req.GameUID) {
			return &validationError{field: "game_uid", message: "must be valid Indonesian phone (08xxx or 628xxx, 10-15 digits)"}
		}
	} else {
		if !reNumeric.MatchString(req.GameUID) {
			return &validationError{field: "game_uid", message: "must be numeric"}
		}
	}
	if !rePhone.MatchString(req.Phone) {
		return &validationError{field: "phone", message: "must be valid Indonesian number (08xxx or 628xxx, 10-15 digits)"}
	}
	if req.ProductID == "" && req.ItemQty <= 0 {
		return &validationError{field: "product_id", message: "either product_id or item_qty is required"}
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

func drainBody(r *http.Request) {
	io.Copy(io.Discard, r.Body)
	r.Body.Close()
}
