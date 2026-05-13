package handlers

import (
	"context"

	"github.com/topup-store/internal/models"
	"github.com/topup-store/internal/services"
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
	createQRISString       string
	createQRISExpiry       string
	createQRISErr          error
	getByUIDPhoneResult    *models.Order
	getByUIDPhoneErr       error
	cancelTransactionErr   error
	checkTransactionStatus string
	checkTransactionErr    error
	recentOrdersResult     []models.Order
	recentOrdersErr        error
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

func (m *mockPaymentService) CreateQRIS(ctx context.Context, order *models.Order) (string, string, string, error) {
	return m.createQRISString, m.createQRISURL, m.createQRISExpiry, m.createQRISErr
}

func (m *mockPaymentService) CreateSnapToken(ctx context.Context, order *models.Order) (string, error) {
	return "", nil
}

func (m *mockPaymentService) GetOrderByUIDAndPhone(ctx context.Context, gameUID, phone string) (*models.Order, error) {
	return m.getByUIDPhoneResult, m.getByUIDPhoneErr
}

func (m *mockPaymentService) UpdateOrderSerialNumber(ctx context.Context, orderID, sn string) error {
	return nil
}

func (m *mockPaymentService) RecordStatusChange(ctx context.Context, orderID, fromStatus, toStatus, reason string) error {
	return nil
}

func (m *mockPaymentService) GetOrderStatusHistory(ctx context.Context, orderID string) ([]models.OrderStatusHistory, error) {
	return nil, nil
}

func (m *mockPaymentService) GetOrderQRIS(ctx context.Context, orderID string) (*models.OrderQRIS, error) {
	return nil, nil
}

func (m *mockPaymentService) CancelTransaction(orderID string) error {
	return m.cancelTransactionErr
}

func (m *mockPaymentService) CheckTransactionStatus(orderID string) (string, string, error) {
	return m.checkTransactionStatus, "", m.checkTransactionErr
}

func (m *mockPaymentService) GetRecentOrdersByPhone(ctx context.Context, phone string, limit int) ([]models.Order, error) {
	return m.recentOrdersResult, m.recentOrdersErr
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

func (m *mockTopupService) ProcessOrder(ctx context.Context, orderID string) error {
	return m.processOrderErr
}

func (m *mockTopupService) FetchDigiflazzPrices(ctx context.Context) ([]services.DigiflazzPrice, error) {
	return nil, nil
}

func (m *mockTopupService) SyncPricesWithAutoCreate(ctx context.Context, marginType string, marginValue int) ([]services.SyncResult, int, int, int, error) {
	return nil, 0, 0, 0, nil
}

func (m *mockTopupService) GetBalance() int {
	return 100000
}

func (m *mockTopupService) DecrementProductStock(ctx context.Context, productID string) (bool, error) {
	return true, nil
}

func (m *mockTopupService) IncrementProductStock(ctx context.Context, productID string) error {
	return nil
}

type mockNotifyService struct {
	sendOrderConfirmationErr error
	sendTopupSuccessErr      error
	sendTopupFailureErr      error
	sendNotificationErr      error
}

func (m *mockNotifyService) SendOrderConfirmation(ctx context.Context, order *models.Order, product *models.Product, phone, qrisURL string) error {
	return m.sendOrderConfirmationErr
}

func (m *mockNotifyService) SendTopupSuccess(ctx context.Context, order *models.Order, product *models.Product, phone, serialNumber string) error {
	return m.sendTopupSuccessErr
}

func (m *mockNotifyService) SendTopupFailure(ctx context.Context, order *models.Order, product *models.Product, phone string) error {
	return m.sendTopupFailureErr
}

func (m *mockNotifyService) SendNotification(phone, message string) error {
	return m.sendNotificationErr
}

type mockWebhookRepo struct {
	isProcessed    bool
	isProcessedErr error
}

func (m *mockWebhookRepo) Log(ctx context.Context, log *models.WebhookLog) error {
	return nil
}

func (m *mockWebhookRepo) List(ctx context.Context, source string, page, perPage int) ([]models.WebhookLog, int, error) {
	return nil, 0, nil
}

func (m *mockWebhookRepo) IsWebhookProcessed(ctx context.Context, source, signature string) (bool, error) {
	return m.isProcessed, m.isProcessedErr
}

func (m *mockWebhookRepo) MarkWebhookProcessed(ctx context.Context, source, signature string) error {
	return nil
}

type mockProductRepo struct {
	getByIDResult  *models.Product
	getByIDErr     error
	listAllResult  []models.Product
	listAllErr     error
	createErr      error
	updateErr      error
	deleteErr      error
	existsBySKU    bool
	existsBySKUErr error
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
