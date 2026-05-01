package repositories

import (
	"context"
	"time"

	"github.com/topup-store/internal/models"
)

type OrderRepository interface {
	Create(ctx context.Context, order *models.Order) error
	GetByID(ctx context.Context, id string) (*models.Order, error)
	GetByMidtransID(ctx context.Context, midtransID string) (*models.Order, error)
	GetByDigiflazzRefID(ctx context.Context, refID string) (*models.Order, error)
	GetByUIDAndPhone(ctx context.Context, gameUID, phone string) (*models.Order, error)
	UpdateStatus(ctx context.Context, id, status string) error
	UpdateStatusIf(ctx context.Context, id, newStatus, expectedStatus string) (bool, error)
	UpdateSerialNumber(ctx context.Context, id, sn string) error
	UpdateWithQRIS(ctx context.Context, id, midtransOrderID, qrisURL string) error
	UpsertQRIS(ctx context.Context, orderID, qrisURL, qrisImageBase64 string) error
	GetQRIS(ctx context.Context, orderID string) (*models.OrderQRIS, error)
	InsertStatusHistory(ctx context.Context, orderID, fromStatus, toStatus, reason string) error
	GetStatusHistory(ctx context.Context, orderID string) ([]models.OrderStatusHistory, error)
	List(ctx context.Context, page, perPage int) ([]models.Order, int, error)
	ListByStatus(ctx context.Context, status string, page, perPage int) ([]models.Order, int, error)
	ListProcessing(ctx context.Context) ([]models.Order, error)
	ExpireOldPending(ctx context.Context) ([]models.Order, error)
	GetPendingOrdersOlderThan(ctx context.Context, age time.Duration) ([]models.Order, error)
	GetRecentByPhone(ctx context.Context, phone string, limit int) ([]models.Order, error)
	ListAllForExport(ctx context.Context) ([]OrderExportRow, error)
}

type ProductRepository interface {
	GetByID(ctx context.Context, id string) (*models.Product, error)
	GetByGameAndDiamonds(ctx context.Context, game string, diamonds int) (*models.Product, error)
	ListByGame(ctx context.Context, game string) ([]models.Product, error)
	ListAll(ctx context.Context) ([]models.Product, error)
	Create(ctx context.Context, p *models.Product) error
	Update(ctx context.Context, p *models.Product) error
	Delete(ctx context.Context, id string) error
	ExistsBySKU(ctx context.Context, sku string, excludeID string) (bool, error)
	UpdateCostPrice(ctx context.Context, sku string, costPrice int) error
	SyncPrice(ctx context.Context, sku string, costPrice, sellingPrice int) error
	CreateFromDigiflazz(ctx context.Context, sku, name, game, productType string, priceIDR, costPriceIDR, diamonds int, description string) error
}

type WebhookRepository interface {
	Log(ctx context.Context, log *models.WebhookLog) error
	List(ctx context.Context, source string, page, perPage int) ([]models.WebhookLog, int, error)
}
