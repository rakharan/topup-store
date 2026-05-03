package services

import (
	"context"
	"testing"
	"time"

	"github.com/topup-store/internal/models"
	"github.com/topup-store/internal/repositories"
)

type mockOrderRepo struct {
	getByIDResult *models.Order
	getByIDErr    error
	updateStatusErr error
	updateStatusIfOk bool
	updateStatusIfErr error
	updateSerialNumberErr error
}

func (m *mockOrderRepo) GetByID(ctx context.Context, id string) (*models.Order, error) {
	return m.getByIDResult, m.getByIDErr
}

func (m *mockOrderRepo) GetByMidtransID(ctx context.Context, midtransID string) (*models.Order, error) {
	return nil, nil
}

func (m *mockOrderRepo) GetByUIDAndPhone(ctx context.Context, gameUID, phone string) (*models.Order, error) {
	return nil, nil
}

func (m *mockOrderRepo) GetByOrderNumber(ctx context.Context, orderNumber string) (*models.Order, error) {
	return nil, nil
}

func (m *mockOrderRepo) Create(ctx context.Context, order *models.Order) error {
	return nil
}

func (m *mockOrderRepo) UpdateStatus(ctx context.Context, id, status string) error {
	return m.updateStatusErr
}

func (m *mockOrderRepo) UpdateStatusIf(ctx context.Context, id, newStatus, expectedStatus string) (bool, error) {
	return m.updateStatusIfOk, m.updateStatusIfErr
}

func (m *mockOrderRepo) UpdateSerialNumber(ctx context.Context, id, sn string) error {
	return m.updateSerialNumberErr
}

func (m *mockOrderRepo) UpdateWithQRIS(ctx context.Context, id, midtransOrderID, qrisURL string) error {
	return nil
}

func (m *mockOrderRepo) List(ctx context.Context, page, perPage int) ([]models.Order, int, error) {
	return nil, 0, nil
}

func (m *mockOrderRepo) ListByStatus(ctx context.Context, status string, page, perPage int) ([]models.Order, int, error) {
	return nil, 0, nil
}

func (m *mockOrderRepo) ListProcessing(ctx context.Context) ([]models.Order, error) {
	return nil, nil
}

func (m *mockOrderRepo) ExpireOldPending(ctx context.Context) ([]models.Order, error) {
	return nil, nil
}

func (m *mockOrderRepo) GetByDigiflazzRefID(ctx context.Context, refID string) (*models.Order, error) {
	return nil, nil
}

func (m *mockOrderRepo) UpsertQRIS(ctx context.Context, orderID, qrisURL, qrString, qrisImageBase64 string, expiryTime *time.Time) error {
	return nil
}

func (m *mockOrderRepo) GetQRIS(ctx context.Context, orderID string) (*models.OrderQRIS, error) {
	return nil, nil
}

func (m *mockOrderRepo) InsertStatusHistory(ctx context.Context, orderID, fromStatus, toStatus, reason string) error {
	return nil
}

func (m *mockOrderRepo) GetStatusHistory(ctx context.Context, orderID string) ([]models.OrderStatusHistory, error) {
	return nil, nil
}

func (m *mockOrderRepo) GetPendingOrdersOlderThan(ctx context.Context, age time.Duration) ([]models.Order, error) {
	return nil, nil
}

func (m *mockOrderRepo) GetRecentByPhone(ctx context.Context, phone string, limit int) ([]models.Order, error) {
	return nil, nil
}

func (m *mockOrderRepo) ListAllForExport(ctx context.Context) ([]repositories.OrderExportRow, error) {
	return nil, nil
}

func (m *mockOrderRepo) GetDailyRevenue(ctx context.Context, startDate, endDate time.Time) ([]repositories.DailyRevenue, error) {
	return nil, nil
}

func (m *mockOrderRepo) GetTopGamesByRevenue(ctx context.Context, startDate, endDate time.Time) ([]repositories.GameStats, error) {
	return nil, nil
}

func (m *mockOrderRepo) GetOverallStats(ctx context.Context, startDate, endDate time.Time) (*repositories.OverallStats, error) {
	return nil, nil
}

type mockProductRepo struct {
	getByIDResult *models.Product
	getByIDErr    error
}

func (m *mockProductRepo) GetByID(ctx context.Context, id string) (*models.Product, error) {
	return m.getByIDResult, m.getByIDErr
}

func (m *mockProductRepo) GetByGameAndDiamonds(ctx context.Context, game string, diamonds int) (*models.Product, error) {
	return nil, nil
}

func (m *mockProductRepo) ListByGame(ctx context.Context, game string) ([]models.Product, error) {
	return nil, nil
}

func (m *mockProductRepo) ListAll(ctx context.Context) ([]models.Product, error) {
	return nil, nil
}

func (m *mockProductRepo) Create(ctx context.Context, p *models.Product) error {
	return nil
}

func (m *mockProductRepo) Update(ctx context.Context, p *models.Product) error {
	return nil
}

func (m *mockProductRepo) Delete(ctx context.Context, id string) error {
	return nil
}

func (m *mockProductRepo) ExistsBySKU(ctx context.Context, sku string, excludeID string) (bool, error) {
	return false, nil
}

func (m *mockProductRepo) UpdateCostPrice(ctx context.Context, sku string, costPrice int) error {
	return nil
}

func (m *mockProductRepo) SyncPrice(ctx context.Context, sku string, costPrice, sellingPrice int) error {
	return nil
}

func (m *mockProductRepo) CreateFromDigiflazz(ctx context.Context, sku, name, game, productType string, priceIDR, costPriceIDR, diamonds int, description string) error {
	return nil
}

func TestTopupService_ProcessOrder_NotPaid(t *testing.T) {
	orderRepo := &mockOrderRepo{
		getByIDResult: &models.Order{ID: "order-1", Status: "pending"},
	}
	productRepo := &mockProductRepo{}
	svc := NewTopupService(orderRepo, productRepo, "user", "key", "https://api.example.com", true, nil)

	err := svc.ProcessOrder("order-1")
	if err == nil {
		t.Error("expected error for non-paid order")
	}
}

func TestTopupService_ProcessOrder_GetOrderFails(t *testing.T) {
	orderRepo := &mockOrderRepo{
		updateStatusIfOk: true,
		getByIDErr:       context.Canceled,
	}
	productRepo := &mockProductRepo{}
	svc := NewTopupService(orderRepo, productRepo, "user", "key", "https://api.example.com", true, nil)

	err := svc.ProcessOrder("order-1")
	if err == nil {
		t.Error("expected error when GetByID fails")
	}
}

func TestPaymentService_UpdateOrderStatusIf(t *testing.T) {
	orderRepo := &mockOrderRepo{
		updateStatusIfOk:  true,
		updateStatusIfErr: nil,
	}
	svc := NewPaymentService(orderRepo, "key", false, nil)

	ok, err := svc.UpdateOrderStatusIf(context.Background(), "order-1", "processing", "paid")
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
	if !ok {
		t.Error("expected true")
	}
}

func TestNotifyService_InvalidPhone(t *testing.T) {
	svc := NewNotifyService("", "http://localhost:3001", "", nil)

	err := svc.SendNotification("invalid", "test message")
	if err == nil {
		t.Error("expected error for invalid phone")
	}
}

func TestNotifyService_ValidPhonePrefix(t *testing.T) {
	tests := []struct {
		phone string
		valid bool
	}{
		{"+6281234567890", true},
		{"6281234567890", true},
		{"invalid", false},
		{"", false},
		{"  ", false},
	}

	svc := NewNotifyService("", "http://localhost:3001", "", nil)

	for _, tt := range tests {
		err := svc.SendNotification(tt.phone, "test")
		if tt.valid && err != nil && err.Error()[:6] != "send n" {
			t.Errorf("phone %q: unexpected error: %v", tt.phone, err)
		}
		if !tt.valid && err == nil {
			t.Errorf("phone %q: expected error, got nil", tt.phone)
		}
	}
}
