package repositories

import (
	"context"

	"github.com/topup-store/internal/models"
)

type OrderRepository interface {
	Create(ctx context.Context, order *models.Order) error
	GetByID(ctx context.Context, id string) (*models.Order, error)
	GetByMidtransID(ctx context.Context, midtransID string) (*models.Order, error)
	GetByUIDAndPhone(ctx context.Context, gameUID, phone string) (*models.Order, error)
	UpdateStatus(ctx context.Context, id, status string) error
	UpdateSerialNumber(ctx context.Context, id, sn string) error
	UpdateWithQRIS(ctx context.Context, id, midtransOrderID, qrisURL string) error
	List(ctx context.Context, page, perPage int) ([]models.Order, int, error)
	ListByStatus(ctx context.Context, status string, page, perPage int) ([]models.Order, int, error)
	ListProcessing(ctx context.Context) ([]models.Order, error)
	ExpireOldPending(ctx context.Context) ([]models.Order, error)
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
}
