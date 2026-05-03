package services

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"regexp"
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

func (s *NotifyService) SendOrderConfirmation(ctx context.Context, order *models.Order, product *models.Product, phone, qrisURL string) error {
	gameLabel := map[string]string{
		"free_fire":      "Free Fire",
		"mobile_legends": "Mobile Legends",
		"pubg_mobile":    "PUBG Mobile",
	}[product.Game]

	uidInfo := order.GameUID
	if product.Game == "mobile_legends" && order.GameServer != "" {
		uidInfo = order.GameUID + " (" + order.GameServer + ")"
	}

	message := fmt.Sprintf(
		"🎮 *Order Dibuat*\n\n"+
			"ID Order: %s\n"+
			"Game: %s\n"+
			"Paket: %s\n"+
			"UID: %s\n"+
			"Total: Rp %d\n\n"+
			"Bayar di: %s\n\n"+
			"⏳ Proses pembayaran akan memakan waktu beberapa menit setelah pembayaran dikonfirmasi.\n"+
			"Cek status order di: https://topup-store.com/status\n\n"+
			"Terima kasih!",
		order.OrderNumber, gameLabel, product.Name, uidInfo, order.AmountIDR, qrisURL,
	)
	return s.sendNotification(ctx, phone, message)
}

func (s *NotifyService) SendTopupSuccess(ctx context.Context, order *models.Order, product *models.Product, phone, serialNumber string) error {
	gameLabel := map[string]string{
		"free_fire":      "Free Fire",
		"mobile_legends": "Mobile Legends",
		"pubg_mobile":    "PUBG Mobile",
	}[product.Game]

	uidInfo := order.GameUID
	if product.Game == "mobile_legends" && order.GameServer != "" {
		uidInfo = order.GameUID + " (" + order.GameServer + ")"
	}

	snLine := ""
	if serialNumber != "" {
		snLine = fmt.Sprintf("\nSN: %s", serialNumber)
	}

	message := fmt.Sprintf(
		"✅ *Pesanan Berhasil!*\n\n"+
			"ID Order: %s\n"+
			"Game: %s\n"+
			"Produk: %s\n"+
			"UID: %s\n"+
			"Total: Rp %d%s\n\n"+
			"Item telah dikirim ke akun kamu. Silakan cek dalam game.\n\n"+
			"Terima kasih telah berbelanja di TopUp Store!",
		order.OrderNumber, gameLabel, product.Name, uidInfo, order.AmountIDR, snLine,
	)
	return s.sendNotification(ctx, phone, message)
}

func (s *NotifyService) SendTopupFailure(ctx context.Context, order *models.Order, product *models.Product, phone string) error {
	gameLabel := map[string]string{
		"free_fire":      "Free Fire",
		"mobile_legends": "Mobile Legends",
		"pubg_mobile":    "PUBG Mobile",
	}[product.Game]

	uidInfo := order.GameUID
	if product.Game == "mobile_legends" && order.GameServer != "" {
		uidInfo = order.GameUID + " (" + order.GameServer + ")"
	}

	message := fmt.Sprintf(
		"❌ *Pesanan Gagal!*\n\n"+
			"ID Order: %s\n"+
			"Game: %s\n"+
			"Produk: %s\n"+
			"UID: %s\n"+
			"Total: Rp %d\n\n"+
			"Top-up gagal diproses. Saldo kamu akan dikembalikan.\n"+
			"Silakan hubungi admin untuk bantuan.",
		order.OrderNumber, gameLabel, product.Name, uidInfo, order.AmountIDR,
	)
	return s.sendNotification(ctx, phone, message)
}

func (s *NotifyService) SendNotification(phone, message string) error {
	return s.sendNotification(context.Background(), phone, message)
}

var phoneRegex = regexp.MustCompile(`^\d{8,15}$`)

func (s *NotifyService) sendNotification(ctx context.Context, phone, message string) error {
	cleaned := strings.TrimSpace(phone)
	if cleaned == "" {
		return fmt.Errorf("invalid phone number format: %s", phone)
	}
	if strings.HasPrefix(cleaned, "0") {
		cleaned = cleaned[1:]
	}
	cleaned = strings.TrimPrefix(cleaned, "+")

	if !phoneRegex.MatchString(cleaned) {
		return fmt.Errorf("invalid phone number format: %s", phone)
	}

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
