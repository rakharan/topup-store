package handlers

import (
	"bytes"
	"crypto/hmac"
	"crypto/sha1"
	"crypto/sha512"
	"encoding/hex"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"github.com/topup-store/internal/cache"
	"github.com/topup-store/internal/models"
)

var testLogger = slog.New(slog.NewTextHandler(os.Stderr, nil))

func TestValidateOrderInput(t *testing.T) {
	tests := []struct {
		name       string
		game       string
		gameUID    string
		gameServer string
		productID  string
		itemQty    int
		phone      string
		wantErr    bool
	}{
		{"valid free_fire", "free_fire", "12345678", "", "abc", 0, "6281234567890", false},
		{"valid mobile_legends", "mobile_legends", "12345", "1234", "abc", 0, "6281234567890", false},
		{"valid pubg_mobile", "pubg_mobile", "5123456789", "", "abc", 0, "6281234567890", false},
		{"valid genshin impact", "genshin_impact", "800123456", "asia", "abc", 0, "6281234567890", false},
		{"invalid game", "valorant", "123", "", "abc", 0, "6281234567890", true},
		{"missing game_uid", "free_fire", "", "", "abc", 0, "6281234567890", true},
		{"non-numeric game_uid", "free_fire", "abc123", "", "abc", 0, "6281234567890", true},
		{"missing server for ML", "mobile_legends", "12345", "", "abc", 0, "6281234567890", true},
		{"missing server for genshin", "genshin_impact", "800123456", "", "abc", 0, "6281234567890", true},
		{"invalid server for genshin", "genshin_impact", "800123456", "asia|bad", "abc", 0, "6281234567890", true},
		{"invalid phone", "free_fire", "12345678", "", "abc", 0, "abc", true},
		{"short phone", "free_fire", "12345678", "", "abc", 0, "1234567", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := struct {
				Game       string `json:"game"`
				GameUID    string `json:"game_uid"`
				GameServer string `json:"game_server"`
				ProductID  string `json:"product_id"`
				ItemQty    int    `json:"item_qty"`
				Phone      string `json:"phone"`
			}{
				Game:       tt.game,
				GameUID:    tt.gameUID,
				GameServer: tt.gameServer,
				ProductID:  tt.productID,
				ItemQty:    tt.itemQty,
				Phone:      tt.phone,
			}
			err := validateOrderInput(req)
			if (err != nil) != tt.wantErr {
				t.Errorf("validateOrderInput() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestCreateOrder_InvalidJSON(t *testing.T) {
	h := &OrderHandler{logger: testLogger}
	req := httptest.NewRequest(http.MethodPost, "/api/orders", strings.NewReader("not json"))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	h.CreateOrder(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("expected status %d, got %d", http.StatusBadRequest, w.Code)
	}
}

func TestCreateOrder_InvalidGame(t *testing.T) {
	h := &OrderHandler{logger: testLogger}
	body := `{"game":"valorant","game_uid":"123","product_id":"abc","phone":"6281234567890"}`
	req := httptest.NewRequest(http.MethodPost, "/api/orders", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	h.CreateOrder(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("expected status %d, got %d", http.StatusBadRequest, w.Code)
	}
}

func TestCreateOrder_EmptyBody(t *testing.T) {
	h := &OrderHandler{logger: testLogger}
	req := httptest.NewRequest(http.MethodPost, "/api/orders", bytes.NewReader([]byte{}))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	h.CreateOrder(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("expected status %d, got %d", http.StatusBadRequest, w.Code)
	}
}

func TestCreateOrder_ProductNotFound(t *testing.T) {
	h := &OrderHandler{
		topupSvc: &mockTopupService{getProductErr: errors.New("not found")},
		logger:   testLogger,
	}
	body := `{"game":"free_fire","game_uid":"12345678","product_id":"nonexistent","phone":"6281234567890"}`
	req := httptest.NewRequest(http.MethodPost, "/api/orders", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	h.CreateOrder(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("expected status %d, got %d", http.StatusBadRequest, w.Code)
	}
}

func TestCreateOrder_Success(t *testing.T) {
	product := &models.Product{
		ID:       "prod-1",
		Game:     "free_fire",
		Name:     "70 Diamonds",
		PriceIDR: 10000,
		SKU:      "ff-70",
		Stock:    -1,
	}

	h := &OrderHandler{
		paymentSvc: &mockPaymentService{
			createOrderErr:   nil,
			createQRISURL:    "https://qris.example.com/pay",
			createQRISString: "",
			createQRISExpiry: "",
			createQRISErr:    nil,
		},
		topupSvc: &mockTopupService{
			getProductResult: product,
		},
		notifySvc: &mockNotifyService{},
		logger:    testLogger,
	}

	body := `{"game":"free_fire","game_uid":"12345678","product_id":"prod-1","phone":"6281234567890"}`
	req := httptest.NewRequest(http.MethodPost, "/api/orders", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	h.CreateOrder(w, req)

	if w.Code != http.StatusCreated {
		t.Errorf("expected status %d, got %d", http.StatusCreated, w.Code)
	}

	var resp map[string]any
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}
	if resp["success"] != true {
		t.Errorf("expected success=true, got %v", resp["success"])
	}
	data := resp["data"].(map[string]any)
	if data["order_id"] == "" {
		t.Error("expected order_id in response")
	}
	if data["qris_url"] != "https://qris.example.com/pay" {
		t.Errorf("expected qris_url, got %v", data["qris_url"])
	}
}

func TestCreateOrder_MissingUID(t *testing.T) {
	h := &OrderHandler{logger: testLogger}
	body := `{"game":"free_fire","game_uid":"","product_id":"abc","phone":"6281234567890"}`
	req := httptest.NewRequest(http.MethodPost, "/api/orders", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	h.CreateOrder(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("expected status %d, got %d", http.StatusBadRequest, w.Code)
	}
}

func TestGetOrder_NotFound(t *testing.T) {
	h := &OrderHandler{
		paymentSvc: &mockPaymentService{getOrderErr: errors.New("not found")},
		logger:     testLogger,
	}
	req := httptest.NewRequest(http.MethodGet, "/api/orders/nonexistent", nil)
	w := httptest.NewRecorder()

	h.GetOrder(w, req)

	if w.Code != http.StatusNotFound {
		t.Errorf("expected status %d, got %d", http.StatusNotFound, w.Code)
	}
}

func TestGetOrder_Success(t *testing.T) {
	order := &models.Order{
		ID:        "order-123",
		ProductID: "prod-1",
		UserPhone: "6281234567890",
		GameUID:   "12345678",
		AmountIDR: 10000,
		Status:    "pending",
	}

	h := &OrderHandler{
		paymentSvc: &mockPaymentService{getOrderResult: order},
		logger:     testLogger,
	}
	req := httptest.NewRequest(http.MethodGet, "/api/orders/order-123", nil)
	w := httptest.NewRecorder()

	h.GetOrder(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected status %d, got %d", http.StatusOK, w.Code)
	}

	var resp map[string]any
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}
	if resp["success"] != true {
		t.Error("expected success=true")
	}
}

func TestLookupOrder_Success(t *testing.T) {
	order := &models.Order{
		ID:        "order-123",
		GameUID:   "12345678",
		UserPhone: "6281234567890",
	}

	h := &OrderHandler{
		paymentSvc: &mockPaymentService{getByUIDPhoneResult: order},
		logger:     testLogger,
	}
	body := `{"game_uid":"12345678","phone":"6281234567890"}`
	req := httptest.NewRequest(http.MethodPost, "/api/orders/lookup", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	h.LookupOrder(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected status %d, got %d", http.StatusOK, w.Code)
	}
}

func TestValidationError_Error(t *testing.T) {
	err := &validationError{field: "game", message: "is required"}
	expected := "game: is required"
	if err.Error() != expected {
		t.Errorf("expected '%s', got '%s'", expected, err.Error())
	}
}

func TestMapTransactionStatus(t *testing.T) {
	wh := &WebhookHandler{}

	tests := []struct {
		txStatus    string
		fraudStatus string
		expected    string
	}{
		{"settlement", "", "paid"},
		{"capture", "accept", "paid"},
		{"capture", "deny", ""},
		{"pending", "", "pending"},
		{"deny", "", "failed"},
		{"expire", "", "failed"},
		{"cancel", "", "failed"},
		{"unknown", "", ""},
	}

	for _, tt := range tests {
		t.Run(tt.txStatus+"_"+tt.fraudStatus, func(t *testing.T) {
			result := wh.mapTransactionStatus(tt.txStatus, tt.fraudStatus)
			if result != tt.expected {
				t.Errorf("expected '%s', got '%s'", tt.expected, result)
			}
		})
	}
}

func TestAdminHandler_ProcessOrder_MissingOrderID(t *testing.T) {
	ah := &AdminHandler{adminPass: "testpass", productRepo: &mockProductRepo{}}

	req := httptest.NewRequest(http.MethodPost, "/admin/process-order", strings.NewReader(`{"order_id":""}`))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	ah.ProcessOrder(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("expected status %d, got %d", http.StatusBadRequest, w.Code)
	}
}

func TestAdminHandler_ProcessOrder_OrderNotFound(t *testing.T) {
	ah := &AdminHandler{
		paymentSvc:  &mockPaymentService{getOrderErr: errors.New("not found")},
		productRepo: &mockProductRepo{},
	}

	req := httptest.NewRequest(http.MethodPost, "/admin/process-order", strings.NewReader(`{"order_id":"nonexistent"}`))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	ah.ProcessOrder(w, req)

	if w.Code != http.StatusNotFound {
		t.Errorf("expected status %d, got %d", http.StatusNotFound, w.Code)
	}
}

func TestAdminHandler_ProcessOrder_WrongStatus(t *testing.T) {
	ah := &AdminHandler{
		paymentSvc:  &mockPaymentService{getOrderResult: &models.Order{ID: "order-123", Status: "paid"}},
		productRepo: &mockProductRepo{},
	}

	req := httptest.NewRequest(http.MethodPost, "/admin/process-order", strings.NewReader(`{"order_id":"order-123"}`))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	ah.ProcessOrder(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("expected status %d, got %d", http.StatusBadRequest, w.Code)
	}
}

func TestAdminHandler_ProcessOrder_Success(t *testing.T) {
	ah := &AdminHandler{
		paymentSvc:  &mockPaymentService{getOrderResult: &models.Order{ID: "order-123", Status: "pending"}},
		topupSvc:    &mockTopupService{},
		productRepo: &mockProductRepo{},
		logger:      testLogger,
	}

	req := httptest.NewRequest(http.MethodPost, "/admin/process-order", strings.NewReader(`{"order_id":"order-123"}`))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	ah.ProcessOrder(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected status %d, got %d", http.StatusOK, w.Code)
	}
}

func TestProductHandler_GetProduct_NotFound(t *testing.T) {
	ph := &ProductHandler{
		topupSvc: &mockTopupService{getProductErr: errors.New("not found")},
		cache:    &cache.Cache{},
		logger:   testLogger,
	}
	req := httptest.NewRequest(http.MethodGet, "/api/products/nonexistent", nil)
	w := httptest.NewRecorder()

	ph.GetProduct(w, req)

	if w.Code != http.StatusNotFound {
		t.Errorf("expected status %d, got %d", http.StatusNotFound, w.Code)
	}
}

func TestProductHandler_GetProduct_Success(t *testing.T) {
	product := &models.Product{
		ID:       "prod-1",
		Game:     "free_fire",
		Name:     "70 Diamonds",
		PriceIDR: 10000,
		SKU:      "ff-70",
	}

	ph := &ProductHandler{
		topupSvc: &mockTopupService{getProductResult: product},
		cache:    &cache.Cache{},
		logger:   testLogger,
	}
	req := httptest.NewRequest(http.MethodGet, "/api/products/prod-1", nil)
	w := httptest.NewRecorder()

	ph.GetProduct(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected status %d, got %d", http.StatusOK, w.Code)
	}
}

func TestProductHandler_ListProducts_ByGame(t *testing.T) {
	products := []models.Product{
		{ID: "prod-1", Game: "free_fire", Name: "70 Diamonds", PriceIDR: 10000},
		{ID: "prod-2", Game: "free_fire", Name: "140 Diamonds", PriceIDR: 20000},
	}

	ph := &ProductHandler{
		topupSvc: &mockTopupService{listProductsResult: products},
		cache:    &cache.Cache{},
		logger:   testLogger,
	}
	req := httptest.NewRequest(http.MethodGet, "/api/products?game=free_fire", nil)
	w := httptest.NewRecorder()

	ph.ListProducts(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected status %d, got %d", http.StatusOK, w.Code)
	}
}

func TestProductHandler_ListProducts_All(t *testing.T) {
	products := []models.Product{
		{ID: "prod-1", Game: "free_fire", Name: "70 Diamonds"},
		{ID: "prod-2", Game: "mobile_legends", Name: "86 Diamonds"},
	}

	ph := &ProductHandler{
		topupSvc: &mockTopupService{listAllProductsResult: products},
		cache:    &cache.Cache{},
		logger:   testLogger,
	}
	req := httptest.NewRequest(http.MethodGet, "/api/products", nil)
	w := httptest.NewRecorder()

	ph.ListProducts(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected status %d, got %d", http.StatusOK, w.Code)
	}
}

func TestWebhookHandler_Midtrans_InvalidPayload(t *testing.T) {
	wh := &WebhookHandler{midtransKey: "test-key", webhookRepo: &mockWebhookRepo{}, logger: testLogger}
	req := httptest.NewRequest(http.MethodPost, "/webhook/midtrans", strings.NewReader("not json"))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	wh.Midtrans(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("expected status %d, got %d", http.StatusBadRequest, w.Code)
	}
}

func TestWebhookHandler_Midtrans_InvalidSignature(t *testing.T) {
	wh := &WebhookHandler{midtransKey: "test-key", webhookRepo: &mockWebhookRepo{}, logger: testLogger}
	body := `{
		"order_id": "order-123",
		"status_code": "200",
		"gross_amount": "10000.00",
		"signature_key": "invalid_signature",
		"transaction_status": "settlement"
	}`
	req := httptest.NewRequest(http.MethodPost, "/webhook/midtrans", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	wh.Midtrans(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Errorf("expected status %d, got %d", http.StatusUnauthorized, w.Code)
	}
}

func TestWebhookHandler_VerifySignature(t *testing.T) {
	wh := &WebhookHandler{midtransKey: "test-key", logger: testLogger}

	hashString := "order-123" + "200" + "10000.00" + "test-key"
	hash := sha512.New()
	hash.Write([]byte(hashString))
	validSig := hex.EncodeToString(hash.Sum(nil))

	if !wh.verifySignature(struct {
		OrderID           string `json:"order_id"`
		StatusCode        string `json:"status_code"`
		GrossAmount       string `json:"gross_amount"`
		SignatureKey      string `json:"signature_key"`
		TransactionStatus string `json:"transaction_status"`
		FraudStatus       string `json:"fraud_status"`
	}{
		OrderID:      "order-123",
		StatusCode:   "200",
		GrossAmount:  "10000.00",
		SignatureKey: validSig,
	}) {
		t.Error("expected valid signature to pass verification")
	}

	if wh.verifySignature(struct {
		OrderID           string `json:"order_id"`
		StatusCode        string `json:"status_code"`
		GrossAmount       string `json:"gross_amount"`
		SignatureKey      string `json:"signature_key"`
		TransactionStatus string `json:"transaction_status"`
		FraudStatus       string `json:"fraud_status"`
	}{
		OrderID:      "order-123",
		StatusCode:   "200",
		GrossAmount:  "10000.00",
		SignatureKey: "wrong_signature",
	}) {
		t.Error("expected invalid signature to fail verification")
	}
}

func TestWebhookHandler_Digiflazz_InvalidPayload(t *testing.T) {
	wh := &WebhookHandler{digiflazzUser: "user", digiflazzAPIKey: "key", webhookRepo: &mockWebhookRepo{}, logger: testLogger}
	req := httptest.NewRequest(http.MethodPost, "/webhook/digiflazz", strings.NewReader("not json"))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	wh.Digiflazz(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected status %d, got %d", http.StatusOK, w.Code)
	}
}

func TestWebhookHandler_Digiflazz_PingEvent(t *testing.T) {
	wh := &WebhookHandler{digiflazzUser: "user", digiflazzAPIKey: "key", webhookRepo: &mockWebhookRepo{}, logger: testLogger}
	body := `{"sed":"AgXXtVAHp","hook_id":"11aaabbb","hook":{"url":"https://example.com/webhook","secret":"test","type":"application/json","status":1}}`
	req := httptest.NewRequest(http.MethodPost, "/webhook/digiflazz", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	wh.Digiflazz(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected status %d, got %d", http.StatusOK, w.Code)
	}
}

func TestWebhookHandler_Digiflazz_NonSuccessStatus(t *testing.T) {
	wh := &WebhookHandler{
		digiflazzUser:   "user",
		digiflazzAPIKey: "key",
		paymentSvc:      &mockPaymentService{getOrderResult: &models.Order{ID: "test-123", Status: "processing"}},
		notifySvc:       &mockNotifyService{},
		webhookRepo:     &mockWebhookRepo{},
		logger:          testLogger,
	}
	body := `{"data":{"ref_id":"test-123","status":"Gagal","sn":"","message":"Gagal"}}`
	req := httptest.NewRequest(http.MethodPost, "/webhook/digiflazz", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	wh.Digiflazz(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected status %d, got %d", http.StatusOK, w.Code)
	}
}

func TestWebhookHandler_Digiflazz_OrderNotFound(t *testing.T) {
	wh := &WebhookHandler{
		digiflazzUser:   "user",
		digiflazzAPIKey: "key",
		paymentSvc:      &mockPaymentService{getOrderErr: errors.New("not found")},
		webhookRepo:     &mockWebhookRepo{},
		logger:          testLogger,
	}
	body := `{"data":{"ref_id":"test-123","status":"Sukses","sn":"SN123","message":"Sukses"}}`
	req := httptest.NewRequest(http.MethodPost, "/webhook/digiflazz", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	wh.Digiflazz(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected status %d, got %d", http.StatusOK, w.Code)
	}
}

func TestWebhookHandler_Digiflazz_VerifySignature(t *testing.T) {
	wh := &WebhookHandler{digiflazzWebhookSecret: "mysecret", logger: testLogger}
	rawBody := []byte(`{"data":{"ref_id":"test-123","status":"Sukses"}}`)

	mac := hmac.New(sha1.New, []byte("mysecret"))
	mac.Write(rawBody)
	expectedSig := "sha1=" + hex.EncodeToString(mac.Sum(nil))

	if !wh.verifyDigiflazzSignature(rawBody, expectedSig) {
		t.Error("expected valid signature to pass")
	}

	if wh.verifyDigiflazzSignature(rawBody, "sha1=invalid") {
		t.Error("expected invalid signature to fail")
	}

	if wh.verifyDigiflazzSignature(rawBody, "") {
		t.Error("expected empty signature to fail")
	}
}

func TestAPIResponseEnvelope(t *testing.T) {
	h := &OrderHandler{
		paymentSvc: &mockPaymentService{getOrderResult: &models.Order{ID: "test-1"}},
	}
	req := httptest.NewRequest(http.MethodGet, "/api/orders/test-1", nil)
	w := httptest.NewRecorder()

	h.GetOrder(w, req)

	var resp map[string]any
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}

	if resp["success"] != true {
		t.Errorf("expected success=true, got %v", resp["success"])
	}
	if resp["data"] == nil {
		t.Error("expected data field")
	}
	if resp["request_id"] == "" {
		t.Error("expected request_id field")
	}
}
