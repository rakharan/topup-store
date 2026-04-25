package services

import (
	"bytes"
	"context"
	"crypto/md5"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"time"

	"github.com/topup-store/internal/constants"
	"github.com/topup-store/internal/models"
	"github.com/topup-store/internal/repositories"
	"github.com/topup-store/internal/retry"
)

type TopupService struct {
	orderRepo     repositories.OrderRepository
	productRepo   repositories.ProductRepository
	digiflazzUser   string
	digiflazzAPIKey string
	digiflazzURL    string
	httpClient      *http.Client
	logger          *slog.Logger
}

func NewTopupService(orderRepo repositories.OrderRepository, productRepo repositories.ProductRepository, user, apiKey, digiflazzURL string, logger *slog.Logger) *TopupService {
	return &TopupService{
		orderRepo:     orderRepo,
		productRepo:   productRepo,
		digiflazzUser:   user,
		digiflazzAPIKey: apiKey,
		digiflazzURL:    digiflazzURL,
		httpClient: &http.Client{
			Timeout: 30 * time.Second,
		},
		logger: logger,
	}
}

func (s *TopupService) ProcessOrder(orderID string) error {
	ctx := context.Background()

	order, err := s.orderRepo.GetByID(ctx, orderID)
	if err != nil {
		return fmt.Errorf("fetch order %s: %w", orderID, err)
	}

	if order.Status != constants.StatusPaid {
		return fmt.Errorf("order %s is not paid (status: %s)", orderID, order.Status)
	}

	product, err := s.productRepo.GetByID(ctx, order.ProductID)
	if err != nil {
		return fmt.Errorf("fetch product %s: %w", order.ProductID, err)
	}

	if err := s.processTopupViaDigiflazz(ctx, order, product); err != nil {
		_ = s.orderRepo.UpdateStatus(ctx, orderID, constants.StatusFailed)
		return fmt.Errorf("digiflazz topup: %w", err)
	}

	return s.orderRepo.UpdateStatus(ctx, orderID, constants.StatusProcessing)
}

func (s *TopupService) processTopupViaDigiflazz(ctx context.Context, order *models.Order, product *models.Product) error {
	customerNo := s.buildCustomerNo(order, product)
	sign := s.generateSign(order.ID)

	payload := map[string]string{
		"username":         s.digiflazzUser,
		"buyer_sku_code":   product.SKU,
		"customer_no":      customerNo,
		"ref_id":           order.ID,
		"sign":             sign,
	}

	body, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("marshal payload: %w", err)
	}

	var result struct {
		Data struct {
			Status  string `json:"status"`
			Message string `json:"message"`
			SN      string `json:"sn"`
		} `json:"data"`
	}

	retryCfg := retry.DefaultConfig()
	err = retry.Do(ctx, retryCfg, func(ctx context.Context) error {
		req, err := http.NewRequestWithContext(ctx, http.MethodPost, s.digiflazzURL, bytes.NewReader(body))
		if err != nil {
			return fmt.Errorf("create request: %w", err)
		}
		req.Header.Set("Content-Type", "application/json")

		resp, err := s.httpClient.Do(req)
		if err != nil {
			return fmt.Errorf("send request: %w", err)
		}
		defer resp.Body.Close()

		respBody, err := io.ReadAll(resp.Body)
		if err != nil {
			return fmt.Errorf("read response: %w", err)
		}

		if err := json.Unmarshal(respBody, &result); err != nil {
			return fmt.Errorf("parse response: %w, body: %s", err, string(respBody))
		}

		if result.Data.Status == "Gagal" || result.Data.Status == "Fail" {
			return fmt.Errorf("digiflazz transaction failed: %s", result.Data.Message)
		}

		return nil
	})

	if err != nil {
		return err
	}

	if result.Data.Status == constants.StatusSuccess {
		s.logger.Info("digiflazz topup success", slog.String("order_id", order.ID), slog.String("sn", result.Data.SN))
	} else {
		s.logger.Info("digiflazz topup pending", slog.String("order_id", order.ID), slog.String("status", result.Data.Status))
	}
	return nil
}

func (s *TopupService) buildCustomerNo(order *models.Order, product *models.Product) string {
	if product.Game == constants.GameMobileLegends && order.GameServer != "" {
		return order.GameUID + "|" + order.GameServer
	}
	return order.GameUID
}

func (s *TopupService) generateSign(refID string) string {
	raw := s.digiflazzUser + s.digiflazzAPIKey + refID
	hash := md5.Sum([]byte(raw))
	return hex.EncodeToString(hash[:])
}

func (s *TopupService) CheckTransactionStatus(orderID string) (status string, sn string, err error) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	order, err := s.orderRepo.GetByID(ctx, orderID)
	if err != nil {
		return "", "", fmt.Errorf("fetch order: %w", err)
	}

	product, err := s.productRepo.GetByID(ctx, order.ProductID)
	if err != nil {
		return "", "", fmt.Errorf("fetch product: %w", err)
	}

	customerNo := s.buildCustomerNo(order, product)
	sign := s.generateSign(orderID)

	payload := map[string]string{
		"username":       s.digiflazzUser,
		"buyer_sku_code": product.SKU,
		"customer_no":    customerNo,
		"ref_id":         orderID,
		"sign":           sign,
	}

	body, err := json.Marshal(payload)
	if err != nil {
		return "", "", fmt.Errorf("marshal payload: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, s.digiflazzURL, bytes.NewReader(body))
	if err != nil {
		return "", "", fmt.Errorf("create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := s.httpClient.Do(req)
	if err != nil {
		return "", "", fmt.Errorf("send request: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", "", fmt.Errorf("read response: %w", err)
	}

	var result struct {
		Data struct {
			Status  string `json:"status"`
			Message string `json:"message"`
			SN      string `json:"sn"`
		} `json:"data"`
	}
	if err := json.Unmarshal(respBody, &result); err != nil {
		return "", "", fmt.Errorf("parse response: %w, body: %s", err, string(respBody))
	}

	return result.Data.Status, result.Data.SN, nil
}

func (s *TopupService) GetProcessingOrders(ctx context.Context) ([]models.Order, error) {
	return s.orderRepo.ListProcessing(ctx)
}

func (s *TopupService) GetProduct(ctx context.Context, id string) (*models.Product, error) {
	return s.productRepo.GetByID(ctx, id)
}

func (s *TopupService) GetProductByGameAndDiamonds(ctx context.Context, game string, diamonds int) (*models.Product, error) {
	return s.productRepo.GetByGameAndDiamonds(ctx, game, diamonds)
}

func (s *TopupService) ListProducts(ctx context.Context, game string) ([]models.Product, error) {
	return s.productRepo.ListByGame(ctx, game)
}

func (s *TopupService) ListAllProducts(ctx context.Context) ([]models.Product, error) {
	return s.productRepo.ListAll(ctx)
}

func (s *TopupService) CompleteOrder(ctx context.Context, orderID, sn string) error {
	_, err := s.orderRepo.GetByID(ctx, orderID)
	if err != nil {
		return err
	}
	if sn != "" {
		if err := s.orderRepo.UpdateSerialNumber(ctx, orderID, sn); err != nil {
			return fmt.Errorf("update serial number: %w", err)
		}
	}
	return s.orderRepo.UpdateStatus(ctx, orderID, constants.StatusSuccess)
}

func (s *TopupService) FailOrder(ctx context.Context, orderID string) error {
	return s.orderRepo.UpdateStatus(ctx, orderID, constants.StatusFailed)
}
