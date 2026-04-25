package services

import (
	"context"

	"github.com/topup-store/internal/models"
)

type PaymentServiceInterface interface {
	CreateOrder(ctx context.Context, order *models.Order) error
	GetOrder(ctx context.Context, orderID string) (*models.Order, error)
	UpdateOrderStatus(ctx context.Context, orderID, status string) error
	UpdateOrderStatusIf(ctx context.Context, orderID, newStatus, expectedStatus string) (bool, error)
	UpdateOrderSerialNumber(ctx context.Context, orderID, sn string) error
	ListOrders(ctx context.Context, page, perPage int) ([]models.Order, int, error)
	CreateQRIS(ctx context.Context, order *models.Order) (string, string, error)
	GetOrderByUIDAndPhone(ctx context.Context, gameUID, phone string) (*models.Order, error)
}

type TopupServiceInterface interface {
	GetProduct(ctx context.Context, id string) (*models.Product, error)
	GetProductByGameAndDiamonds(ctx context.Context, game string, diamonds int) (*models.Product, error)
	ListProducts(ctx context.Context, game string) ([]models.Product, error)
	ListAllProducts(ctx context.Context) ([]models.Product, error)
	ProcessOrder(orderID string) error
}

type NotifyServiceInterface interface {
	SendOrderConfirmation(ctx context.Context, order *models.Order, phone string) error
	SendTopupSuccess(ctx context.Context, order *models.Order, phone string) error
	SendTopupFailure(ctx context.Context, order *models.Order, phone string) error
	SendNotification(phone, message string) error
}
