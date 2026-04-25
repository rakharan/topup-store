package handlers

import (
	"context"

	"github.com/topup-store/internal/models"
)

type mockPaymentService struct {
	createOrderErr         error
	getOrderResult         *models.Order
	getOrderErr            error
	updateOrderStatusErr   error
	updateOrderStatusIfOk  bool
	updateOrderStatusIfErr error
	listOrdersResult       []models.Order
	listOrdersTotal        int
	listOrdersErr          error
	createQRISURL          string
	createQRISBase64       string
	createQRISErr          error
	getByUIDPhoneResult    *models.Order
	getByUIDPhoneErr       error
}

func (m *mockPaymentService) CreateOrder(ctx context.Context, order *models.Order) error {
	return m.createOrderErr
}

func (m *mockPaymentService) GetOrder(ctx context.Context, orderID string) (*models.Order, error) {
	return m.getOrderResult, m.getOrderErr
}

func (m *mockPaymentService) UpdateOrderStatus(ctx context.Context, orderID, status string) error {
	return m.updateOrderStatusErr
}

func (m *mockPaymentService) UpdateOrderStatusIf(ctx context.Context, orderID, newStatus, expectedStatus string) (bool, error) {
	return m.updateOrderStatusIfOk, m.updateOrderStatusIfErr
}

func (m *mockPaymentService) ListOrders(ctx context.Context, page, perPage int) ([]models.Order, int, error) {
	return m.listOrdersResult, m.listOrdersTotal, m.listOrdersErr
}

func (m *mockPaymentService) CreateQRIS(ctx context.Context, order *models.Order) (string, string, error) {
	return m.createQRISURL, m.createQRISBase64, m.createQRISErr
}

func (m *mockPaymentService) GetOrderByUIDAndPhone(ctx context.Context, gameUID, phone string) (*models.Order, error) {
	return m.getByUIDPhoneResult, m.getByUIDPhoneErr
}

func (m *mockPaymentService) UpdateOrderSerialNumber(ctx context.Context, orderID, sn string) error {
	return nil
}

type mockTopupService struct {
	getProductResult       *models.Product
	getProductErr          error
	getProductByGameResult *models.Product
	getProductByGameErr    error
	listProductsResult     []models.Product
	listProductsErr        error
	listAllProductsResult  []models.Product
	listAllProductsErr     error
	processOrderErr        error
}

func (m *mockTopupService) GetProduct(ctx context.Context, id string) (*models.Product, error) {
	return m.getProductResult, m.getProductErr
}

func (m *mockTopupService) GetProductByGameAndDiamonds(ctx context.Context, game string, diamonds int) (*models.Product, error) {
	return m.getProductByGameResult, m.getProductByGameErr
}

func (m *mockTopupService) ListProducts(ctx context.Context, game string) ([]models.Product, error) {
	return m.listProductsResult, m.listProductsErr
}

func (m *mockTopupService) ListAllProducts(ctx context.Context) ([]models.Product, error) {
	return m.listAllProductsResult, m.listAllProductsErr
}

func (m *mockTopupService) ProcessOrder(orderID string) error {
	return m.processOrderErr
}

type mockNotifyService struct {
	sendOrderConfirmationErr error
	sendTopupSuccessErr      error
	sendNotificationErr      error
}

func (m *mockNotifyService) SendOrderConfirmation(ctx context.Context, order *models.Order, phone string) error {
	return m.sendOrderConfirmationErr
}

func (m *mockNotifyService) SendTopupSuccess(ctx context.Context, order *models.Order, phone string) error {
	return m.sendTopupSuccessErr
}

func (m *mockNotifyService) SendTopupFailure(ctx context.Context, order *models.Order, phone string) error {
	return nil
}

func (m *mockNotifyService) SendNotification(phone, message string) error {
	return m.sendNotificationErr
}

type mockProductRepo struct {
	getByIDResult    *models.Product
	getByIDErr       error
	listAllResult    []models.Product
	listAllErr       error
	createErr        error
	updateErr        error
	deleteErr        error
	existsBySKU      bool
	existsBySKUErr   error
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
	return m.listAllResult, m.listAllErr
}

func (m *mockProductRepo) Create(ctx context.Context, p *models.Product) error {
	return m.createErr
}

func (m *mockProductRepo) Update(ctx context.Context, p *models.Product) error {
	return m.updateErr
}

func (m *mockProductRepo) Delete(ctx context.Context, id string) error {
	return m.deleteErr
}

func (m *mockProductRepo) ExistsBySKU(ctx context.Context, sku string, excludeID string) (bool, error) {
	return m.existsBySKU, m.existsBySKUErr
}
