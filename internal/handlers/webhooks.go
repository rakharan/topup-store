package handlers

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha1"
	"crypto/sha512"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"

	"github.com/topup-store/internal/apperrors"
	"github.com/topup-store/internal/constants"
	"github.com/topup-store/internal/middleware"
	"github.com/topup-store/internal/models"
	"github.com/topup-store/internal/repositories"
	"github.com/topup-store/internal/services"
)

type WebhookHandler struct {
	paymentSvc             services.PaymentServiceInterface
	topupSvc               services.TopupServiceInterface
	notifySvc              services.NotifyServiceInterface
	webhookRepo            repositories.WebhookRepository
	midtransKey            string
	digiflazzUser          string
	digiflazzAPIKey        string
	digiflazzWebhookSecret string
	rootCtx                context.Context
	logger                 *slog.Logger
}

func NewWebhookHandler(paymentSvc services.PaymentServiceInterface, topupSvc services.TopupServiceInterface, notifySvc services.NotifyServiceInterface, webhookRepo repositories.WebhookRepository, midtransKey, digiflazzUser, digiflazzAPIKey, digiflazzWebhookSecret string, rootCtx context.Context, logger *slog.Logger) *WebhookHandler {
	return &WebhookHandler{
		paymentSvc:             paymentSvc,
		topupSvc:               topupSvc,
		notifySvc:              notifySvc,
		webhookRepo:            webhookRepo,
		midtransKey:            midtransKey,
		digiflazzUser:          digiflazzUser,
		digiflazzAPIKey:        digiflazzAPIKey,
		digiflazzWebhookSecret: digiflazzWebhookSecret,
		rootCtx:                rootCtx,
		logger:                 logger,
	}
}

func (h *WebhookHandler) Midtrans(w http.ResponseWriter, r *http.Request) {
	rawBody, _ := io.ReadAll(r.Body)
	r.Body = io.NopCloser(bytes.NewReader(rawBody))

	var payload struct {
		OrderID           string `json:"order_id"`
		StatusCode        string `json:"status_code"`
		GrossAmount       string `json:"gross_amount"`
		SignatureKey      string `json:"signature_key"`
		TransactionStatus string `json:"transaction_status"`
		FraudStatus       string `json:"fraud_status"`
	}

	if err := json.Unmarshal(rawBody, &payload); err != nil {
		h.logWebhook(r.Context(), "midtrans", payload.OrderID, string(rawBody), r.Header.Get("X-Signature"), r.UserAgent(), "failed", "invalid json")
		apperrors.WriteError(w, http.StatusBadRequest, apperrors.FieldError("payload", "invalid payload"), middleware.GetRequestID(r.Context()))
		return
	}

	if !h.verifySignature(payload) {
		h.logWebhook(r.Context(), "midtrans", payload.OrderID, string(rawBody), payload.SignatureKey, r.UserAgent(), "failed", "invalid signature")
		h.logger.Warn("midtrans webhook: invalid signature", slog.String("order_id", payload.OrderID))
		apperrors.WriteError(w, http.StatusUnauthorized, apperrors.FieldError("signature", "invalid signature"), middleware.GetRequestID(r.Context()))
		return
	}

	order, err := h.paymentSvc.GetOrder(r.Context(), payload.OrderID)
	if err != nil {
		h.logWebhook(r.Context(), "midtrans", payload.OrderID, string(rawBody), payload.SignatureKey, r.UserAgent(), "skipped", "order not found")
		h.logger.Warn("midtrans webhook: order not found", slog.String("order_id", payload.OrderID))
		w.WriteHeader(http.StatusOK)
		return
	}

	newStatus := h.mapTransactionStatus(payload.TransactionStatus, payload.FraudStatus)
	if newStatus == "" {
		h.logWebhook(r.Context(), "midtrans", payload.OrderID, string(rawBody), payload.SignatureKey, r.UserAgent(), "skipped", "unmapped status")
		h.logger.Warn("midtrans webhook: unmapped status", slog.String("status", payload.TransactionStatus), slog.String("order_id", payload.OrderID))
		w.WriteHeader(http.StatusOK)
		return
	}

	if order.Status == newStatus {
		h.logWebhook(r.Context(), "midtrans", payload.OrderID, string(rawBody), payload.SignatureKey, r.UserAgent(), "skipped", "already in status")
		h.logger.Info("midtrans webhook: order already in status", slog.String("order_id", order.ID), slog.String("status", newStatus))
		w.WriteHeader(http.StatusOK)
		return
	}

	oldStatus := order.Status
	if err := h.paymentSvc.UpdateOrderStatus(r.Context(), order.ID, newStatus); err != nil {
		h.logWebhook(r.Context(), "midtrans", payload.OrderID, string(rawBody), payload.SignatureKey, r.UserAgent(), "failed", err.Error())
		h.logger.Error("midtrans webhook: failed to update order", slog.String("order_id", order.ID), slog.String("error", err.Error()))
		w.WriteHeader(http.StatusOK)
		return
	}

	h.paymentSvc.RecordStatusChange(r.Context(), order.ID, oldStatus, newStatus, "midtrans webhook")
	h.logWebhook(r.Context(), "midtrans", payload.OrderID, string(rawBody), payload.SignatureKey, r.UserAgent(), "processed", "")

	if newStatus == constants.StatusPaid {
		orderCopy := *order
		go func() {
			defer func() {
				if r := recover(); r != nil {
					h.logger.Error("topup process panicked", slog.String("order_id", orderCopy.ID), slog.Any("panic", r))
				}
			}()
			if err := h.topupSvc.ProcessOrder(orderCopy.ID); err != nil {
				h.logger.Error("topup process failed", slog.String("order_id", orderCopy.ID), slog.String("error", err.Error()))
				return
			}
			h.logger.Info("topup processed successfully", slog.String("order_id", orderCopy.ID))
		}()
	}

	w.WriteHeader(http.StatusOK)
}

func (h *WebhookHandler) verifySignature(payload struct {
	OrderID           string `json:"order_id"`
	StatusCode        string `json:"status_code"`
	GrossAmount       string `json:"gross_amount"`
	SignatureKey      string `json:"signature_key"`
	TransactionStatus string `json:"transaction_status"`
	FraudStatus       string `json:"fraud_status"`
}) bool {
	hashString := fmt.Sprintf("%s%s%s%s", payload.OrderID, payload.StatusCode, payload.GrossAmount, h.midtransKey)
	hash := sha512.New()
	hash.Write([]byte(hashString))
	expectedSig := hex.EncodeToString(hash.Sum(nil))
	return payload.SignatureKey == expectedSig
}

func (h *WebhookHandler) mapTransactionStatus(txStatus, fraudStatus string) string {
	switch txStatus {
	case "capture":
		if fraudStatus == "accept" {
			return constants.StatusPaid
		}
	case "settlement":
		return constants.StatusPaid
	case "pending":
		return constants.StatusPending
	case "deny", "expire", "cancel":
		return constants.StatusFailed
	}
	return ""
}

func (h *WebhookHandler) Digiflazz(w http.ResponseWriter, r *http.Request) {
	rawBody, err := io.ReadAll(r.Body)
	if err != nil {
		h.logger.Error("digiflazz webhook: failed to read body", slog.String("error", err.Error()))
		w.WriteHeader(http.StatusOK)
		return
	}
	r.Body = io.NopCloser(bytes.NewReader(rawBody))

	h.logger.Info("digiflazz webhook: received",
		slog.String("x_hub_signature", r.Header.Get("X-Hub-Signature")),
		slog.String("user_agent", r.Header.Get("User-Agent")),
		slog.String("x_digiflazz_event", r.Header.Get("X-Digiflazz-Event")),
	)

	var generic map[string]any
	if err := json.Unmarshal(rawBody, &generic); err != nil {
		h.logWebhook(r.Context(), "digiflazz", "", string(rawBody), r.Header.Get("X-Hub-Signature"), r.UserAgent(), "failed", "invalid json")
		h.logger.Warn("digiflazz webhook: invalid JSON", slog.String("error", err.Error()))
		w.WriteHeader(http.StatusOK)
		return
	}

	if _, hasPing := generic["sed"]; hasPing {
		h.logger.Info("digiflazz webhook: ping event received")
		w.WriteHeader(http.StatusOK)
		return
	}

	sigHeader := r.Header.Get("X-Hub-Signature")
	if h.digiflazzWebhookSecret != "" {
		if !h.verifyDigiflazzSignature(rawBody, sigHeader) {
			refID, _ := generic["data"].(map[string]any)["ref_id"].(string)
			h.logWebhook(r.Context(), "digiflazz", refID, string(rawBody), sigHeader, r.UserAgent(), "failed", "invalid signature")
			h.logger.Warn("digiflazz webhook: invalid signature")
			apperrors.WriteError(w, http.StatusUnauthorized, apperrors.FieldError("signature", "invalid signature"), middleware.GetRequestID(r.Context()))
			return
		}
	} else {
		h.logger.Warn("digiflazz webhook: signature verification disabled (no secret configured)")
	}

	data, ok := generic["data"].(map[string]any)
	if !ok {
		h.logWebhook(r.Context(), "digiflazz", "", string(rawBody), sigHeader, r.UserAgent(), "failed", "no data field")
		h.logger.Warn("digiflazz webhook: no data field in payload")
		w.WriteHeader(http.StatusOK)
		return
	}

	refID, _ := data["ref_id"].(string)
	status, _ := data["status"].(string)
	sn, _ := data["sn"].(string)

	if refID == "" {
		h.logWebhook(r.Context(), "digiflazz", "", string(rawBody), sigHeader, r.UserAgent(), "skipped", "missing ref_id")
		h.logger.Warn("digiflazz webhook: missing ref_id")
		w.WriteHeader(http.StatusOK)
		return
	}

	order, err := h.paymentSvc.GetOrder(r.Context(), refID)
	if err != nil {
		h.logWebhook(r.Context(), "digiflazz", refID, string(rawBody), sigHeader, r.UserAgent(), "skipped", "order not found")
		h.logger.Warn("digiflazz webhook: order not found, skipping",
			slog.String("ref_id", refID),
		)
		w.WriteHeader(http.StatusOK)
		return
	}

	if status == "Sukses" {
		if order.Status == constants.StatusSuccess {
			h.logWebhook(r.Context(), "digiflazz", refID, string(rawBody), sigHeader, r.UserAgent(), "skipped", "already success")
			h.logger.Info("digiflazz webhook: order already success", slog.String("order_id", order.ID))
			w.WriteHeader(http.StatusOK)
			return
		}

		oldStatus := order.Status
		if err := h.paymentSvc.UpdateOrderStatus(r.Context(), order.ID, constants.StatusSuccess); err != nil {
			h.logWebhook(r.Context(), "digiflazz", refID, string(rawBody), sigHeader, r.UserAgent(), "failed", err.Error())
			h.logger.Error("digiflazz webhook: failed to update order",
				slog.String("order_id", order.ID),
				slog.String("error", err.Error()),
			)
			w.WriteHeader(http.StatusOK)
			return
		}

		h.paymentSvc.RecordStatusChange(r.Context(), order.ID, oldStatus, constants.StatusSuccess, "digiflazz webhook")

		if sn != "" {
			if err := h.paymentSvc.UpdateOrderSerialNumber(r.Context(), order.ID, sn); err != nil {
				h.logger.Warn("digiflazz webhook: failed to save serial number",
					slog.String("order_id", order.ID),
					slog.String("error", err.Error()),
				)
			}
		}

		h.logWebhook(r.Context(), "digiflazz", refID, string(rawBody), sigHeader, r.UserAgent(), "processed", "")

		orderCopy := *order
		snCopy := sn
		go func() {
			defer func() {
				if r := recover(); r != nil {
					h.logger.Error("digiflazz notify panicked", slog.String("order_id", orderCopy.ID), slog.Any("panic", r))
				}
			}()
			product, err := h.topupSvc.GetProduct(h.rootCtx, orderCopy.ProductID)
			if err != nil {
				h.logger.Error("digiflazz webhook: failed to get product for notification", slog.String("order_id", orderCopy.ID), slog.String("error", err.Error()))
				return
			}
			if err := h.notifySvc.SendTopupSuccess(h.rootCtx, &orderCopy, product, orderCopy.UserPhone, snCopy); err != nil {
				h.logger.Error("digiflazz webhook: failed to send notification",
					slog.String("order_id", orderCopy.ID),
					slog.String("error", err.Error()),
				)
			}
		}()

		h.logger.Info("digiflazz webhook: order marked as success",
			slog.String("order_id", order.ID),
			slog.String("sn", sn),
		)
		w.WriteHeader(http.StatusOK)
		return
	}

	if status == "Gagal" || status == "Fail" {
		if order.Status == constants.StatusFailed {
			h.logWebhook(r.Context(), "digiflazz", refID, string(rawBody), sigHeader, r.UserAgent(), "skipped", "already failed")
			h.logger.Info("digiflazz webhook: order already failed", slog.String("order_id", order.ID))
			w.WriteHeader(http.StatusOK)
			return
		}

		oldStatus := order.Status
		if err := h.paymentSvc.UpdateOrderStatus(r.Context(), order.ID, constants.StatusFailed); err != nil {
			h.logWebhook(r.Context(), "digiflazz", refID, string(rawBody), sigHeader, r.UserAgent(), "failed", err.Error())
			h.logger.Error("digiflazz webhook: failed to update order",
				slog.String("order_id", order.ID),
				slog.String("error", err.Error()),
			)
			w.WriteHeader(http.StatusOK)
			return
		}

		h.paymentSvc.RecordStatusChange(r.Context(), order.ID, oldStatus, constants.StatusFailed, "digiflazz webhook")
		h.logWebhook(r.Context(), "digiflazz", refID, string(rawBody), sigHeader, r.UserAgent(), "processed", "")

		orderCopy := *order
		go func() {
			defer func() {
				if r := recover(); r != nil {
					h.logger.Error("digiflazz failure notify panicked", slog.String("order_id", orderCopy.ID), slog.Any("panic", r))
				}
			}()
			product, err := h.topupSvc.GetProduct(h.rootCtx, orderCopy.ProductID)
			if err != nil {
				h.logger.Error("digiflazz webhook: failed to get product for failure notification", slog.String("order_id", orderCopy.ID), slog.String("error", err.Error()))
				return
			}
			if err := h.notifySvc.SendTopupFailure(h.rootCtx, &orderCopy, product, orderCopy.UserPhone); err != nil {
				h.logger.Error("digiflazz webhook: failed to send failure notification",
					slog.String("order_id", orderCopy.ID),
					slog.String("error", err.Error()),
				)
			}
		}()

		h.logger.Info("digiflazz webhook: order marked as failed",
			slog.String("order_id", order.ID),
			slog.String("status", status),
		)
		w.WriteHeader(http.StatusOK)
		return
	}

	h.logWebhook(r.Context(), "digiflazz", refID, string(rawBody), sigHeader, r.UserAgent(), "skipped", "non-success status")
	h.logger.Info("digiflazz webhook: non-success status",
		slog.String("ref_id", refID),
		slog.String("status", status),
	)
	w.WriteHeader(http.StatusOK)
}

func (h *WebhookHandler) verifyDigiflazzSignature(rawBody []byte, headerSig string) bool {
	if headerSig == "" {
		return false
	}

	expectedMAC := hmac.New(sha1.New, []byte(h.digiflazzWebhookSecret))
	expectedMAC.Write(rawBody)
	expectedSig := "sha1=" + hex.EncodeToString(expectedMAC.Sum(nil))

	return hmac.Equal([]byte(headerSig), []byte(expectedSig))
}

func (h *WebhookHandler) logWebhook(ctx context.Context, source, refID, payload, signature, userAgent, status, errMsg string) {
	log := &models.WebhookLog{
		Source:    source,
		RefID:     nilIfEmpty(refID),
		Payload:   payload,
		Signature: nilIfEmpty(signature),
		UserAgent: nilIfEmpty(userAgent),
		Status:    status,
		Error:     nilIfEmpty(errMsg),
	}
	if err := h.webhookRepo.Log(ctx, log); err != nil {
		h.logger.Error("webhook: failed to log", slog.String("source", source), slog.String("error", err.Error()))
	}
}

func nilIfEmpty(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}
