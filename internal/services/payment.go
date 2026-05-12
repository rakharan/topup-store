package services

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/midtrans/midtrans-go"
	"github.com/midtrans/midtrans-go/coreapi"
	"github.com/midtrans/midtrans-go/snap"
	"github.com/topup-store/internal/models"
	"github.com/topup-store/internal/repositories"
)

type PaymentService struct {
	orderRepo repositories.OrderRepository
	snap      *snap.Client
	core      *coreapi.Client
	isProd    bool
	logger    *slog.Logger
}

func NewPaymentService(orderRepo repositories.OrderRepository, serverKey string, isProd bool, logger *slog.Logger) *PaymentService {
	env := midtrans.Sandbox
	if isProd {
		env = midtrans.Production
	}

	snapClient := &snap.Client{}
	snapClient.New(serverKey, env)

	coreClient := &coreapi.Client{}
	coreClient.New(serverKey, env)

	return &PaymentService{
		orderRepo: orderRepo,
		snap:      snapClient,
		core:      coreClient,
		isProd:    isProd,
		logger:    logger,
	}
}

func (s *PaymentService) CreateQRIS(ctx context.Context, order *models.Order) (qrString, qrisURL, expiryTime string, err error) {
	gameName := map[string]string{
		"free_fire":      "Free Fire",
		"mobile_legends": "Mobile Legends",
		"pubg_mobile":    "PUBG Mobile",
	}[order.Channel]
	if gameName == "" {
		gameName = "Game Top-Up"
	}

	expiryDuration := 30
	expiryTimeVal := time.Now().Add(time.Duration(expiryDuration) * time.Minute)
	expiryTime = expiryTimeVal.Format(time.RFC3339)

	coreResp, coreErr := s.core.ChargeTransaction(&coreapi.ChargeReq{
		PaymentType: coreapi.PaymentTypeQris,
		TransactionDetails: midtrans.TransactionDetails{
			OrderID:  order.ID,
			GrossAmt: int64(order.AmountIDR),
		},
		CustomerDetails: &midtrans.CustomerDetails{
			Phone: order.UserPhone,
		},
		Items: &[]midtrans.ItemDetails{
			{
				ID:    order.ProductID,
				Price: int64(order.AmountIDR),
				Qty:   1,
				Name:  gameName,
			},
		},
		CustomExpiry: &coreapi.CustomExpiry{
			ExpiryDuration: expiryDuration,
			Unit:           "minute",
		},
	})

	if coreErr == nil && coreResp.QRString != "" {
		s.logger.Info("qris: core api success", slog.String("order_id", order.ID))
		if storeErr := s.orderRepo.UpsertQRIS(ctx, order.ID, "", coreResp.QRString, "", &expiryTimeVal); storeErr != nil {
			s.logger.Warn("qris: failed to store qr_string", slog.String("error", storeErr.Error()))
		}
		if storeErr := s.orderRepo.UpdateWithQRIS(ctx, order.ID, order.ID, ""); storeErr != nil {
			s.logger.Warn("qris: failed to update order", slog.String("error", storeErr.Error()))
		}
		return coreResp.QRString, "", expiryTime, nil
	}

	if coreErr != nil {
		s.logger.Warn("qris: core api failed, falling back to snap", slog.String("error", coreErr.Message))
	}

	snapReq := &snap.Request{
		TransactionDetails: midtrans.TransactionDetails{
			OrderID:  order.ID,
			GrossAmt: int64(order.AmountIDR),
		},
		Expiry: &snap.ExpiryDetails{
			Unit:     "minute",
			Duration: 30,
		},
		CustomerDetail: &midtrans.CustomerDetails{
			Phone: order.UserPhone,
		},
		Items: &[]midtrans.ItemDetails{
			{
				ID:    order.ProductID,
				Price: int64(order.AmountIDR),
				Qty:   1,
				Name:  gameName,
			},
		},
		CustomField1: order.GameUID,
		CustomField2: order.UserPhone,
		CustomField3: gameName,
		Metadata: map[string]string{
			"game_uid":    order.GameUID,
			"game_server": order.GameServer,
			"product_id":  order.ProductID,
			"channel":     order.Channel,
		},
	}

	snapResp, snapErr := s.snap.CreateTransaction(snapReq)
	if snapErr != nil {
		return "", "", "", fmt.Errorf("midtrans create transaction: %s", snapErr.Message)
	}

	if snapResp.RedirectURL == "" {
		return "", "", "", fmt.Errorf("midtrans returned empty QRIS URL")
	}

	qrisURL = snapResp.RedirectURL
	if storeErr := s.orderRepo.UpsertQRIS(ctx, order.ID, qrisURL, "", "", &expiryTimeVal); storeErr != nil {
		s.logger.Warn("qris: failed to store qris_url", slog.String("error", storeErr.Error()))
	}
	if storeErr := s.orderRepo.UpdateWithQRIS(ctx, order.ID, order.ID, qrisURL); storeErr != nil {
		s.logger.Warn("qris: failed to update order", slog.String("error", storeErr.Error()))
	}

	return "", qrisURL, expiryTime, nil
}

func (s *PaymentService) GetOrderByMidtransID(ctx context.Context, midtransOrderID string) (*models.Order, error) {
	return s.orderRepo.GetByMidtransID(ctx, midtransOrderID)
}

func (s *PaymentService) UpdateOrderStatus(ctx context.Context, orderID, status string) error {
	return s.orderRepo.UpdateStatus(ctx, orderID, status)
}

func (s *PaymentService) UpdateOrderStatusIf(ctx context.Context, orderID, newStatus, expectedStatus string) (bool, error) {
	return s.orderRepo.UpdateStatusIf(ctx, orderID, newStatus, expectedStatus)
}

func (s *PaymentService) UpdateOrderSerialNumber(ctx context.Context, orderID, sn string) error {
	return s.orderRepo.UpdateSerialNumber(ctx, orderID, sn)
}

func (s *PaymentService) GetOrder(ctx context.Context, orderID string) (*models.Order, error) {
	order, err := s.orderRepo.GetByID(ctx, orderID)
	if err != nil {
		order, err = s.orderRepo.GetByOrderNumber(ctx, orderID)
		if err != nil {
			return s.orderRepo.GetByDigiflazzRefID(ctx, orderID)
		}
	}
	return order, nil
}

func (s *PaymentService) ListOrders(ctx context.Context, page, perPage int) ([]models.Order, int, error) {
	return s.orderRepo.List(ctx, page, perPage)
}

func (s *PaymentService) CreateOrder(ctx context.Context, order *models.Order) error {
	return s.orderRepo.Create(ctx, order)
}

func (s *PaymentService) GetOrderByUIDAndPhone(ctx context.Context, gameUID, phone string) (*models.Order, error) {
	return s.orderRepo.GetByUIDAndPhone(ctx, gameUID, phone)
}

func (s *PaymentService) RecordStatusChange(ctx context.Context, orderID, fromStatus, toStatus, reason string) error {
	return s.orderRepo.InsertStatusHistory(ctx, orderID, fromStatus, toStatus, reason)
}

func (s *PaymentService) GetOrderStatusHistory(ctx context.Context, orderID string) ([]models.OrderStatusHistory, error) {
	return s.orderRepo.GetStatusHistory(ctx, orderID)
}

func (s *PaymentService) GetOrderQRIS(ctx context.Context, orderID string) (*models.OrderQRIS, error) {
	return s.orderRepo.GetQRIS(ctx, orderID)
}

func (s *PaymentService) CancelTransaction(orderID string) error {
	resp, mErr := s.core.CancelTransaction(orderID)
	if mErr != nil {
		return fmt.Errorf("midtrans cancel transaction: %s", mErr.Message)
	}
	s.logger.Info("midtrans: transaction canceled", slog.String("order_id", orderID), slog.String("status", resp.TransactionStatus))
	return nil
}

func (s *PaymentService) CheckTransactionStatus(orderID string) (string, string, error) {
	resp, mErr := s.core.CheckTransaction(orderID)
	if mErr != nil {
		return "", "", fmt.Errorf("midtrans check transaction: %s", mErr.Message)
	}
	return resp.TransactionStatus, resp.FraudStatus, nil
}

func (s *PaymentService) GetRecentOrdersByPhone(ctx context.Context, phone string, limit int) ([]models.Order, error) {
	return s.orderRepo.GetRecentByPhone(ctx, phone, limit)
}
