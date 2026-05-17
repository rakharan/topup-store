package repositories

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/golang-migrate/migrate/v4"
	_ "github.com/golang-migrate/migrate/v4/database/postgres"
	_ "github.com/golang-migrate/migrate/v4/source/file"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/topup-store/internal/db"
	"github.com/topup-store/internal/models"
)

var testPool *pgxpool.Pool

func TestMain(m *testing.M) {
	dsn := os.Getenv("TEST_DATABASE_URL")
	if dsn == "" {
		dsn = "postgres://topup:topup@localhost:5432/topup_store_repo_test?sslmode=disable"
	}
	if !isTestDatabase(dsn) {
		fmt.Fprintf(os.Stderr, "REFUSING: repository tests require a test database, got %q\n", dsn)
		os.Exit(1)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	if err := ensureTestDatabase(ctx, dsn); err != nil {
		fmt.Fprintf(os.Stderr, "SKIP: cannot prepare test database: %v\n", err)
		os.Exit(0)
	}

	pool, err := db.NewPool(ctx, db.PoolConfig{
		DSN:      dsn,
		MaxConns: 5,
		MinConns: 1,
	})
	if err != nil {
		fmt.Fprintf(os.Stderr, "SKIP: cannot connect to test database: %v\n", err)
		os.Exit(0)
	}
	testPool = pool

	if err := migrateTestDatabase(dsn); err != nil {
		fmt.Fprintf(os.Stderr, "SKIP: cannot migrate test database: %v\n", err)
		pool.Close()
		os.Exit(0)
	}

	code := m.Run()
	pool.Close()
	os.Exit(code)
}

func isTestDatabase(dsn string) bool {
	return strings.Contains(strings.ToLower(databaseName(dsn)), "test")
}

func databaseName(dsn string) string {
	parsed, err := url.Parse(dsn)
	if err != nil {
		return dsn
	}
	return strings.Trim(parsed.Path, "/")
}

func ensureTestDatabase(ctx context.Context, dsn string) error {
	dbName := databaseName(dsn)
	parsed, err := url.Parse(dsn)
	if err != nil {
		return err
	}
	parsed.Path = "/postgres"
	adminPool, err := pgxpool.New(ctx, parsed.String())
	if err != nil {
		return err
	}
	defer adminPool.Close()
	_, err = adminPool.Exec(ctx, fmt.Sprintf("CREATE DATABASE %s", dbName))
	if err != nil && !strings.Contains(err.Error(), "already exists") {
		return err
	}
	return nil
}

func migrateTestDatabase(dsn string) error {
	_, filename, _, _ := runtime.Caller(0)
	migrationsPath := filepath.Join(filepath.Dir(filename), "..", "..", "migrations")
	m, err := migrate.New("file://"+filepath.ToSlash(migrationsPath), dsn)
	if err != nil {
		return err
	}
	if err := m.Up(); err != nil && err != migrate.ErrNoChange {
		return err
	}
	return nil
}

func withCleanTables(t *testing.T, fn func(ctx context.Context)) {
	t.Helper()
	ctx := context.Background()
	cleanTables(t, ctx)
	fn(ctx)
}

func orderRepo() *PGOrderRepository {
	return &PGOrderRepository{pool: testPool}
}

func productRepo() *PGProductRepository {
	return &PGProductRepository{pool: testPool}
}

func webhookRepo() *PGWebhookRepository {
	return &PGWebhookRepository{pool: testPool}
}

func cleanTables(t *testing.T, ctx context.Context) {
	t.Helper()
	tables := []string{
		"referral_point_ledger",
		"order_referrals",
		"referral_codes",
		"coupons",
		"order_status_history",
		"order_qris",
		"orders",
		"products",
		"webhooks_log",
	}
	for _, table := range tables {
		if _, err := testPool.Exec(ctx, fmt.Sprintf("TRUNCATE TABLE %s CASCADE", table)); err != nil {
			t.Fatalf("clean table %s: %v", table, err)
		}
	}
}

func createTestProduct(t *testing.T, ctx context.Context, game string, itemQty int) string {
	t.Helper()
	repo := &PGProductRepository{pool: testPool}
	id := uuid.New().String()
	product := &models.Product{
		ID:           id,
		Game:         game,
		Name:         fmt.Sprintf("Test %d Diamonds", itemQty),
		Description:  "test product",
		PriceIDR:     itemQty * 100,
		CostPriceIDR: itemQty * 80,
		ItemQty:      itemQty,
		ProductType:  "diamond",
		SKU:          fmt.Sprintf("test_%s_%d", game, itemQty),
		IsActive:     true,
		Stock:        -1,
	}
	if err := repo.Create(ctx, product); err != nil {
		t.Fatalf("create test product: %v", err)
	}
	// Verify product was actually created
	var count int
	if err := testPool.QueryRow(ctx, "SELECT COUNT(*) FROM products WHERE id = $1", id).Scan(&count); err != nil {
		t.Fatalf("verify test product: %v", err)
	}
	if count == 0 {
		t.Fatalf("test product was not found in database after creation")
	}
	return id
}

func TestOrderRepository_CreateAndGet(t *testing.T) {
	withCleanTables(t, func(ctx context.Context) {
		repo := orderRepo()

		productID := createTestProduct(t, ctx, "free_fire", 100)
		order := &models.Order{
			ID:             uuid.New().String(),
			OrderNumber:    "FT-20240101-0001",
			ProductID:      productID,
			UserPhone:      "6281234567890",
			GameUID:        "12345678",
			GameServer:     "",
			AmountIDR:      10000,
			Status:         "pending",
			Channel:        "web",
			DigiflazzRefID: "ref-123",
		}

		if err := repo.Create(ctx, order); err != nil {
			t.Fatalf("create order: %v", err)
		}

		got, err := repo.GetByID(ctx, order.ID)
		if err != nil {
			t.Fatalf("get by id: %v", err)
		}
		if got.OrderNumber != order.OrderNumber {
			t.Errorf("order number = %s; want %s", got.OrderNumber, order.OrderNumber)
		}
		if got.Status != "pending" {
			t.Errorf("status = %s; want pending", got.Status)
		}
	})
}

func TestOrderRepository_GetByOrderNumber(t *testing.T) {
	withCleanTables(t, func(ctx context.Context) {
		repo := orderRepo()

		productID := createTestProduct(t, ctx, "free_fire", 50)
		order := &models.Order{
			ID:          uuid.New().String(),
			OrderNumber: "FT-20240101-0002",
			ProductID:   productID,
			UserPhone:   "6281234567890",
			GameUID:     "12345678",
			AmountIDR:   5000,
			Channel:     "web",
		}
		if err := repo.Create(ctx, order); err != nil {
			t.Fatalf("create: %v", err)
		}

		got, err := repo.GetByOrderNumber(ctx, order.OrderNumber)
		if err != nil {
			t.Fatalf("get by order number: %v", err)
		}
		if got.ID != order.ID {
			t.Errorf("id = %s; want %s", got.ID, order.ID)
		}
	})
}

func TestOrderRepository_UpdateStatus(t *testing.T) {
	withCleanTables(t, func(ctx context.Context) {
		repo := orderRepo()

		productID := createTestProduct(t, ctx, "free_fire", 50)
		order := &models.Order{
			ID:          uuid.New().String(),
			OrderNumber: "FT-20240101-0003",
			ProductID:   productID,
			UserPhone:   "6281234567890",
			GameUID:     "12345678",
			AmountIDR:   5000,
			Channel:     "web",
		}
		if err := repo.Create(ctx, order); err != nil {
			t.Fatalf("create: %v", err)
		}

		if err := repo.UpdateStatus(ctx, order.ID, "paid"); err != nil {
			t.Fatalf("update status: %v", err)
		}

		got, err := repo.GetByID(ctx, order.ID)
		if err != nil {
			t.Fatalf("get after update: %v", err)
		}
		if got.Status != "paid" {
			t.Errorf("status = %s; want paid", got.Status)
		}
	})
}

func TestOrderRepository_ListIncludesExpired(t *testing.T) {
	withCleanTables(t, func(ctx context.Context) {
		repo := orderRepo()

		productID := createTestProduct(t, ctx, "free_fire", 70)
		order := &models.Order{
			ID:          uuid.New().String(),
			OrderNumber: "FT-20240101-0006",
			ProductID:   productID,
			UserPhone:   "6281234567890",
			GameUID:     "12345678",
			AmountIDR:   7000,
			Channel:     "web",
		}
		if err := repo.Create(ctx, order); err != nil {
			t.Fatalf("create: %v", err)
		}
		if err := repo.UpdateStatus(ctx, order.ID, "expired"); err != nil {
			t.Fatalf("update status: %v", err)
		}

		orders, total, err := repo.List(ctx, 1, 10)
		if err != nil {
			t.Fatalf("list: %v", err)
		}
		if total != 1 {
			t.Fatalf("total = %d; want 1", total)
		}
		if len(orders) != 1 {
			t.Fatalf("len = %d; want 1", len(orders))
		}
		if orders[0].Status != "expired" {
			t.Errorf("status = %s; want expired", orders[0].Status)
		}
	})
}

func TestOrderRepository_ExpireOldPendingScansReturnedOrder(t *testing.T) {
	withCleanTables(t, func(ctx context.Context) {
		repo := orderRepo()

		productID := createTestProduct(t, ctx, "free_fire", 80)
		order := &models.Order{
			ID:             uuid.New().String(),
			OrderNumber:    "FT-20240101-0007",
			ProductID:      productID,
			UserPhone:      "6281234567890",
			GameUID:        "12345678",
			AmountIDR:      8000,
			Channel:        "web",
			StockReserved:  true,
			DigiflazzRefID: "ref-expire",
		}
		if err := repo.Create(ctx, order); err != nil {
			t.Fatalf("create: %v", err)
		}
		if _, err := testPool.Exec(ctx, "UPDATE orders SET created_at = NOW() - INTERVAL '31 minutes' WHERE id = $1", order.ID); err != nil {
			t.Fatalf("age order: %v", err)
		}

		expired, err := repo.ExpireOldPending(ctx)
		if err != nil {
			t.Fatalf("expire old pending: %v", err)
		}
		if len(expired) != 1 {
			t.Fatalf("expired len = %d; want 1", len(expired))
		}
		if expired[0].Status != "expired" {
			t.Errorf("status = %s; want expired", expired[0].Status)
		}
		if !expired[0].StockReserved {
			t.Error("stock_reserved = false; want true")
		}
	})
}

func TestOrderRepository_UpdateStatusIf(t *testing.T) {
	withCleanTables(t, func(ctx context.Context) {
		repo := orderRepo()

		productID := createTestProduct(t, ctx, "free_fire", 50)
		order := &models.Order{
			ID:          uuid.New().String(),
			OrderNumber: "FT-20240101-0004",
			ProductID:   productID,
			UserPhone:   "6281234567890",
			GameUID:     "12345678",
			AmountIDR:   5000,
			Channel:     "web",
		}
		if err := repo.Create(ctx, order); err != nil {
			t.Fatalf("create: %v", err)
		}

		// Transition from pending -> paid should succeed
		ok, err := repo.UpdateStatusIf(ctx, order.ID, "paid", "pending")
		if err != nil {
			t.Fatalf("update status if: %v", err)
		}
		if !ok {
			t.Error("expected update to succeed")
		}

		// Second transition from pending -> paid should fail
		ok, err = repo.UpdateStatusIf(ctx, order.ID, "paid", "pending")
		if err != nil {
			t.Fatalf("update status if second time: %v", err)
		}
		if ok {
			t.Error("expected update to fail")
		}
	})
}

func TestOrderRepository_AwardsReferralPointsOnSuccess(t *testing.T) {
	withCleanTables(t, func(ctx context.Context) {
		repo := orderRepo()
		refRepo := NewReferralCodeRepository(testPool)

		productID := createTestProduct(t, ctx, "mobile_legends", 50)
		referral := &models.ReferralCode{
			Code:         "REFTEST",
			OwnerPhone:   "628111111111",
			DiscountIDR:  500,
			RewardPoints: 700,
			IsActive:     true,
		}
		if err := refRepo.Create(ctx, referral); err != nil {
			t.Fatalf("create referral code: %v", err)
		}

		order := &models.Order{
			ID:          uuid.New().String(),
			OrderNumber: "FT-20240101-0099",
			ProductID:   productID,
			UserPhone:   "628222222222",
			GameUID:     "12345678",
			GameServer:  "1234",
			AmountIDR:   5000,
			Channel:     "web",
		}
		if err := repo.Create(ctx, order); err != nil {
			t.Fatalf("create order: %v", err)
		}
		if err := refRepo.ApplyToOrder(ctx, order.ID, referral.ID, 500); err != nil {
			t.Fatalf("apply referral to order: %v", err)
		}
		if err := repo.UpdateStatus(ctx, order.ID, "success"); err != nil {
			t.Fatalf("mark success: %v", err)
		}
		if err := repo.UpdateStatus(ctx, order.ID, "success"); err != nil {
			t.Fatalf("mark success twice: %v", err)
		}

		balances, err := refRepo.ListPointBalances(ctx)
		if err != nil {
			t.Fatalf("list balances: %v", err)
		}
		if len(balances) != 1 {
			t.Fatalf("balances len = %d; want 1", len(balances))
		}
		if balances[0].OwnerPhone != referral.OwnerPhone {
			t.Fatalf("owner phone = %s; want %s", balances[0].OwnerPhone, referral.OwnerPhone)
		}
		if balances[0].Points != 700 {
			t.Fatalf("points = %d; want 700", balances[0].Points)
		}
	})
}

func TestOrderRepository_StatusHistory(t *testing.T) {
	withCleanTables(t, func(ctx context.Context) {
		repo := orderRepo()

		productID := createTestProduct(t, ctx, "free_fire", 50)
		order := &models.Order{
			ID:          uuid.New().String(),
			OrderNumber: "FT-20240101-0005",
			ProductID:   productID,
			UserPhone:   "6281234567890",
			GameUID:     "12345678",
			AmountIDR:   5000,
			Channel:     "web",
		}
		if err := repo.Create(ctx, order); err != nil {
			t.Fatalf("create: %v", err)
		}

		fromStatus := "pending"
		if err := repo.InsertStatusHistory(ctx, order.ID, fromStatus, "paid", "payment received"); err != nil {
			t.Fatalf("insert status history: %v", err)
		}

		history, err := repo.GetStatusHistory(ctx, order.ID)
		if err != nil {
			t.Fatalf("get status history: %v", err)
		}
		if len(history) != 1 {
			t.Fatalf("history len = %d; want 1", len(history))
		}
		if history[0].ToStatus != "paid" {
			t.Errorf("to_status = %s; want paid", history[0].ToStatus)
		}
		if history[0].Reason == nil || *history[0].Reason != "payment received" {
			reason := "<nil>"
			if history[0].Reason != nil {
				reason = *history[0].Reason
			}
			t.Errorf("reason = %s; want payment received", reason)
		}
	})
}

func TestOrderRepository_GetOverallStatsNoSuccessOrders(t *testing.T) {
	withCleanTables(t, func(ctx context.Context) {
		repo := orderRepo()

		productID := createTestProduct(t, ctx, "free_fire", 90)
		order := &models.Order{
			ID:          uuid.New().String(),
			OrderNumber: "FT-20240101-0008",
			ProductID:   productID,
			UserPhone:   "6281234567890",
			GameUID:     "12345678",
			AmountIDR:   9000,
			Channel:     "web",
		}
		if err := repo.Create(ctx, order); err != nil {
			t.Fatalf("create: %v", err)
		}

		stats, err := repo.GetOverallStats(ctx, time.Now().Add(-time.Hour), time.Now().Add(time.Hour))
		if err != nil {
			t.Fatalf("overall stats: %v", err)
		}
		if stats.TotalOrders != 1 {
			t.Fatalf("total_orders = %d; want 1", stats.TotalOrders)
		}
		if stats.AvgOrderValue != 0 {
			t.Errorf("avg_order_value = %v; want 0", stats.AvgOrderValue)
		}
		if _, err := json.Marshal(stats); err != nil {
			t.Fatalf("marshal stats: %v", err)
		}
	})
}

func TestOrderRepository_AnalyticsUseNetProfit(t *testing.T) {
	withCleanTables(t, func(ctx context.Context) {
		repo := orderRepo()

		productID := createTestProduct(t, ctx, "free_fire", 100)
		successOrder := &models.Order{
			ID:             uuid.New().String(),
			OrderNumber:    "FT-20240101-0009",
			ProductID:      productID,
			UserPhone:      "6281234567890",
			GameUID:        "12345678",
			AmountIDR:      11000,
			Channel:        "web",
			DigiflazzRefID: "ref-success-net-profit",
		}
		if err := repo.Create(ctx, successOrder); err != nil {
			t.Fatalf("create success order: %v", err)
		}
		if err := repo.UpdateStatus(ctx, successOrder.ID, "success"); err != nil {
			t.Fatalf("mark success: %v", err)
		}

		failedOrder := &models.Order{
			ID:             uuid.New().String(),
			OrderNumber:    "FT-20240101-0010",
			ProductID:      productID,
			UserPhone:      "6281234567890",
			GameUID:        "87654321",
			AmountIDR:      999000,
			Channel:        "web",
			DigiflazzRefID: "ref-failed-net-profit",
		}
		if err := repo.Create(ctx, failedOrder); err != nil {
			t.Fatalf("create failed order: %v", err)
		}
		if err := repo.UpdateStatus(ctx, failedOrder.ID, "failed"); err != nil {
			t.Fatalf("mark failed: %v", err)
		}

		start := time.Now().Add(-time.Hour)
		end := time.Now().Add(time.Hour)

		stats, err := repo.GetOverallStats(ctx, start, end)
		if err != nil {
			t.Fatalf("overall stats: %v", err)
		}
		if stats.TotalRevenue != 2780 {
			t.Fatalf("total_revenue = %d; want 2780", stats.TotalRevenue)
		}
		if stats.TotalMidtransFee != 220 {
			t.Fatalf("total_midtrans_fee = %d; want 220", stats.TotalMidtransFee)
		}
		if stats.AvgOrderValue != 2780 {
			t.Fatalf("avg_order_value = %v; want 2780", stats.AvgOrderValue)
		}

		daily, err := repo.GetDailyRevenue(ctx, start, end)
		if err != nil {
			t.Fatalf("daily revenue: %v", err)
		}
		if len(daily) != 1 {
			t.Fatalf("daily len = %d; want 1", len(daily))
		}
		if daily[0].Orders != 1 || daily[0].Revenue != 2780 {
			t.Fatalf("daily = orders %d revenue %d; want orders 1 revenue 2780", daily[0].Orders, daily[0].Revenue)
		}

		games, err := repo.GetTopGamesByRevenue(ctx, start, end)
		if err != nil {
			t.Fatalf("top games: %v", err)
		}
		if len(games) != 1 {
			t.Fatalf("games len = %d; want 1", len(games))
		}
		if games[0].Game != "free_fire" || games[0].Orders != 1 || games[0].Revenue != 2780 {
			t.Fatalf("game stats = %+v; want free_fire orders 1 revenue 2780", games[0])
		}
	})
}

func TestProductRepository_CreateAndGet(t *testing.T) {
	withCleanTables(t, func(ctx context.Context) {
		repo := productRepo()

		product := &models.Product{
			ID:               uuid.New().String(),
			Game:             "free_fire",
			Name:             "100 Diamonds",
			Description:      "Test product",
			PriceIDR:         15000,
			CostPriceIDR:     10000,
			ItemQty:          100,
			ProductType:      "diamond",
			SKU:              "ff_100",
			CustomerNoFormat: "uid_server_concat",
			IsActive:         true,
			Stock:            -1,
		}

		if err := repo.Create(ctx, product); err != nil {
			t.Fatalf("create product: %v", err)
		}

		got, err := repo.GetByID(ctx, product.ID)
		if err != nil {
			t.Fatalf("get by id: %v", err)
		}
		if got.Name != product.Name {
			t.Errorf("name = %s; want %s", got.Name, product.Name)
		}
		if got.SKU != product.SKU {
			t.Errorf("sku = %s; want %s", got.SKU, product.SKU)
		}
		if got.CustomerNoFormat != product.CustomerNoFormat {
			t.Errorf("customer_no_format = %s; want %s", got.CustomerNoFormat, product.CustomerNoFormat)
		}
	})
}

func TestProductRepository_CreateFromDigiflazzHoyoUsesPipeFormat(t *testing.T) {
	withCleanTables(t, func(ctx context.Context) {
		repo := productRepo()

		err := repo.CreateFromDigiflazz(ctx, "gi_60", "Genshin Impact 60 Genesis Crystals", "genshin_impact", "diamond", 12000, 10000, 60, "test")
		if err != nil {
			t.Fatalf("create from digiflazz: %v", err)
		}

		got, err := repo.GetByGameAndDiamonds(ctx, "genshin_impact", 60)
		if err != nil {
			t.Fatalf("get product: %v", err)
		}
		if got.CustomerNoFormat != "uid_server_pipe" {
			t.Fatalf("customer_no_format = %q; want uid_server_pipe", got.CustomerNoFormat)
		}
	})
}

func TestProductRepository_GetByGameAndDiamonds(t *testing.T) {
	withCleanTables(t, func(ctx context.Context) {
		repo := productRepo()

		product := &models.Product{
			ID:           uuid.New().String(),
			Game:         "mobile_legends",
			Name:         "86 Diamonds",
			PriceIDR:     12000,
			CostPriceIDR: 8000,
			ItemQty:      86,
			ProductType:  "diamond",
			SKU:          "ml_86",
			IsActive:     true,
			Stock:        -1,
		}
		if err := repo.Create(ctx, product); err != nil {
			t.Fatalf("create: %v", err)
		}

		got, err := repo.GetByGameAndDiamonds(ctx, "mobile_legends", 86)
		if err != nil {
			t.Fatalf("get by game and diamonds: %v", err)
		}
		if got.ID != product.ID {
			t.Errorf("id = %s; want %s", got.ID, product.ID)
		}
	})
}

func TestProductRepository_ListByGame(t *testing.T) {
	withCleanTables(t, func(ctx context.Context) {
		repo := productRepo()

		for i := 0; i < 3; i++ {
			product := &models.Product{
				ID:          uuid.New().String(),
				Game:        "pubg_mobile",
				Name:        fmt.Sprintf("Product %d", i),
				PriceIDR:    int(10000 * (i + 1)),
				ItemQty:     100 * (i + 1),
				ProductType: "diamond",
				SKU:         fmt.Sprintf("pubg_%d", i),
				IsActive:    true,
				Stock:       -1,
			}
			if err := repo.Create(ctx, product); err != nil {
				t.Fatalf("create product %d: %v", i, err)
			}
		}

		products, err := repo.ListByGame(ctx, "pubg_mobile")
		if err != nil {
			t.Fatalf("list by game: %v", err)
		}
		if len(products) != 3 {
			t.Errorf("len = %d; want 3", len(products))
		}
	})
}

func TestWebhookRepository_LogAndList(t *testing.T) {
	withCleanTables(t, func(ctx context.Context) {
		repo := webhookRepo()

		log := &models.WebhookLog{
			Source:    "midtrans",
			RefID:     strPtr("order-1"),
			Payload:   `{"status":"ok"}`,
			Signature: strPtr("sig-123"),
			UserAgent: strPtr("Midtrans/1.0"),
			Status:    "processed",
		}

		if err := repo.Log(ctx, log); err != nil {
			t.Fatalf("log webhook: %v", err)
		}

		logs, total, err := repo.List(ctx, "midtrans", 1, 10)
		if err != nil {
			t.Fatalf("list webhooks: %v", err)
		}
		if total != 1 {
			t.Errorf("total = %d; want 1", total)
		}
		if len(logs) != 1 {
			t.Fatalf("len = %d; want 1", len(logs))
		}
		if logs[0].Source != "midtrans" {
			t.Errorf("source = %s; want midtrans", logs[0].Source)
		}
	})
}

func strPtr(s string) *string {
	return &s
}
