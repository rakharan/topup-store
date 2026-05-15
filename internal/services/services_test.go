package services

import (
	"context"
	"testing"
	"time"

	"github.com/topup-store/internal/constants"
	"github.com/topup-store/internal/models"
	"github.com/topup-store/internal/repositories"
)

type mockOrderRepo struct {
	getByIDResult         *models.Order
	getByIDErr            error
	updateStatusErr       error
	updateStatusIfOk      bool
	updateStatusIfErr     error
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

func (m *mockOrderRepo) UpsertSnap(ctx context.Context, orderID, snapToken, snapRedirectURL string) error {
	return nil
}

func (m *mockOrderRepo) GetSnap(ctx context.Context, orderID string) (*models.OrderSnap, error) {
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

func (m *mockOrderRepo) ListFailedForRetry(ctx context.Context, maxRetries int) ([]models.Order, error) {
	return nil, nil
}

func (m *mockOrderRepo) IncrementRetryCount(ctx context.Context, id string) error {
	return nil
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

func (m *mockProductRepo) DecrementStock(ctx context.Context, id string) (bool, error) {
	return true, nil
}

func (m *mockProductRepo) IncrementStock(ctx context.Context, id string) error {
	return nil
}

func TestTopupService_ProcessOrder_NotPaid(t *testing.T) {
	orderRepo := &mockOrderRepo{
		getByIDResult: &models.Order{ID: "order-1", Status: "pending"},
	}
	productRepo := &mockProductRepo{}
	svc := NewTopupService(orderRepo, productRepo, "user", "key", "https://api.example.com", true, nil)

	err := svc.ProcessOrder(context.Background(), "order-1")
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

	err := svc.ProcessOrder(context.Background(), "order-1")
	if err == nil {
		t.Error("expected error when GetByID fails")
	}
}

func TestTopupService_BuildCustomerNo(t *testing.T) {
	svc := NewTopupService(nil, nil, "user", "key", "https://api.example.com", true, nil)
	order := &models.Order{GameUID: "12345", GameServer: "556"}

	tests := []struct {
		name    string
		product *models.Product
		want    string
	}{
		{
			name:    "mobile legends default concatenates uid and server",
			product: &models.Product{Game: constants.GameMobileLegends},
			want:    "12345556",
		},
		{
			name:    "explicit concat",
			product: &models.Product{Game: constants.GameMobileLegends, CustomerNoFormat: "uid_server_concat"},
			want:    "12345556",
		},
		{
			name:    "explicit space",
			product: &models.Product{Game: constants.GameMobileLegends, CustomerNoFormat: "uid_server_space"},
			want:    "12345 556",
		},
		{
			name:    "explicit uid only",
			product: &models.Product{Game: constants.GameMobileLegends, CustomerNoFormat: "uid"},
			want:    "12345",
		},
		{
			name:    "non mobile legends ignores server by default",
			product: &models.Product{Game: constants.GameFreeFire},
			want:    "12345",
		},
		{
			name:    "hoyo games default to uid pipe server",
			product: &models.Product{Game: constants.GameGenshinImpact},
			want:    "12345|556",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := svc.buildCustomerNo(order, tt.product)
			if got != tt.want {
				t.Fatalf("customer_no = %q; want %q", got, tt.want)
			}
		})
	}
}

func TestTopupService_InferHoyoGames(t *testing.T) {
	tests := []struct {
		name  string
		sku   string
		title string
		brand string
		want  string
	}{
		{name: "genshin sku", sku: "gi_60", want: constants.GameGenshinImpact},
		{name: "genshin name", title: "Genshin Impact 60 Genesis Crystals", want: constants.GameGenshinImpact},
		{name: "star rail sku", sku: "hsr_60", want: constants.GameHonkaiStarRail},
		{name: "star rail brand", brand: "Honkai Star Rail", want: constants.GameHonkaiStarRail},
		{name: "zenless sku", sku: "zzz_60", want: constants.GameZenlessZoneZero},
		{name: "honkai impact sku", sku: "hi3_60", want: constants.GameHonkaiImpact3},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := inferGame(tt.sku, tt.title, tt.brand)
			if got != tt.want {
				t.Fatalf("inferGame() = %q; want %q", got, tt.want)
			}
		})
	}
}

func TestTopupService_ExtractItemQuantityFromName(t *testing.T) {
	tests := []struct {
		name string
		want int
	}{
		{name: "MOBILELEGEND - 50 Diamond", want: 50},
		{name: "PUBG Mobile 60 UC", want: 60},
		{name: "Genshin Impact 60 Genesis Crystals", want: 60},
		{name: "Honkai Star Rail 60 Oneiric Shards", want: 60},
		{name: "Zenless Zone Zero 300 Monochrome", want: 300},
		{name: "Honkai Impact 3 330 B-Chips", want: 330},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := extractDiamondsFromName(tt.name)
			if got != tt.want {
				t.Fatalf("extractDiamondsFromName() = %d; want %d", got, tt.want)
			}
		})
	}
}

func TestTopupService_TrimsDigiflazzConfig(t *testing.T) {
	svc := NewTopupService(nil, nil, " user ", " key \n", " https://api.example.com ", true, nil)

	if svc.digiflazzUser != "user" {
		t.Fatalf("digiflazzUser = %q; want user", svc.digiflazzUser)
	}
	if svc.digiflazzAPIKey != "key" {
		t.Fatalf("digiflazzAPIKey = %q; want key", svc.digiflazzAPIKey)
	}
	if svc.digiflazzURL != "https://api.example.com" {
		t.Fatalf("digiflazzURL = %q; want https://api.example.com", svc.digiflazzURL)
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

	// Use a non-existent endpoint to avoid real HTTP calls
	svc := NewNotifyService("", "http://localhost:0", "", nil)

	for _, tt := range tests {
		err := svc.SendNotification(tt.phone, "test")
		if tt.valid && err != nil {
			// Valid phones should pass validation; network errors are expected with dummy endpoint
			if err.Error()[:6] != "send n" && err.Error()[:6] != "Post \"h" {
				t.Errorf("phone %q: unexpected error: %v", tt.phone, err)
			}
		}
		if !tt.valid && err == nil {
			t.Errorf("phone %q: expected error, got nil", tt.phone)
		}
	}
}
