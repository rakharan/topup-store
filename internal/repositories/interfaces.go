package repositories

import (
	"context"
	"time"

	"github.com/topup-store/internal/models"
)

type OrderRepository interface {
	Create(ctx context.Context, order *models.Order) error
	GetByID(ctx context.Context, id string) (*models.Order, error)
	GetByOrderNumber(ctx context.Context, orderNumber string) (*models.Order, error)
	GetByMidtransID(ctx context.Context, midtransID string) (*models.Order, error)
	GetByDigiflazzRefID(ctx context.Context, refID string) (*models.Order, error)
	GetByUIDAndPhone(ctx context.Context, gameUID, phone string) (*models.Order, error)
	UpdateStatus(ctx context.Context, id, status string) error
	UpdateStatusIf(ctx context.Context, id, newStatus, expectedStatus string) (bool, error)
	UpdateSerialNumber(ctx context.Context, id, sn string) error
	UpdateWithQRIS(ctx context.Context, id, midtransOrderID, qrisURL string) error
	UpsertQRIS(ctx context.Context, orderID, qrisURL, qrString, qrisImageBase64 string, expiryTime *time.Time) error
	GetQRIS(ctx context.Context, orderID string) (*models.OrderQRIS, error)
	UpsertSnap(ctx context.Context, orderID, snapToken, snapRedirectURL string) error
	GetSnap(ctx context.Context, orderID string) (*models.OrderSnap, error)
	InsertStatusHistory(ctx context.Context, orderID, fromStatus, toStatus, reason string) error
	GetStatusHistory(ctx context.Context, orderID string) ([]models.OrderStatusHistory, error)
	List(ctx context.Context, page, perPage int) ([]models.Order, int, error)
	ListProcessing(ctx context.Context) ([]models.Order, error)
	ExpireOldPending(ctx context.Context) ([]models.Order, error)
	GetPendingOrdersOlderThan(ctx context.Context, age time.Duration) ([]models.Order, error)
	GetRecentByPhone(ctx context.Context, phone string, limit int) ([]models.Order, error)
	ListAllForExport(ctx context.Context) ([]OrderExportRow, error)
	GetDailyRevenue(ctx context.Context, startDate, endDate time.Time) ([]DailyRevenue, error)
	GetTopGamesByRevenue(ctx context.Context, startDate, endDate time.Time) ([]GameStats, error)
	GetOverallStats(ctx context.Context, startDate, endDate time.Time) (*OverallStats, error)
	ListFailedForRetry(ctx context.Context, maxRetries int) ([]models.Order, error)
	IncrementRetryCount(ctx context.Context, id string) error
	CountSuccessOrders(ctx context.Context) (int, error)
}

type ProductRepository interface {
	GetByID(ctx context.Context, id string) (*models.Product, error)
	GetByGameAndDiamonds(ctx context.Context, game string, itemQty int) (*models.Product, error)
	ListByGame(ctx context.Context, game string) ([]models.Product, error)
	ListAll(ctx context.Context) ([]models.Product, error)
	Create(ctx context.Context, p *models.Product) error
	Update(ctx context.Context, p *models.Product) error
	Delete(ctx context.Context, id string) error
	ExistsBySKU(ctx context.Context, sku string, excludeID string) (bool, error)
	UpdateCostPrice(ctx context.Context, sku string, costPrice int) error
	SyncPrice(ctx context.Context, sku string, costPrice, sellingPrice int) error
	CreateFromDigiflazz(ctx context.Context, sku, name, game, productType string, priceIDR, costPriceIDR, itemQty int, description string) error
	DecrementStock(ctx context.Context, id string) (bool, error)
	IncrementStock(ctx context.Context, id string) error
}

type WebhookRepository interface {
	Log(ctx context.Context, log *models.WebhookLog) error
	List(ctx context.Context, source string, page, perPage int) ([]models.WebhookLog, int, error)
	IsWebhookProcessed(ctx context.Context, source, signature string) (bool, error)
	MarkWebhookProcessed(ctx context.Context, source, signature string) error
}

type AuditLogEntry struct {
	Action     string
	EntityType string
	EntityID   string
	OldValue   string
	NewValue   string
	AdminIP    string
	AdminUA    string
}

type BlockedIdentityRepository interface {
	Create(ctx context.Context, b *models.BlockedIdentity) error
	Delete(ctx context.Context, id string) error
	List(ctx context.Context) ([]models.BlockedIdentity, error)
	IsBlocked(ctx context.Context, phone, gameUID, ipAddress string) (bool, string, error)
}

type AuditLogRepository interface {
	Log(ctx context.Context, entry *AuditLogEntry) error
}

type ReferralCodeRepository interface {
	Create(ctx context.Context, code *models.ReferralCode) error
	GetByCode(ctx context.Context, code string) (*models.ReferralCode, error)
	List(ctx context.Context) ([]models.ReferralCode, error)
	Delete(ctx context.Context, id string) error
	IncrementUsage(ctx context.Context, id string) error
	ApplyToOrder(ctx context.Context, orderID, codeID string, discount int) error
	ListPointBalances(ctx context.Context) ([]models.ReferralPointBalance, error)
	RedeemPoints(ctx context.Context, ownerPhone, couponCode string, points int) error
}
