package services

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/topup-store/internal/models"
	"github.com/topup-store/internal/retry"
)

type NotifyService struct {
	fonnteToken  string
	waBotBaseURL string
	waBotToken   string
	httpClient   *http.Client
	logger       *slog.Logger
}

func NewNotifyService(fonnteToken, waBotBaseURL, waBotToken string, logger *slog.Logger) *NotifyService {
	return &NotifyService{
		fonnteToken:  fonnteToken,
		waBotBaseURL: waBotBaseURL,
		waBotToken:   waBotToken,
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
	cleaned := strings.TrimSpace(phone)
	if cleaned == "" {
		return fmt.Errorf("invalid phone number format: %s", phone)
	}
	if strings.HasPrefix(cleaned, "0") {
		cleaned = cleaned[1:]
	}
	cleaned = strings.TrimPrefix(cleaned, "+")

	if s.fonnteToken != "" {
		err := s.sendViaFonnte(ctx, cleaned, message)
		if err == nil {
			return nil
		}
		s.logger.Warn("fonnte api failed, falling back to bot", "error", err)
	}
	return s.sendViaBot(ctx, cleaned, message)
}

func (s *NotifyService) sendViaFonnte(ctx context.Context, phone, message string) error {
	payload := map[string]string{
		"target":      phone,
		"message":     message,
		"countryCode": "62",
	}

	body, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("marshal payload: %w", err)
	}

	retryCfg := retry.DefaultConfig()
	return retry.Do(ctx, retryCfg, func(ctx context.Context) error {
		req, err := http.NewRequestWithContext(ctx,
			http.MethodPost,
			"https://api.fonnte.com/send",
			bytes.NewReader(body),
		)
		if err != nil {
			return fmt.Errorf("create request: %w", err)
		}
		req.Header.Set("Authorization", s.fonnteToken)
		req.Header.Set("Content-Type", "application/json")

		resp, err := s.httpClient.Do(req)
		if err != nil {
			return fmt.Errorf("send notification: %w", err)
		}
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusOK {
			return fmt.Errorf("fonnte api returned status %d", resp.StatusCode)
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
		if s.waBotToken != "" {
			req.Header.Set("X-Bot-Token", s.waBotToken)
		}

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
