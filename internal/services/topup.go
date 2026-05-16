package services

import (
	"bytes"
	"context"
	"crypto/md5" //nolint:gosec // required by Digiflazz API for signature generation
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"sync/atomic"
	"time"

	"github.com/topup-store/internal/constants"
	"github.com/topup-store/internal/models"
	"github.com/topup-store/internal/repositories"
)

type TopupService struct {
	orderRepo       repositories.OrderRepository
	productRepo     repositories.ProductRepository
	digiflazzUser   string
	digiflazzAPIKey string
	digiflazzURL    string
	digiflazzTest   bool
	httpClient      *http.Client
	logger          *slog.Logger
	balance         atomic.Int64
}

func NewTopupService(orderRepo repositories.OrderRepository, productRepo repositories.ProductRepository, user, apiKey, digiflazzURL string, testing bool, logger *slog.Logger) *TopupService {
	if logger == nil {
		logger = slog.New(slog.NewTextHandler(io.Discard, nil))
	}

	return &TopupService{
		orderRepo:       orderRepo,
		productRepo:     productRepo,
		digiflazzUser:   strings.TrimSpace(user),
		digiflazzAPIKey: strings.TrimSpace(apiKey),
		digiflazzURL:    strings.TrimSpace(digiflazzURL),
		digiflazzTest:   testing,
		httpClient: &http.Client{
			Timeout: 30 * time.Second,
		},
		logger: logger,
	}
}

func (s *TopupService) ProcessOrder(ctx context.Context, orderID string) error {

	updated, err := s.orderRepo.UpdateStatusIf(ctx, orderID, constants.StatusProcessing, constants.StatusPaid)
	if err != nil {
		return fmt.Errorf("atomic status transition: %w", err)
	}
	if !updated {
		return fmt.Errorf("order %s was already being processed or is not in paid status", orderID)
	}

	s.orderRepo.InsertStatusHistory(ctx, orderID, constants.StatusPaid, constants.StatusProcessing, "auto process after payment")

	order, err := s.orderRepo.GetByID(ctx, orderID)
	if err != nil {
		if err2 := s.orderRepo.UpdateStatus(ctx, orderID, constants.StatusFailed); err2 != nil {
			s.logger.Error("failed to mark order as failed", slog.String("order_id", orderID), slog.String("error", err2.Error()))
		}
		s.orderRepo.InsertStatusHistory(ctx, orderID, constants.StatusProcessing, constants.StatusFailed, "order not found")
		return fmt.Errorf("fetch order %s: %w", orderID, err)
	}

	product, err := s.productRepo.GetByID(ctx, order.ProductID)
	if err != nil {
		if err2 := s.orderRepo.UpdateStatus(ctx, orderID, constants.StatusFailed); err2 != nil {
			s.logger.Error("failed to mark order as failed", slog.String("order_id", orderID), slog.String("error", err2.Error()))
		}
		s.orderRepo.InsertStatusHistory(ctx, orderID, constants.StatusProcessing, constants.StatusFailed, "product not found")
		return fmt.Errorf("fetch product %s: %w", order.ProductID, err)
	}

	result, err := s.processTopupViaDigiflazz(ctx, order, product)
	if err != nil {
		current, getErr := s.orderRepo.GetByID(ctx, orderID)
		if getErr == nil && current.Status == constants.StatusSuccess {
			return nil
		}
		if strings.Contains(err.Error(), "rc=41") {
			s.logger.Warn("digiflazz topup returned rc=41; leaving order in processing state for webhook/poller resolution",
				slog.String("order_id", orderID),
				slog.String("error", err.Error()),
			)
			s.orderRepo.InsertStatusHistory(ctx, orderID, constants.StatusProcessing, constants.StatusProcessing, err.Error())
			return nil
		}
		s.orderRepo.InsertStatusHistory(ctx, orderID, constants.StatusProcessing, constants.StatusProcessing, err.Error())
		return fmt.Errorf("digiflazz topup: %w", err)
	}

	switch normalizeDigiflazzStatus(result.Status) {
	case constants.StatusSuccess:
		if result.SN != "" {
			if err := s.orderRepo.UpdateSerialNumber(ctx, orderID, result.SN); err != nil {
				return fmt.Errorf("update serial number: %w", err)
			}
		}
		updated, err := s.orderRepo.UpdateStatusIf(ctx, orderID, constants.StatusSuccess, constants.StatusProcessing)
		if err != nil {
			return fmt.Errorf("mark success: %w", err)
		}
		if updated {
			s.orderRepo.InsertStatusHistory(ctx, orderID, constants.StatusProcessing, constants.StatusSuccess, "digiflazz topup completed")
		}
	case constants.StatusFailed:
		reason := result.Message
		if reason == "" {
			reason = "digiflazz transaction failed"
		}
		updated, err := s.orderRepo.UpdateStatusIf(ctx, orderID, constants.StatusFailed, constants.StatusProcessing)
		if err != nil {
			return fmt.Errorf("mark failed: %w", err)
		}
		if updated {
			s.orderRepo.InsertStatusHistory(ctx, orderID, constants.StatusProcessing, constants.StatusFailed, reason)
		}
		return fmt.Errorf("digiflazz transaction failed: %s", reason)
	case constants.StatusProcessing:
		s.logger.Info("digiflazz topup pending", slog.String("order_id", orderID))
	default:
		s.logger.Warn("digiflazz topup returned unknown status",
			slog.String("order_id", orderID),
			slog.String("status", result.Status),
		)
	}
	return nil
}

type digiflazzTopupData struct {
	Status         string `json:"status"`
	Message        string `json:"message"`
	SN             string `json:"sn"`
	BuyerLastSaldo int    `json:"buyer_last_saldo"`
	Price          int    `json:"price"`
}

type CustomerValidationResult struct {
	Checked bool   `json:"checked"`
	Valid   bool   `json:"valid"`
	Message string `json:"message,omitempty"`
	SN      string `json:"sn,omitempty"`
}

func (s *TopupService) processTopupViaDigiflazz(ctx context.Context, order *models.Order, product *models.Product) (*digiflazzTopupData, error) {
	customerNo := s.buildCustomerNo(order, product)
	refID := order.DigiflazzRefID
	if refID == "" {
		refID = order.ID
	}
	sign, raw := s.generateSignDebug(refID)

	s.logger.Info("digiflazz topup request",
		slog.String("order_id", order.ID),
		slog.String("ref_id", refID),
		slog.String("sku", product.SKU),
		slog.String("customer_no", customerNo),
		slog.String("api_url", s.digiflazzURL),
		slog.Bool("testing", s.digiflazzTest),
		slog.Int("username_len", len(s.digiflazzUser)),
		slog.Int("api_key_len", len(s.digiflazzAPIKey)),
		slog.String("sign", sign),
		slog.String("sign_raw", raw),
	)

	payload := map[string]any{
		"username":       s.digiflazzUser,
		"buyer_sku_code": product.SKU,
		"customer_no":    customerNo,
		"ref_id":         refID,
		"sign":           sign,
		"testing":        s.digiflazzTest,
	}

	body, err := json.Marshal(payload)
	if err != nil {
		return nil, fmt.Errorf("marshal payload: %w", err)
	}

	var result struct {
		Data digiflazzTopupData `json:"data"`
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, s.digiflazzURL, bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := s.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("send request: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read response: %w", err)
	}

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		var errBody struct {
			Data struct {
				RC      string `json:"rc"`
				Message string `json:"message"`
			} `json:"data"`
		}
		_ = json.Unmarshal(respBody, &errBody)
		if errBody.Data.RC == "41" {
			s.logger.Warn("digiflazz signature mismatch (rc=41) — transaction may still be processed by Digiflazz backend; leaving order in processing state for webhook/poller to resolve",
				slog.String("order_id", order.ID),
				slog.String("ref_id", refID),
				slog.String("message", errBody.Data.Message),
			)
			return nil, fmt.Errorf("digiflazz signature mismatch (rc=41): %s", errBody.Data.Message)
		}
		return nil, fmt.Errorf("digiflazz returned status %d: %s", resp.StatusCode, string(respBody))
	}

	if err := json.Unmarshal(respBody, &result); err != nil {
		return nil, fmt.Errorf("parse response: %w, body: %s", err, string(respBody))
	}

	if result.Data.Status == constants.StatusSuccess {
		s.logger.Info("digiflazz topup success",
			slog.String("order_id", order.ID),
			slog.String("sn", result.Data.SN),
			slog.Int("balance", result.Data.BuyerLastSaldo),
			slog.Int("price", result.Data.Price),
		)
		if result.Data.BuyerLastSaldo > 0 {
			s.balance.Store(int64(result.Data.BuyerLastSaldo))
		}
	}
	return &result.Data, nil
}

func (s *TopupService) ValidateCustomer(ctx context.Context, game, gameUID, gameServer string) (*CustomerValidationResult, error) {
	product, err := s.getValidationProduct(ctx, game)
	if err != nil {
		return nil, err
	}
	if product == nil {
		return &CustomerValidationResult{Checked: false, Valid: true}, nil
	}

	order := &models.Order{
		ID:             "validation-" + strconv.FormatInt(time.Now().UnixNano(), 10),
		GameUID:        gameUID,
		GameServer:     gameServer,
		DigiflazzRefID: "validation-" + strconv.FormatInt(time.Now().UnixNano(), 10),
	}
	result, err := s.processTopupViaDigiflazz(ctx, order, product)
	if err != nil {
		return nil, fmt.Errorf("validate customer: %w", err)
	}

	normalizedStatus := normalizeDigiflazzStatus(result.Status)
	validation := &CustomerValidationResult{
		Checked: true,
		Valid:   normalizedStatus != constants.StatusFailed,
		Message: result.Message,
		SN:      result.SN,
	}
	if !validation.Valid {
		if validation.Message == "" {
			validation.Message = "UID atau server tidak valid"
		}
		return validation, fmt.Errorf("%s", validation.Message)
	}
	return validation, nil
}

func (s *TopupService) getValidationProduct(ctx context.Context, game string) (*models.Product, error) {
	products, err := s.productRepo.ListAll(ctx)
	if err != nil {
		return nil, fmt.Errorf("list validation products: %w", err)
	}
	for _, product := range products {
		if product.Game == game && product.ProductType == constants.ProductTypeValidation && product.IsActive && product.DeletedAt == nil {
			return &product, nil
		}
	}
	return nil, nil
}

func (s *TopupService) buildCustomerNo(order *models.Order, product *models.Product) string {
	if order.GameServer == "" {
		return order.GameUID
	}

	switch product.CustomerNoFormat {
	case "uid_server_concat":
		return order.GameUID + order.GameServer
	case "uid_server_space":
		return order.GameUID + " " + order.GameServer
	case "uid_server_pipe":
		return order.GameUID + "|" + order.GameServer
	case "uid":
		return order.GameUID
	}

	if constants.PipeServerCustomerNoGames[product.Game] {
		return order.GameUID + "|" + order.GameServer
	}
	if product.Game == constants.GameMobileLegends {
		return order.GameUID + order.GameServer
	}
	return order.GameUID
}

func (s *TopupService) generateSign(refID string) string {
	raw := s.digiflazzUser + s.digiflazzAPIKey + refID
	hash := md5.Sum([]byte(raw))
	return hex.EncodeToString(hash[:])
}

// generateSignDebug returns the sign plus the raw concatenated string for diagnostics.
func (s *TopupService) generateSignDebug(refID string) (sign, raw string) {
	raw = s.digiflazzUser + s.digiflazzAPIKey + refID
	hash := md5.Sum([]byte(raw))
	sign = hex.EncodeToString(hash[:])
	return sign, raw
}

func normalizeDigiflazzStatus(status string) string {
	switch strings.ToLower(status) {
	case "sukses", constants.StatusSuccess:
		return constants.StatusSuccess
	case "pending":
		return constants.StatusProcessing
	case "gagal", "fail", constants.StatusFailed:
		return constants.StatusFailed
	default:
		return status
	}
}

func signPrefix(sign string) string {
	if len(sign) <= 8 {
		return sign
	}
	return sign[:8]
}

func (s *TopupService) CheckTransactionStatus(orderID string) (status string, sn string, err error) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	order, err := s.orderRepo.GetByID(ctx, orderID)
	if err != nil {
		return "", "", fmt.Errorf("fetch order: %w", err)
	}

	product, err := s.productRepo.GetByID(ctx, order.ProductID)
	if err != nil {
		return "", "", fmt.Errorf("fetch product: %w", err)
	}

	customerNo := s.buildCustomerNo(order, product)
	refID := order.DigiflazzRefID
	if refID == "" {
		refID = orderID
	}
	sign := s.generateSign(refID)

	payload := map[string]any{
		"username":       s.digiflazzUser,
		"buyer_sku_code": product.SKU,
		"customer_no":    customerNo,
		"ref_id":         refID,
		"sign":           sign,
		"testing":        s.digiflazzTest,
	}

	body, err := json.Marshal(payload)
	if err != nil {
		return "", "", fmt.Errorf("marshal payload: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, s.digiflazzURL, bytes.NewReader(body))
	if err != nil {
		return "", "", fmt.Errorf("create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := s.httpClient.Do(req)
	if err != nil {
		return "", "", fmt.Errorf("send request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		respBody, _ := io.ReadAll(resp.Body)
		return "", "", fmt.Errorf("digiflazz returned status %d: %s", resp.StatusCode, string(respBody))
	}

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", "", fmt.Errorf("read response: %w", err)
	}

	var result struct {
		Data struct {
			Status         string `json:"status"`
			Message        string `json:"message"`
			SN             string `json:"sn"`
			BuyerLastSaldo int    `json:"buyer_last_saldo"`
		} `json:"data"`
	}
	if err := json.Unmarshal(respBody, &result); err != nil {
		return "", "", fmt.Errorf("parse response: %w, body: %s", err, string(respBody))
	}

	if result.Data.BuyerLastSaldo > 0 {
		s.balance.Store(int64(result.Data.BuyerLastSaldo))
	}

	return result.Data.Status, result.Data.SN, nil
}

func (s *TopupService) GetProcessingOrders(ctx context.Context) ([]models.Order, error) {
	return s.orderRepo.ListProcessing(ctx)
}

func (s *TopupService) GetProduct(ctx context.Context, id string) (*models.Product, error) {
	return s.productRepo.GetByID(ctx, id)
}

func (s *TopupService) GetProductByGameAndDiamonds(ctx context.Context, game string, itemQty int) (*models.Product, error) {
	return s.productRepo.GetByGameAndDiamonds(ctx, game, itemQty)
}

func (s *TopupService) ListProducts(ctx context.Context, game string) ([]models.Product, error) {
	return s.productRepo.ListByGame(ctx, game)
}

func (s *TopupService) ListAllProducts(ctx context.Context) ([]models.Product, error) {
	products, err := s.productRepo.ListAll(ctx)
	if err != nil {
		return nil, err
	}
	publicProducts := make([]models.Product, 0, len(products))
	for _, product := range products {
		if product.ProductType != constants.ProductTypeValidation {
			publicProducts = append(publicProducts, product)
		}
	}
	return publicProducts, nil
}

func (s *TopupService) DecrementProductStock(ctx context.Context, productID string) (bool, error) {
	return s.productRepo.DecrementStock(ctx, productID)
}

func (s *TopupService) IncrementProductStock(ctx context.Context, productID string) error {
	return s.productRepo.IncrementStock(ctx, productID)
}

type DigiflazzPrice struct {
	SKU         string
	Name        string
	Price       int
	Category    string
	Brand       string
	Description string
}

func (s *TopupService) FetchDigiflazzPrices(ctx context.Context) ([]DigiflazzPrice, error) {
	sign := s.generateSign("pricelist")

	payload := map[string]string{
		"username": s.digiflazzUser,
		"sign":     sign,
	}

	body, err := json.Marshal(payload)
	if err != nil {
		return nil, fmt.Errorf("marshal payload: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, s.digiflazzURL+"/../price-list", bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := s.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("send request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		respBody, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("digiflazz returned status %d: %s", resp.StatusCode, string(respBody))
	}

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read response: %w", err)
	}

	var rawResult struct {
		Data json.RawMessage `json:"data"`
	}
	if err := json.Unmarshal(respBody, &rawResult); err != nil {
		return nil, fmt.Errorf("parse response: %w, body: %s", err, string(respBody))
	}

	var errMsg struct {
		RC      string `json:"rc"`
		Message string `json:"message"`
	}
	if err := json.Unmarshal(rawResult.Data, &errMsg); err == nil && errMsg.RC != "" && errMsg.RC != "00" {
		return nil, fmt.Errorf("digiflazz error: %s", errMsg.Message)
	}

	var products []struct {
		SKU         string `json:"buyer_sku_code"`
		Name        string `json:"product_name"`
		Price       int    `json:"price"`
		Category    string `json:"category"`
		Brand       string `json:"brand"`
		Description string `json:"desc"`
	}
	if err := json.Unmarshal(rawResult.Data, &products); err != nil {
		return nil, fmt.Errorf("parse products: %w, body: %s", err, string(respBody))
	}

	var prices []DigiflazzPrice
	for _, p := range products {
		if p.SKU != "" && p.Price > 0 {
			prices = append(prices, DigiflazzPrice{
				SKU:         p.SKU,
				Name:        p.Name,
				Price:       p.Price,
				Category:    p.Category,
				Brand:       p.Brand,
				Description: p.Description,
			})
		}
	}

	return prices, nil
}

func (s *TopupService) GetBalance() int {
	return int(s.balance.Load())
}

func (s *TopupService) CompleteOrder(ctx context.Context, orderID, sn string) error {
	_, err := s.orderRepo.GetByID(ctx, orderID)
	if err != nil {
		return err
	}
	if sn != "" {
		if err := s.orderRepo.UpdateSerialNumber(ctx, orderID, sn); err != nil {
			return fmt.Errorf("update serial number: %w", err)
		}
	}
	return s.orderRepo.UpdateStatus(ctx, orderID, constants.StatusSuccess)
}

func (s *TopupService) FailOrder(ctx context.Context, orderID string) error {
	return s.orderRepo.UpdateStatus(ctx, orderID, constants.StatusFailed)
}

type SyncResult struct {
	SKU         string `json:"sku"`
	Name        string `json:"name"`
	Cost        int    `json:"cost"`
	Price       int    `json:"price"`
	Margin      int    `json:"margin"`
	Tier        string `json:"tier"`
	Created     bool   `json:"created"`
	Game        string `json:"game,omitempty"`
	ProductType string `json:"product_type,omitempty"`
	ItemQty     int    `json:"item_qty,omitempty"`
}

func (s *TopupService) SyncPricesWithAutoCreate(ctx context.Context, marginType string, marginValue int, recalculatePrices bool) ([]SyncResult, int, int, int, error) {
	if marginType == "" {
		marginType = "tiered"
	}

	prices, err := s.FetchDigiflazzPrices(ctx)
	if err != nil {
		return nil, 0, 0, 0, fmt.Errorf("fetch digiflazz prices: %w", err)
	}

	var results []SyncResult
	updated := 0
	created := 0
	skipped := 0
	currentProductsBySKU := map[string]models.Product{}
	if !recalculatePrices {
		currentProducts, err := s.productRepo.ListAll(ctx)
		if err != nil {
			return nil, 0, 0, 0, fmt.Errorf("list current products: %w", err)
		}
		for _, product := range currentProducts {
			currentProductsBySKU[product.SKU] = product
		}
	}

	for _, p := range prices {
		if p.Price <= 0 || p.SKU == "" {
			skipped++
			continue
		}

		costPrice := p.Price
		sellingPrice, tier := CalcTieredPrice(costPrice, marginType, marginValue)

		exists, err := s.productRepo.ExistsBySKU(ctx, p.SKU, "")
		if err != nil {
			s.logger.Warn("sync: check SKU existence failed", slog.String("sku", p.SKU), slog.String("error", err.Error()))
			skipped++
			continue
		}

		if !exists {
			game := inferGame(p.SKU, p.Name, p.Brand)
			if game == "" {
				s.logger.Debug("sync: skipping unknown game", slog.String("sku", p.SKU), slog.String("name", p.Name), slog.String("brand", p.Brand))
				skipped++
				continue
			}

			productType := detectProductType(p.Name)
			itemQty := extractItemQtyFromName(p.Name)
			if itemQty == 0 && productType != "diamond" {
				itemQty = 1
			}

			if err := s.productRepo.CreateFromDigiflazz(ctx, p.SKU, p.Name, game, productType, sellingPrice, costPrice, itemQty, p.Description); err != nil {
				s.logger.Warn("sync: failed to create product", slog.String("sku", p.SKU), slog.String("error", err.Error()))
				skipped++
				continue
			}

			created++
			results = append(results, SyncResult{
				SKU:         p.SKU,
				Name:        p.Name,
				Cost:        costPrice,
				Price:       sellingPrice,
				Margin:      sellingPrice - costPrice,
				Tier:        tier,
				Created:     true,
				Game:        game,
				ProductType: productType,
				ItemQty:     itemQty,
			})
			continue
		}

		if recalculatePrices {
			if err := s.productRepo.SyncPrice(ctx, p.SKU, costPrice, sellingPrice); err != nil {
				s.logger.Warn("sync: failed to update price", slog.String("sku", p.SKU), slog.String("error", err.Error()))
				skipped++
				continue
			}

			updated++
			results = append(results, SyncResult{
				SKU:    p.SKU,
				Name:   p.Name,
				Cost:   costPrice,
				Price:  sellingPrice,
				Margin: sellingPrice - costPrice,
				Tier:   tier,
			})
			continue
		}

		if err := s.productRepo.UpdateCostPrice(ctx, p.SKU, costPrice); err != nil {
			s.logger.Warn("sync: failed to update cost price", slog.String("sku", p.SKU), slog.String("error", err.Error()))
			skipped++
			continue
		}

		currentPrice := 0
		if product, ok := currentProductsBySKU[p.SKU]; ok {
			currentPrice = product.PriceIDR
		}
		updated++
		results = append(results, SyncResult{
			SKU:    p.SKU,
			Name:   p.Name,
			Cost:   costPrice,
			Price:  currentPrice,
			Margin: currentPrice - costPrice,
			Tier:   "manual_price_preserved",
		})
	}

	return results, updated, created, skipped, nil
}

func CalcTieredPrice(costPrice int, marginType string, marginValue int) (sellingPrice int, tier string) {
	if marginType == "fixed" {
		sellingPrice = costPrice + marginValue
		tier = "fixed"
	} else if marginType == "percent" {
		sellingPrice = int(float64(costPrice) * (1 + float64(marginValue)/100))
		tier = "flat_percent"
	} else {
		switch {
		case costPrice < 5000:
			sellingPrice = int(float64(costPrice) * 1.15)
			minPrice := costPrice + 200
			if sellingPrice < minPrice {
				sellingPrice = minPrice
			}
			tier = "low (<5k, 15% min 200)"
		case costPrice <= 50000:
			sellingPrice = int(float64(costPrice) * 1.10)
			tier = "mid (5k-50k, 10%)"
		default:
			sellingPrice = int(float64(costPrice) * 1.05)
			maxPrice := costPrice + 5000
			if sellingPrice > maxPrice {
				sellingPrice = maxPrice
			}
			tier = "high (>50k, 5% max 5k)"
		}
	}

	sellingPrice = (sellingPrice + 50) / 100 * 100
	return
}

var skuPrefixGame = map[string]string{
	"ff_":      constants.GameFreeFire,
	"ml_":      constants.GameMobileLegends,
	"pubg_":    constants.GamePUBGMobile,
	"gi_":      constants.GameGenshinImpact,
	"genshin_": constants.GameGenshinImpact,
	"hsr_":     constants.GameHonkaiStarRail,
	"zzz_":     constants.GameZenlessZoneZero,
	"hi3_":     constants.GameHonkaiImpact3,
}

func inferGame(sku, name, brand string) string {
	skuLower := strings.ToLower(sku)
	for prefix, game := range skuPrefixGame {
		if strings.HasPrefix(skuLower, prefix) {
			return game
		}
	}

	text := strings.ToLower(name + " " + brand)
	switch {
	case strings.Contains(text, "genshin"):
		return constants.GameGenshinImpact
	case strings.Contains(text, "star rail") || strings.Contains(text, "hsr"):
		return constants.GameHonkaiStarRail
	case strings.Contains(text, "zenless") || strings.Contains(text, "zzz"):
		return constants.GameZenlessZoneZero
	case strings.Contains(text, "honkai impact") || strings.Contains(text, "hi3"):
		return constants.GameHonkaiImpact3
	}
	return ""
}

var itemQuantityRegex = regexp.MustCompile(`(\d+)\s*(dm|diamonds?|uc|genesis\s*crystals?|crystals?|oneiric\s*shards?|shards?|monochromes?|polychromes?|lunites?|b-?chips?|chips?|stellar\s*jades?|jades?|d$)`)
var firstNumberRegex = regexp.MustCompile(`\d+`)

func extractItemQtyFromName(name string) int {
	nameLower := strings.ToLower(name)
	matches := itemQuantityRegex.FindStringSubmatch(nameLower)
	if len(matches) >= 2 {
		n, err := strconv.Atoi(matches[1])
		if err == nil {
			return n
		}
	}
	firstNumber := firstNumberRegex.FindString(nameLower)
	if firstNumber != "" {
		n, err := strconv.Atoi(firstNumber)
		if err == nil {
			return n
		}
	}
	return 0
}

var subscriptionKeywords = []string{"weekly", "monthly", "membership", "season pass", "diamond pass", "twilight pass", "starlight pass", "starlight", "welkin", "express supply", "inter-knot membership", "monthly card"}
var validationKeywords = []string{"cek id", "check id", "validasi", "validation", "nickname"}

func detectProductType(name string) string {
	nameLower := strings.ToLower(name)
	for _, keyword := range validationKeywords {
		if strings.Contains(nameLower, keyword) {
			return constants.ProductTypeValidation
		}
	}
	for _, keyword := range subscriptionKeywords {
		if strings.Contains(nameLower, keyword) {
			return constants.ProductTypeSubscription
		}
	}
	return constants.ProductTypeDiamond
}
