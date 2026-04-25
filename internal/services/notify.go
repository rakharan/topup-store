package services

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"time"

	"github.com/topup-store/internal/constants"
	"github.com/topup-store/internal/models"
	"github.com/topup-store/internal/retry"
)

type NotifyService struct {
	whatsappNum   string
	waToken       string
	waPhoneID     string
	waBotBaseURL  string
	httpClient    *http.Client
	logger        *slog.Logger
}

func NewNotifyService(whatsappNum, waToken, waPhoneID, waBotBaseURL string, logger *slog.Logger) *NotifyService {
	return &NotifyService{
		whatsappNum:  whatsappNum,
		waToken:      waToken,
		waPhoneID:    waPhoneID,
		waBotBaseURL: waBotBaseURL,
		httpClient:   &http.Client{Timeout: 10 * time.Second},
		logger:       logger,
	}
}

func (s *NotifyService) SendOrderConfirmation(ctx context.Context, order *models.Order, phone string) error {
	message := fmt.Sprintf(
		"Halo! Order kamu telah dibuat.\nID Order: %s\nTotal: Rp %d\nSilakan scan QRIS untuk membayar.",
		order.ID, order.AmountIDR,
	)
	return s.sendNotification(ctx, phone, message)
}

func (s *NotifyService) SendTopupSuccess(ctx context.Context, order *models.Order, phone string) error {
	message := fmt.Sprintf(
		"Top-up berhasil!\nID Order: %s\nGame UID: %s\nTerima kasih telah berbelanja di TopUp Store.",
		order.ID, order.GameUID,
	)
	return s.sendNotification(ctx, phone, message)
}

func (s *NotifyService) SendTopupFailure(ctx context.Context, order *models.Order, phone string) error {
	message := fmt.Sprintf(
		"Top-up gagal!\nID Order: %s\nGame UID: %s\nSilakan hubungi admin untuk bantuan.",
		order.ID, order.GameUID,
	)
	return s.sendNotification(ctx, phone, message)
}

func (s *NotifyService) SendNotification(phone, message string) error {
	return s.sendNotification(context.Background(), phone, message)
}

func (s *NotifyService) sendNotification(ctx context.Context, phone, message string) error {
	if s.waToken != "" && s.waPhoneID != "" {
		err := s.sendViaCloudAPI(ctx, phone, message)
		if err == nil {
			return nil
		}
		s.logger.Warn("cloud api failed, falling back to bot", "error", err)
	}
	return s.sendViaBot(ctx, phone, message)
}

func (s *NotifyService) sendViaCloudAPI(ctx context.Context, phone, message string) error {
	payload := map[string]any{
		"messaging_product": "whatsapp",
		"to":                phone,
		"type":              "text",
		"text":              map[string]string{"body": message},
	}

	body, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("marshal payload: %w", err)
	}

	retryCfg := retry.DefaultConfig()
	return retry.Do(ctx, retryCfg, func(ctx context.Context) error {
		req, err := http.NewRequestWithContext(ctx,
			http.MethodPost,
			fmt.Sprintf("%s/%s/messages", constants.WhatsAppCloudAPIURL, s.waPhoneID),
			bytes.NewReader(body),
		)
		if err != nil {
			return fmt.Errorf("create request: %w", err)
		}
		req.Header.Set("Authorization", "Bearer "+s.waToken)
		req.Header.Set("Content-Type", "application/json")

		resp, err := s.httpClient.Do(req)
		if err != nil {
			return fmt.Errorf("send notification: %w", err)
		}
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusOK {
			return fmt.Errorf("cloud api returned status %d", resp.StatusCode)
		}

		return nil
	})
}

func (s *NotifyService) sendViaBot(ctx context.Context, phone, message string) error {
	payload := map[string]string{
		"phone":   phone,
		"message": message,
	}

	body, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("marshal payload: %w", err)
	}

	retryCfg := retry.DefaultConfig()
	return retry.Do(ctx, retryCfg, func(ctx context.Context) error {
		req, err := http.NewRequestWithContext(ctx, http.MethodPost, s.waBotBaseURL+"/notify", bytes.NewReader(body))
		if err != nil {
			return fmt.Errorf("create request: %w", err)
		}
		req.Header.Set("Content-Type", "application/json")

		resp, err := s.httpClient.Do(req)
		if err != nil {
			return fmt.Errorf("send notification: %w", err)
		}
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusOK {
			return fmt.Errorf("notification service returned status %d", resp.StatusCode)
		}

		return nil
	})
}
