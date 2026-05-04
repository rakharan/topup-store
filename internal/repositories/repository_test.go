package repositories

import (
	"context"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/topup-store/internal/db"
	"github.com/topup-store/internal/models"
)

var testPool *pgxpool.Pool

func TestMain(m *testing.M) {
	dsn := os.Getenv("TEST_DATABASE_URL")
	if dsn == "" {
		dsn = "postgres://topup:topup@localhost:5432/topup_store?sslmode=disable"
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

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

	code := m.Run()
	pool.Close()
	os.Exit(code)
}

func withTx(t *testing.T, fn func(ctx context.Context, tx pgx.Tx)) {
	t.Helper()
	ctx := context.Background()
	tx, err := testPool.Begin(ctx)
	if err != nil {
		t.Fatalf("begin tx: %v", err)
	}
	defer tx.Rollback(ctx)
	fn(ctx, tx)
}

func orderRepoWithTx(tx pgx.Tx) *PGOrderRepository {
	// Wrap tx in a minimal pool-like interface for the repository
	// Since PGOrderRepository takes *pgxpool.Pool, we use a workaround
	// by creating a temporary wrapper. For simplicity, we use the pool
	// directly in these tests (non-parallel) and clean up manually.
	return &PGOrderRepository{pool: testPool}
}

func productRepoWithTx(tx pgx.Tx) *PGProductRepository {
	return &PGProductRepository{pool: testPool}
}

func webhookRepoWithTx(tx pgx.Tx) *PGWebhookRepository {
	return &PGWebhookRepository{pool: testPool}
}

func cleanTables(t *testing.T, ctx context.Context) {
	t.Helper()
	tables := []string{
		"order_status_history",
		"order_qris",
		"orders",
		"products",
		"webhooks_log",
	}
	for _, table := range tables {
		if _, err := testPool.Exec(ctx, fmt.Sprintf("DELETE FROM %s", table)); err != nil {
			t.Fatalf("clean table %s: %v", table, err)
		}
	}
}

func createTestProduct(t *testing.T, ctx context.Context, game string, diamonds int) string {
	t.Helper()
	repo := &PGProductRepository{pool: testPool}
	id := uuid.New().String()
	product := &models.Product{
		ID:           id,
		Game:         game,
		Name:         fmt.Sprintf("Test %d Diamonds", diamonds),
		Description:  "test product",
		PriceIDR:     diamonds * 100,
		CostPriceIDR: diamonds * 80,
		Diamonds:     diamonds,
		ProductType:  "diamond",
		SKU:          fmt.Sprintf("test_%s_%d", game, diamonds),
		IsActive:     true,
	}
	if err := repo.Create(ctx, product); err != nil {
		t.Fatalf("create test product: %v", err)
	}
	return id
}

func TestOrderRepository_CreateAndGet(t *testing.T) {
	withTx(t, func(ctx context.Context, tx pgx.Tx) {
		cleanTables(t, ctx)
		repo := orderRepoWithTx(tx)

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
	withTx(t, func(ctx context.Context, tx pgx.Tx) {
		cleanTables(t, ctx)
		repo := orderRepoWithTx(tx)

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
	withTx(t, func(ctx context.Context, tx pgx.Tx) {
		cleanTables(t, ctx)
		repo := orderRepoWithTx(tx)

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

func TestOrderRepository_UpdateStatusIf(t *testing.T) {
	withTx(t, func(ctx context.Context, tx pgx.Tx) {
		cleanTables(t, ctx)
		repo := orderRepoWithTx(tx)

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

func TestOrderRepository_StatusHistory(t *testing.T) {
	withTx(t, func(ctx context.Context, tx pgx.Tx) {
		cleanTables(t, ctx)
		repo := orderRepoWithTx(tx)

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

func TestProductRepository_CreateAndGet(t *testing.T) {
	withTx(t, func(ctx context.Context, tx pgx.Tx) {
		cleanTables(t, ctx)
		repo := productRepoWithTx(tx)

		product := &models.Product{
			ID:           uuid.New().String(),
			Game:         "free_fire",
			Name:         "100 Diamonds",
			Description:  "Test product",
			PriceIDR:     15000,
			CostPriceIDR: 10000,
			Diamonds:     100,
			ProductType:  "diamond",
			SKU:          "ff_100",
			IsActive:     true,
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
	})
}

func TestProductRepository_GetByGameAndDiamonds(t *testing.T) {
	withTx(t, func(ctx context.Context, tx pgx.Tx) {
		cleanTables(t, ctx)
		repo := productRepoWithTx(tx)

		product := &models.Product{
			ID:           uuid.New().String(),
			Game:         "mobile_legends",
			Name:         "86 Diamonds",
			PriceIDR:     12000,
			CostPriceIDR: 8000,
			Diamonds:     86,
			ProductType:  "diamond",
			SKU:          "ml_86",
			IsActive:     true,
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
	withTx(t, func(ctx context.Context, tx pgx.Tx) {
		cleanTables(t, ctx)
		repo := productRepoWithTx(tx)

		for i := 0; i < 3; i++ {
			product := &models.Product{
				ID:          uuid.New().String(),
				Game:        "pubg_mobile",
				Name:        fmt.Sprintf("Product %d", i),
				PriceIDR:    int(10000 * (i + 1)),
				Diamonds:    100 * (i + 1),
				ProductType: "diamond",
				SKU:         fmt.Sprintf("pubg_%d", i),
				IsActive:    true,
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
	withTx(t, func(ctx context.Context, tx pgx.Tx) {
		cleanTables(t, ctx)
		repo := webhookRepoWithTx(tx)

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
