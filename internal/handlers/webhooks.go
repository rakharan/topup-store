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
	"github.com/topup-store/internal/services"
)

type WebhookHandler struct {
	paymentSvc             services.PaymentServiceInterface
	topupSvc               services.TopupServiceInterface
	notifySvc              services.NotifyServiceInterface
	midtransKey            string
	digiflazzUser          string
	digiflazzAPIKey        string
	digiflazzWebhookSecret string
	logger                 *slog.Logger
}

func NewWebhookHandler(paymentSvc services.PaymentServiceInterface, topupSvc services.TopupServiceInterface, notifySvc services.NotifyServiceInterface, midtransKey, digiflazzUser, digiflazzAPIKey, digiflazzWebhookSecret string, logger *slog.Logger) *WebhookHandler {
	return &WebhookHandler{
		paymentSvc:             paymentSvc,
		topupSvc:               topupSvc,
		notifySvc:              notifySvc,
		midtransKey:            midtransKey,
		digiflazzUser:          digiflazzUser,
		digiflazzAPIKey:        digiflazzAPIKey,
		digiflazzWebhookSecret: digiflazzWebhookSecret,
		logger:                 logger,
	}
}

func (h *WebhookHandler) Midtrans(w http.ResponseWriter, r *http.Request) {
	var payload struct {
		OrderID             string `json:"order_id"`
		StatusCode          string `json:"status_code"`
		GrossAmount         string `json:"gross_amount"`
		SignatureKey        string `json:"signature_key"`
		TransactionStatus   string `json:"transaction_status"`
		FraudStatus         string `json:"fraud_status"`
	}

	if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
		apperrors.WriteError(w, http.StatusBadRequest, apperrors.FieldError("payload", "invalid payload"), middleware.GetRequestID(r.Context()))
		return
	}

	if !h.verifySignature(payload) {
		h.logger.Warn("midtrans webhook: invalid signature", slog.String("order_id", payload.OrderID))
		apperrors.WriteError(w, http.StatusUnauthorized, apperrors.FieldError("signature", "invalid signature"), middleware.GetRequestID(r.Context()))
		return
	}

	order, err := h.paymentSvc.GetOrder(r.Context(), payload.OrderID)
	if err != nil {
		h.logger.Error("midtrans webhook: order not found", slog.String("order_id", payload.OrderID), slog.String("error", err.Error()))
		apperrors.WriteError(w, http.StatusNotFound, apperrors.ErrNotFound, middleware.GetRequestID(r.Context()))
		return
	}

	newStatus := h.mapTransactionStatus(payload.TransactionStatus, payload.FraudStatus)
	if newStatus == "" {
		h.logger.Warn("midtrans webhook: unmapped status", slog.String("status", payload.TransactionStatus), slog.String("order_id", payload.OrderID))
		w.WriteHeader(http.StatusOK)
		return
	}

	if order.Status == newStatus {
		h.logger.Info("midtrans webhook: order already in status", slog.String("order_id", order.ID), slog.String("status", newStatus))
		w.WriteHeader(http.StatusOK)
		return
	}

	if err := h.paymentSvc.UpdateOrderStatus(r.Context(), order.ID, newStatus); err != nil {
		h.logger.Error("midtrans webhook: failed to update order", slog.String("order_id", order.ID), slog.String("error", err.Error()))
		apperrors.WriteError(w, http.StatusInternalServerError, apperrors.ErrInternal, middleware.GetRequestID(r.Context()))
		return
	}

	if newStatus == constants.StatusPaid {
		orderCopy := *order
		go func() {
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
	OrderID             string `json:"order_id"`
	StatusCode          string `json:"status_code"`
	GrossAmount         string `json:"gross_amount"`
	SignatureKey        string `json:"signature_key"`
	TransactionStatus   string `json:"transaction_status"`
	FraudStatus         string `json:"fraud_status"`
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
		slog.String("raw_body", string(rawBody)),
		slog.String("x_hub_signature", r.Header.Get("X-Hub-Signature")),
		slog.String("user_agent", r.Header.Get("User-Agent")),
		slog.String("x_digiflazz_event", r.Header.Get("X-Digiflazz-Event")),
	)

	var generic map[string]any
	if err := json.Unmarshal(rawBody, &generic); err != nil {
		h.logger.Warn("digiflazz webhook: invalid JSON", slog.String("error", err.Error()))
		w.WriteHeader(http.StatusOK)
		return
	}

	if _, hasPing := generic["sed"]; hasPing {
		h.logger.Info("digiflazz webhook: ping event received")
		w.WriteHeader(http.StatusOK)
		return
	}

	if h.digiflazzWebhookSecret != "" {
		if !h.verifyDigiflazzSignature(rawBody, r.Header.Get("X-Hub-Signature")) {
			h.logger.Warn("digiflazz webhook: invalid signature")
			apperrors.WriteError(w, http.StatusUnauthorized, apperrors.FieldError("signature", "invalid signature"), middleware.GetRequestID(r.Context()))
			return
		}
	}

	data, ok := generic["data"].(map[string]any)
	if !ok {
		h.logger.Warn("digiflazz webhook: no data field in payload")
		w.WriteHeader(http.StatusOK)
		return
	}

	refID, _ := data["ref_id"].(string)
	status, _ := data["status"].(string)
	sn, _ := data["sn"].(string)

	if refID == "" {
		h.logger.Warn("digiflazz webhook: missing ref_id")
		w.WriteHeader(http.StatusOK)
		return
	}

	order, err := h.paymentSvc.GetOrder(r.Context(), refID)
	if err != nil {
		h.logger.Error("digiflazz webhook: order not found",
			slog.String("ref_id", refID),
			slog.String("error", err.Error()),
		)
		apperrors.WriteError(w, http.StatusNotFound, apperrors.ErrNotFound, middleware.GetRequestID(r.Context()))
		return
	}

	if status == "Sukses" {
		if order.Status == constants.StatusSuccess {
			h.logger.Info("digiflazz webhook: order already success", slog.String("order_id", order.ID))
			w.WriteHeader(http.StatusOK)
			return
		}

		if err := h.paymentSvc.UpdateOrderStatus(r.Context(), order.ID, constants.StatusSuccess); err != nil {
			h.logger.Error("digiflazz webhook: failed to update order",
				slog.String("order_id", order.ID),
				slog.String("error", err.Error()),
			)
			apperrors.WriteError(w, http.StatusInternalServerError, apperrors.ErrInternal, middleware.GetRequestID(r.Context()))
			return
		}

		if sn != "" {
			if err := h.paymentSvc.UpdateOrderSerialNumber(r.Context(), order.ID, sn); err != nil {
				h.logger.Warn("digiflazz webhook: failed to save serial number",
					slog.String("order_id", order.ID),
					slog.String("error", err.Error()),
				)
			}
		}

		orderCopy := *order
		go func() {
			ctx := context.Background()
			if err := h.notifySvc.SendTopupSuccess(ctx, &orderCopy, orderCopy.UserPhone); err != nil {
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
			h.logger.Info("digiflazz webhook: order already failed", slog.String("order_id", order.ID))
			w.WriteHeader(http.StatusOK)
			return
		}

		if err := h.paymentSvc.UpdateOrderStatus(r.Context(), order.ID, constants.StatusFailed); err != nil {
			h.logger.Error("digiflazz webhook: failed to update order",
				slog.String("order_id", order.ID),
				slog.String("error", err.Error()),
			)
			apperrors.WriteError(w, http.StatusInternalServerError, apperrors.ErrInternal, middleware.GetRequestID(r.Context()))
			return
		}

		orderCopy := *order
		go func() {
			ctx := context.Background()
			if err := h.notifySvc.SendTopupFailure(ctx, &orderCopy, orderCopy.UserPhone); err != nil {
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
