package services

import (
	"context"
	"testing"

	"github.com/topup-store/internal/models"
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

func TestTopupService_ProcessOrder_NotPaid(t *testing.T) {
	orderRepo := &mockOrderRepo{
		getByIDResult: &models.Order{ID: "order-1", Status: "pending"},
	}
	productRepo := &mockProductRepo{}
	svc := NewTopupService(orderRepo, productRepo, "user", "key", "https://api.example.com", nil)

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
	svc := NewTopupService(orderRepo, productRepo, "user", "key", "https://api.example.com", nil)

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
	svc := NewPaymentService(orderRepo, "key", false)

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
