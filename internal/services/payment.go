package services

import (
	"context"
	"fmt"

	"github.com/midtrans/midtrans-go"
	"github.com/midtrans/midtrans-go/snap"
	"github.com/topup-store/internal/models"
	"github.com/topup-store/internal/repositories"
)

type PaymentService struct {
	orderRepo repositories.OrderRepository
	snap      *snap.Client
	isProd    bool
}

func NewPaymentService(orderRepo repositories.OrderRepository, serverKey string, isProd bool) *PaymentService {
	env := midtrans.Sandbox
	if isProd {
		env = midtrans.Production
	}

	client := &snap.Client{}
	client.New(serverKey, env)

	return &PaymentService{
		orderRepo: orderRepo,
		snap:      client,
		isProd:    isProd,
	}
}

func (s *PaymentService) CreateQRIS(ctx context.Context, order *models.Order) (string, string, error) {
	req := &snap.Request{
		TransactionDetails: midtrans.TransactionDetails{
			OrderID:  order.ID,
			GrossAmt: int64(order.AmountIDR),
		},
	}

	snapResp, mErr := s.snap.CreateTransaction(req)
	if mErr != nil {
		return "", "", fmt.Errorf("midtrans create transaction: %s", mErr.Message)
	}

	if snapResp.RedirectURL == "" {
		return "", "", fmt.Errorf("midtrans returned empty QRIS URL")
	}

	qrisURL := snapResp.RedirectURL

	if err := s.orderRepo.UpdateWithQRIS(ctx, order.ID, order.ID, qrisURL); err != nil {
		return "", "", fmt.Errorf("update order with qris details: %w", err)
	}

	return qrisURL, "", nil
}

func (s *PaymentService) GetOrderByMidtransID(ctx context.Context, midtransOrderID string) (*models.Order, error) {
	return s.orderRepo.GetByMidtransID(ctx, midtransOrderID)
}

func (s *PaymentService) UpdateOrderStatus(ctx context.Context, orderID, status string) error {
	return s.orderRepo.UpdateStatus(ctx, orderID, status)
}

func (s *PaymentService) UpdateOrderSerialNumber(ctx context.Context, orderID, sn string) error {
	return s.orderRepo.UpdateSerialNumber(ctx, orderID, sn)
}

func (s *PaymentService) GetOrder(ctx context.Context, orderID string) (*models.Order, error) {
	return s.orderRepo.GetByID(ctx, orderID)
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
