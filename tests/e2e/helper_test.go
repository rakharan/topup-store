package e2e

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"testing"
	"time"

	"github.com/golang-migrate/migrate/v4"
	_ "github.com/golang-migrate/migrate/v4/database/postgres"
	_ "github.com/golang-migrate/migrate/v4/source/file"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/go-chi/chi/v5"
	chimiddleware "github.com/go-chi/chi/v5/middleware"
	"github.com/topup-store/internal/cache"
	"github.com/topup-store/internal/handlers"
	"github.com/topup-store/internal/middleware"
	"github.com/topup-store/internal/repositories"
	"github.com/topup-store/internal/services"
)

type TestServer struct {
	Server   *httptest.Server
	Pool     *pgxpool.Pool
	Cleanup  func()
}

func SetupTestServer(t testing.TB) *TestServer {
	t.Helper()

	// Change to project root so template paths work
	_, filename, _, _ := runtime.Caller(0)
	projectRoot := filepath.Join(filepath.Dir(filename), "..", "..")
	os.Chdir(projectRoot)

	dsn := os.Getenv("DATABASE_URL")
	if dsn == "" {
		dsn = "postgres://topup:topup@localhost:5432/topup_store_test?sslmode=disable"
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// Use existing test database, just clean tables
	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		// Try to create database if it doesn't exist
		adminDSN := "postgres://topup:topup@localhost:5432/postgres?sslmode=disable"
		adminPool, adminErr := pgxpool.New(ctx, adminDSN)
		if adminErr == nil {
			_, _ = adminPool.Exec(ctx, "CREATE DATABASE topup_store_test")
			adminPool.Close()
			pool, err = pgxpool.New(ctx, dsn)
		}
		if err != nil {
			t.Fatalf("failed to connect to test database: %v", err)
		}
	}

	// Drop and recreate schema for clean state
	_, _ = pool.Exec(ctx, "DROP SCHEMA IF EXISTS public CASCADE")
	_, _ = pool.Exec(ctx, "CREATE SCHEMA public")
	_, _ = pool.Exec(ctx, "GRANT ALL ON SCHEMA public TO topup")
	_, _ = pool.Exec(ctx, "GRANT ALL ON SCHEMA public TO public")

	m, err := migrate.New("file://migrations", dsn)
	if err != nil {
		pool.Close()
		t.Fatalf("failed to create migrate instance: %v", err)
	}
	if err := m.Up(); err != nil && err != migrate.ErrNoChange {
		pool.Close()
		t.Fatalf("failed to run migrations: %v", err)
	}

	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))

	orderRepo := repositories.NewOrderRepository(pool)
	productRepo := repositories.NewProductRepository(pool)
	webhookRepo := repositories.NewWebhookRepository(pool)

	paymentSvc := services.NewPaymentService(orderRepo, "test-key", false, logger)
	topupSvc := services.NewTopupService(orderRepo, productRepo, "test-user", "test-key", "https://api.digiflazz.com/v1/transaction", true, logger)
	notifySvc := services.NewNotifyService("", "", "", logger)
	webhookRetrySvc := services.NewWebhookRetryService(pool, logger)

	cacheStore, _ := cache.New("", logger)
	
	orders := handlers.NewOrderHandler(paymentSvc, topupSvc, notifySvc, pool, ctx, logger)
	products := handlers.NewProductHandler(topupSvc, cacheStore, logger)
	_ = handlers.NewAdminHandler(paymentSvc, topupSvc, notifySvc, productRepo, webhookRepo, orderRepo, nil, cacheStore, webhookRetrySvc, "test-admin-pass", logger)
	webhook := handlers.NewWebhookHandler(paymentSvc, topupSvc, notifySvc, webhookRepo, "", "", "", "", ctx, logger)
	pages := handlers.NewPageHandler(topupSvc, paymentSvc, notifySvc, "", "test-admin-pass", "/admin", "", false, false, logger)

	r := chi.NewRouter()
	r.Use(chimiddleware.RequestID)
	r.Use(chimiddleware.Recoverer)

	csrfStore := middleware.NewCSRFStore(pool)
	csrfMW := middleware.CSRFMiddleware(csrfStore)

	r.Get("/health", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprintf(w, `{"status":"ok"}`)
	})
	r.Get("/ready", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprintf(w, `{"status":"ok"}`)
	})

	// Static files
	fs := http.FileServer(http.Dir("web/static"))
	r.Handle("/static/*", http.StripPrefix("/static/", fs))

	r.Get("/", pages.Home)
	r.Get("/api/products", products.ListProducts)
	r.Get("/api/products/{id}", products.GetProduct)
	r.With(csrfMW).Post("/api/orders", orders.CreateOrder)
	r.Get("/api/orders/{id}", orders.GetOrder)
	r.Post("/api/orders/lookup", orders.LookupOrder)
	r.Post("/webhook/midtrans", webhook.Midtrans)
	r.Post("/webhook/digiflazz", webhook.Digiflazz)

	server := httptest.NewServer(r)

	return &TestServer{
		Server:  server,
		Pool:    pool,
		Cleanup: func() { server.Close(); pool.Close() },
	}
}
