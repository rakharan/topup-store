package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/go-chi/chi/v5"
	chimiddleware "github.com/go-chi/chi/v5/middleware"
	"github.com/golang-migrate/migrate/v4"
	_ "github.com/golang-migrate/migrate/v4/database/postgres"
	_ "github.com/golang-migrate/migrate/v4/source/file"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/topup-store/internal/cache"
	"github.com/topup-store/internal/config"
	"github.com/topup-store/internal/constants"
	"github.com/topup-store/internal/db"
	"github.com/topup-store/internal/handlers"
	"github.com/topup-store/internal/middleware"
	"github.com/topup-store/internal/models"
	"github.com/topup-store/internal/repositories"
	"github.com/topup-store/internal/seed"
	"github.com/topup-store/internal/services"
)

var startTime = time.Now()

func main() {
	if len(os.Args) > 1 {
		switch os.Args[1] {
		case "seed-products":
			runSeedProducts()
			return
		}
	}

	cfg, err := config.Load()
	if err != nil {
		slog.Error("Failed to load config", slog.String("error", err.Error()))
		os.Exit(1)
	}

	logLevel := slog.LevelInfo
	if lvl := os.Getenv("LOG_LEVEL"); lvl != "" {
		switch strings.ToLower(lvl) {
		case "debug":
			logLevel = slog.LevelDebug
		case "warn":
			logLevel = slog.LevelWarn
		case "error":
			logLevel = slog.LevelError
		}
	}

	var handler slog.Handler
	opts := &slog.HandlerOptions{Level: logLevel}
	if cfg.LogFormat == "json" {
		handler = slog.NewJSONHandler(os.Stdout, opts)
	} else {
		handler = slog.NewTextHandler(os.Stdout, opts)
	}
	logger := slog.New(handler)

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	pool, err := db.NewPool(ctx, db.PoolConfig{
		DSN:             cfg.DatabaseURL,
		MaxConns:        cfg.DBMaxConns,
		MinConns:        cfg.DBMinConns,
		MaxConnLifetime: mustParseDuration(cfg.DBMaxConnLifetime),
		MaxConnIdleTime: mustParseDuration(cfg.DBMaxConnIdleTime),
	})
	if err != nil {
		logger.Error("Failed to connect to database", slog.String("error", err.Error()))
		os.Exit(1)
	}
	defer pool.Close()

	if cfg.AutoMigrate {
		logger.Info("Running auto-migrations...")
		m, err := migrate.New("file://migrations", cfg.DatabaseURL)
		if err != nil {
			logger.Warn("auto-migrate: failed to initialize", slog.String("error", err.Error()))
		} else {
			if err := m.Up(); err != nil && err != migrate.ErrNoChange {
				logger.Warn("auto-migrate: some migrations failed (may be partial)", slog.String("error", err.Error()))
			} else if err == migrate.ErrNoChange {
				logger.Info("auto-migrate: database is already up to date")
			} else {
				logger.Info("auto-migrate: database is up to date")
			}
		}
	}

	rootCtx, rootCancel := context.WithCancel(context.Background())
	defer rootCancel()

	orderRepo := repositories.NewOrderRepository(pool)
	productRepo := repositories.NewProductRepository(pool)
	webhookRepo := repositories.NewWebhookRepository(pool)

	paymentSvc := services.NewPaymentService(orderRepo, cfg.MidtransServerKey, cfg.MidtransIsProd, logger)
	topupSvc := services.NewTopupService(orderRepo, productRepo, cfg.DigiflazzUsername, cfg.DigiflazzAPIKey, cfg.DigiflazzAPIURL, cfg.DigiflazzTesting, logger)
	notifySvc := services.NewNotifyService(cfg.FonnteToken, cfg.WaBotBaseURL, cfg.WaBotToken, logger)

	cacheStore, err := cache.New(cfg.RedisURL, logger)
	if err != nil {
		logger.Warn("Failed to initialize cache", slog.String("error", err.Error()))
		cacheStore = nil
	}
	if cacheStore != nil {
		defer cacheStore.Close()
	}

	retrySvc := services.NewWebhookRetryService(pool, logger)

	blockedIdentityRepo := repositories.NewBlockedIdentityRepository(pool)
	referralCodeRepo := repositories.NewReferralCodeRepository(pool)
	pages := handlers.NewPageHandler(orderRepo, topupSvc, paymentSvc, notifySvc, cfg.WhatsappNumber, cfg.AdminPassword, cfg.AdminPath, cfg.MidtransClientKey, cfg.MidtransIsProd, cfg.MidtransIsProd, cfg.AnnouncementText, cfg.AnnouncementLevel, logger)
	orders := handlers.NewOrderHandler(paymentSvc, topupSvc, notifySvc, blockedIdentityRepo, referralCodeRepo, pool, rootCtx, logger)
	products := handlers.NewProductHandler(topupSvc, cacheStore, logger)
	webhook := handlers.NewWebhookHandler(paymentSvc, topupSvc, notifySvc, webhookRepo, cfg.MidtransServerKey, cfg.DigiflazzUsername, cfg.DigiflazzAPIKey, cfg.DigiflazzWebhookSecret, rootCtx, logger)
	auditRepo := repositories.NewAuditLogRepository(pool)
	admin := handlers.NewAdminHandler(paymentSvc, topupSvc, notifySvc, productRepo, webhookRepo, orderRepo, blockedIdentityRepo, referralCodeRepo, auditRepo, cacheStore, retrySvc, cfg.AdminPassword, pool, logger)
	csrfStore := middleware.NewCSRFStore(pool)
	csrfMW := middleware.CSRFMiddleware(csrfStore)

	r := chi.NewRouter()
	r.Use(chimiddleware.Recoverer)
	r.Use(middleware.RequestID)
	r.Use(middleware.StructuredLogging(logger))

	metricsMW := middleware.NewMetricsMiddleware()
	r.Use(metricsMW.Middleware)

	timeoutDuration := mustParseDuration(cfg.RequestTimeout)
	r.Use(middleware.Timeout(timeoutDuration))

	rateLimiter := middleware.NewRateLimiter(200, time.Minute)
	if cacheStore != nil && cacheStore.Client() != nil {
		rateLimiter.WithRedis(cacheStore.Client())
	}
	r.Use(rateLimiter.Middleware)
	r.Use(middleware.MaxBodyMiddleware(1 << 20)) // 1MB
	r.Use(middleware.SecurityHeaders)
	r.Use(middleware.MaintenanceMode(cfg.MaintenanceMode, cfg.AdminPath, cfg.MaintenanceMessage))

	r.Get("/health", healthHandler(pool))
	r.Get("/ready", readyHandler(pool, cacheStore, cfg))
	r.Get("/metrics", metricsMW.Handler())

	r.Get("/", pages.Home)
	r.With(csrfMW).Get("/order", pages.OrderForm)
	r.Get("/status", pages.Status)
	r.Get("/terms", pages.Terms)
	r.Get("/refund", pages.Refund)
	r.With(csrfMW).Route(cfg.AdminPath, func(r chi.Router) {
		r.Get("/", pages.Admin)
		r.Post("/", pages.Admin)
	})

	r.Route("/api", func(r chi.Router) {
		var allowedOrigins []string
		if cfg.AllowedOrigins != "" {
			for _, o := range strings.Split(cfg.AllowedOrigins, ",") {
				trimmed := strings.TrimSpace(o)
				if trimmed != "" {
					allowedOrigins = append(allowedOrigins, trimmed)
				}
			}
		}
		r.Use(middleware.CORS(allowedOrigins))
		orderRateLimiter := middleware.NewRateLimiter(30, time.Minute)
		if cacheStore != nil && cacheStore.Client() != nil {
			orderRateLimiter.WithRedis(cacheStore.Client())
		}
		r.With(orderRateLimiter.Middleware, csrfMW).Post("/orders", orders.CreateOrder)
		r.Get("/orders", orders.ListOrders)
		r.Get("/orders/{id}", orders.GetOrder)
		r.Post("/orders/lookup", orders.LookupOrder)
		r.Get("/orders/recent", orders.RecentOrders)
		r.With(csrfMW).Post("/orders/{id}/cancel", orders.CancelOrder)
		r.Get("/products", products.ListProducts)
		r.Get("/products/{id}", products.GetProduct)
	})

	webhookRateLimiter := middleware.NewRateLimiter(60, time.Minute)
	if cacheStore != nil && cacheStore.Client() != nil {
		webhookRateLimiter.WithRedis(cacheStore.Client())
	}
	r.With(webhookRateLimiter.Middleware).Post("/webhook/midtrans", webhook.Midtrans)
	r.With(webhookRateLimiter.Middleware).Post("/webhook/digiflazz", webhook.Digiflazz)

	adminRateLimiter := middleware.NewRateLimiter(100, time.Minute)
	if cacheStore != nil && cacheStore.Client() != nil {
		adminRateLimiter.WithRedis(cacheStore.Client())
	}
	r.With(middleware.AdminAuth(cfg.AdminPassword), adminRateLimiter.Middleware, csrfMW).Post(cfg.AdminPath+"/process-order", admin.ProcessOrder)
	r.With(middleware.AdminAuth(cfg.AdminPassword), adminRateLimiter.Middleware, csrfMW).Post(cfg.AdminPath+"/retry-order", admin.RetryOrder)
	r.With(middleware.AdminAuth(cfg.AdminPassword), adminRateLimiter.Middleware, csrfMW).Post(cfg.AdminPath+"/direct-topup", admin.DirectTopup)

	r.Route(cfg.AdminPath+"/products", func(r chi.Router) {
		r.Use(middleware.AdminAuth(cfg.AdminPassword))
		r.Use(adminRateLimiter.Middleware)
		r.Use(csrfMW)
		r.Get("/", admin.ListProducts)
		r.Post("/", admin.CreateProduct)
		r.Put("/{id}", admin.UpdateProduct)
		r.Delete("/{id}", admin.DeleteProduct)
		r.Post("/sync-prices", admin.SyncPrices)
		r.Post("/sync-all", admin.SyncPricesFromDigiflazz)
	})

	r.With(middleware.AdminAuth(cfg.AdminPassword), adminRateLimiter.Middleware).Get(cfg.AdminPath+"/balance", admin.GetBalance)
	r.With(middleware.AdminAuth(cfg.AdminPassword), adminRateLimiter.Middleware).Get(cfg.AdminPath+"/orders/export", admin.ExportOrdersCSV)
	r.With(middleware.AdminAuth(cfg.AdminPassword), adminRateLimiter.Middleware).Get(cfg.AdminPath+"/analytics", admin.GetAnalytics)
	r.With(middleware.AdminAuth(cfg.AdminPassword), adminRateLimiter.Middleware).Get(cfg.AdminPath+"/retry-queue/stats", admin.GetRetryQueueStats)
	r.With(middleware.AdminAuth(cfg.AdminPassword), adminRateLimiter.Middleware).Get(cfg.AdminPath+"/retry-queue/dead", admin.ListDeadItems)
	r.With(middleware.AdminAuth(cfg.AdminPassword), adminRateLimiter.Middleware, csrfMW).Post(cfg.AdminPath+"/retry-queue/{id}/retry", admin.RetryDeadItem)
	r.With(middleware.AdminAuth(cfg.AdminPassword), adminRateLimiter.Middleware).Get(cfg.AdminPath+"/supplier-status", admin.GetSupplierStatus)
	r.With(middleware.AdminAuth(cfg.AdminPassword), adminRateLimiter.Middleware).Get(cfg.AdminPath+"/webhooks", admin.ListWebhookLogs)
	r.With(middleware.AdminAuth(cfg.AdminPassword), adminRateLimiter.Middleware).Get(cfg.AdminPath+"/orders/{id}", admin.GetOrderDetail)
	r.With(middleware.AdminAuth(cfg.AdminPassword), adminRateLimiter.Middleware, csrfMW).Post(cfg.AdminPath+"/override-status", admin.OverrideOrderStatus)

	r.Route(cfg.AdminPath+"/blocked-identities", func(r chi.Router) {
		r.Use(middleware.AdminAuth(cfg.AdminPassword))
		r.Use(adminRateLimiter.Middleware)
		r.Use(csrfMW)
		r.Get("/", admin.ListBlockedIdentities)
		r.Post("/", admin.CreateBlockedIdentity)
		r.Delete("/{id}", admin.DeleteBlockedIdentity)
	})

	r.Route(cfg.AdminPath+"/referral-codes", func(r chi.Router) {
		r.Use(middleware.AdminAuth(cfg.AdminPassword))
		r.Use(adminRateLimiter.Middleware)
		r.Use(csrfMW)
		r.Get("/", admin.ListReferralCodes)
		r.Post("/", admin.CreateReferralCode)
		r.Delete("/{id}", admin.DeleteReferralCode)
	})
	r.Route(cfg.AdminPath+"/referral-points", func(r chi.Router) {
		r.Use(middleware.AdminAuth(cfg.AdminPassword))
		r.Use(adminRateLimiter.Middleware)
		r.Use(csrfMW)
		r.Get("/", admin.ListReferralPointBalances)
		r.Post("/redeem", admin.RedeemReferralPoints)
	})

	fs := http.FileServer(http.Dir("web/static"))
	r.Handle("/static/*", http.StripPrefix("/static/", newStaticFileHandler(fs)))

	r.NotFound(pages.NotFound)

	shutdownCtx, shutdownCancel := context.WithCancel(context.Background())
	defer shutdownCancel()

	go func() {
		defer func() {
			if r := recover(); r != nil {
				logger.Error("order expiry ticker panicked", slog.Any("panic", r))
			}
		}()
		startOrderExpiryTickerWithNotify(shutdownCtx, paymentSvc, orderRepo, topupSvc, notifySvc, logger)
	}()
	go func() {
		defer func() {
			if r := recover(); r != nil {
				logger.Error("digiflazz poller panicked", slog.Any("panic", r))
			}
		}()
		startDigiflazzPoller(shutdownCtx, orderRepo, topupSvc, notifySvc, logger)
	}()
	go func() {
		defer func() {
			if r := recover(); r != nil {
				logger.Error("midtrans poller panicked", slog.Any("panic", r))
			}
		}()
		startMidtransPoller(shutdownCtx, paymentSvc, topupSvc, notifySvc, orderRepo, logger)
	}()
	go func() {
		defer func() {
			if r := recover(); r != nil {
				logger.Error("failed order retry ticker panicked", slog.Any("panic", r))
			}
		}()
		startFailedOrderRetryTicker(shutdownCtx, orderRepo, topupSvc, logger)
	}()
	go func() {
		defer func() {
			if r := recover(); r != nil {
				logger.Error("webhook retry worker panicked", slog.Any("panic", r))
			}
		}()
		retrySvc.RunWorker(shutdownCtx, 1*time.Minute)
	}()
	logger.Info("Background tickers started (expiry + digiflazz poller + webhook retry)")

	srv := &http.Server{
		Addr:    ":" + cfg.Port,
		Handler: r,
	}

	serverErr := make(chan error, 1)
	go func() {
		defer func() {
			if r := recover(); r != nil {
				logger.Error("server panicked", slog.Any("panic", r))
				serverErr <- fmt.Errorf("server panic: %v", r)
			}
		}()
		logger.Info("Server starting", slog.String("port", cfg.Port))
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			serverErr <- fmt.Errorf("server listen: %w", err)
		}
	}()

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)

	select {
	case <-quit:
		logger.Info("Shutting down server...")
		shutdownCancel()

		shutdownCtx2, shutdownCancel2 := context.WithTimeout(context.Background(), 5*time.Second)
		defer shutdownCancel2()

		if err := srv.Shutdown(shutdownCtx2); err != nil {
			logger.Error("Server forced to shutdown", slog.String("error", err.Error()))
		} else {
			logger.Info("Server stopped")
		}
	case err := <-serverErr:
		logger.Error("Server failed, shutting down", slog.String("error", err.Error()))
		shutdownCancel()
		rootCancel()
	}
}

func healthHandler(pool interface{ Ping(context.Context) error }) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if err := json.NewEncoder(w).Encode(map[string]any{
			"status":  "ok",
			"version": "1.0.0",
			"uptime":  time.Since(startTime).String(),
		}); err != nil {
			slog.Error("healthHandler: failed to encode response", slog.String("error", err.Error()))
		}
	}
}

type cacheChecker interface {
	IsEnabled() bool
}

func readyHandler(pool *pgxpool.Pool, cache cacheChecker, cfg *config.Config) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ctx, cancel := context.WithTimeout(r.Context(), 3*time.Second)
		defer cancel()

		if err := pool.Ping(ctx); err != nil {
			w.WriteHeader(http.StatusServiceUnavailable)
			json.NewEncoder(w).Encode(map[string]any{
				"status":   "not ready",
				"database": "error",
			})
			return
		}

		var productCount int
		if err := pool.QueryRow(ctx, `SELECT COUNT(*) FROM products WHERE is_active = true AND deleted_at IS NULL`).Scan(&productCount); err != nil {
			w.WriteHeader(http.StatusServiceUnavailable)
			json.NewEncoder(w).Encode(map[string]any{
				"status":   "not ready",
				"database": "connected",
				"products": "error",
			})
			return
		}

		warnings := []string{}
		if productCount == 0 {
			warnings = append(warnings, "no active products")
		}
		if cfg.MidtransClientKey == "" {
			warnings = append(warnings, "MIDTRANS_CLIENT_KEY is not configured")
		}

		w.Header().Set("Content-Type", "application/json")
		if err := json.NewEncoder(w).Encode(map[string]any{
			"status": "ready",
			"checks": map[string]any{
				"database":        "connected",
				"cache_enabled":   cache.IsEnabled(),
				"active_products": productCount,
			},
			"config": map[string]bool{
				"midtrans_server_key": cfg.MidtransServerKey != "",
				"midtrans_client_key": cfg.MidtransClientKey != "",
				"midtrans_production": cfg.MidtransIsProd,
				"digiflazz":           cfg.DigiflazzUsername != "" && cfg.DigiflazzAPIKey != "",
				"fonnte":              cfg.FonnteToken != "",
			},
			"warnings": warnings,
		}); err != nil {
			slog.Error("readyHandler: failed to encode response", slog.String("error", err.Error()))
		}
	}
}

func startOrderExpiryTickerWithNotify(ctx context.Context, paymentSvc services.PaymentServiceInterface, orderRepo repositories.OrderRepository, topupSvc services.TopupServiceInterface, notifySvc services.NotifyServiceInterface, logger *slog.Logger) {
	ticker := time.NewTicker(5 * time.Minute)
	defer ticker.Stop()

	sem := make(chan struct{}, 5)

	for {
		select {
		case <-ctx.Done():
			logger.Info("Order expiry ticker stopped")
			return
		case <-ticker.C:
			expiredOrders, err := orderRepo.ExpireOldPending(ctx)
			if err != nil {
				logger.Error("order expiry ticker", slog.String("error", err.Error()))
				continue
			}

			if len(expiredOrders) > 0 {
				logger.Info("order expiry ticker: expired orders", slog.Int("count", len(expiredOrders)))
				for _, o := range expiredOrders {
					sem <- struct{}{}
					go func(order models.Order) {
						defer func() { <-sem }()
						if order.StockReserved {
							if err := topupSvc.IncrementProductStock(ctx, order.ProductID); err != nil {
								logger.Warn("order expiry ticker: failed to restore stock", slog.String("order_id", order.ID), slog.String("product_id", order.ProductID), slog.String("error", err.Error()))
							}
						}
						if err := paymentSvc.CancelTransaction(order.ID); err != nil {
							logger.Error("order expiry ticker: failed to cancel in midtrans", slog.String("order_id", order.ID), slog.String("error", err.Error()))
						}
						if order.UserPhone != "" {
							msg := "Order " + order.ID + " telah kadaluarsa karena belum dibayar dalam 30 menit."
							if err := notifySvc.SendNotification(order.UserPhone, msg); err != nil {
								logger.Error("order expiry ticker: failed to notify", slog.String("order_id", order.ID), slog.String("error", err.Error()))
							}
						}
					}(o)
				}
			}
		}
	}
}

func startFailedOrderRetryTicker(ctx context.Context, orderRepo repositories.OrderRepository, topupSvc services.TopupServiceInterface, logger *slog.Logger) {
	ticker := time.NewTicker(10 * time.Minute)
	defer ticker.Stop()

	sem := make(chan struct{}, 3)

	for {
		select {
		case <-ctx.Done():
			logger.Info("Failed order retry ticker stopped")
			return
		case <-ticker.C:
			orders, err := orderRepo.ListFailedForRetry(ctx, 3)
			if err != nil {
				logger.Error("failed order retry: failed to list", slog.String("error", err.Error()))
				continue
			}
			if len(orders) == 0 {
				continue
			}

			logger.Info("failed order retry: retrying orders", slog.Int("count", len(orders)))
			for _, order := range orders {
				sem <- struct{}{}
				go func(o models.Order) {
					defer func() { <-sem }()
					defer func() {
						if r := recover(); r != nil {
							logger.Error("failed order retry panicked", slog.String("order_id", o.ID), slog.Any("panic", r))
						}
					}()

					if err := orderRepo.IncrementRetryCount(ctx, o.ID); err != nil {
						logger.Error("failed order retry: failed to increment count", slog.String("order_id", o.ID), slog.String("error", err.Error()))
					}
					orderRepo.InsertStatusHistory(ctx, o.ID, constants.StatusFailed, constants.StatusPaid, "auto retry: resetting for reprocessing")
					if err := topupSvc.ProcessOrder(ctx, o.ID); err != nil {
						logger.Error("failed order retry: process failed", slog.String("order_id", o.ID), slog.String("error", err.Error()))
						orderRepo.InsertStatusHistory(ctx, o.ID, constants.StatusProcessing, constants.StatusFailed, "auto retry failed: "+err.Error())
						return
					}
					logger.Info("failed order retry: success", slog.String("order_id", o.ID))
				}(order)
			}
		}
	}
}

func startDigiflazzPoller(ctx context.Context, orderRepo repositories.OrderRepository, topupSvc *services.TopupService, notifySvc services.NotifyServiceInterface, logger *slog.Logger) {
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()

	sem := make(chan struct{}, 5)

	for {
		select {
		case <-ctx.Done():
			logger.Info("Digiflazz poller stopped")
			return
		case <-ticker.C:
			orders, err := topupSvc.GetProcessingOrders(ctx)
			if err != nil {
				logger.Error("digiflazz poller: failed to get processing orders", slog.String("error", err.Error()))
				continue
			}
			if len(orders) == 0 {
				continue
			}

			for _, order := range orders {
				sem <- struct{}{}
				go func() {
					defer func() { <-sem }()
					status, sn, err := topupSvc.CheckTransactionStatus(order.ID)
					if err != nil {
						logger.Error("digiflazz poller: failed to check order", slog.String("order_id", order.ID), slog.String("error", err.Error()))
						return
					}

					if status == "Pending" {
						logger.Info("digiflazz poller: order still pending", slog.String("order_id", order.ID))
						return
					}

					if status == "Sukses" {
						if err := topupSvc.CompleteOrder(ctx, order.ID, sn); err != nil {
							logger.Error("digiflazz poller: failed to complete order", slog.String("order_id", order.ID), slog.String("error", err.Error()))
							return
						}
						logger.Info("digiflazz poller: order completed via polling", slog.String("order_id", order.ID), slog.String("sn", sn))
						product, err := topupSvc.GetProduct(ctx, order.ProductID)
						if err != nil {
							logger.Error("digiflazz poller: failed to get product for notification", slog.String("order_id", order.ID), slog.String("error", err.Error()))
							return
						}
						if err := notifySvc.SendTopupSuccess(ctx, &order, product, order.UserPhone, sn); err != nil {
							logger.Error("digiflazz poller: failed to notify", slog.String("order_id", order.ID), slog.String("error", err.Error()))
						}
					} else if status == "Gagal" || status == "Fail" {
						if err := topupSvc.FailOrder(ctx, order.ID); err != nil {
							logger.Error("digiflazz poller: failed to fail order", slog.String("order_id", order.ID), slog.String("error", err.Error()))
						}
						logger.Info("digiflazz poller: order failed via polling", slog.String("order_id", order.ID))
						product, err := topupSvc.GetProduct(ctx, order.ProductID)
						if err != nil {
							logger.Error("digiflazz poller: failed to get product for failure notification", slog.String("order_id", order.ID), slog.String("error", err.Error()))
							return
						}
						if err := notifySvc.SendTopupFailure(ctx, &order, product, order.UserPhone); err != nil {
							logger.Error("digiflazz poller: failed to notify failure", slog.String("order_id", order.ID), slog.String("error", err.Error()))
						}
					}
				}()
			}
		}
	}
}

func startMidtransPoller(ctx context.Context, paymentSvc services.PaymentServiceInterface, topupSvc *services.TopupService, notifySvc services.NotifyServiceInterface, orderRepo repositories.OrderRepository, logger *slog.Logger) {
	ticker := time.NewTicker(3 * time.Minute)
	defer ticker.Stop()

	sem := make(chan struct{}, 5)

	for {
		select {
		case <-ctx.Done():
			logger.Info("Midtrans poller stopped")
			return
		case <-ticker.C:
			orders, err := orderRepo.GetPendingOrdersOlderThan(ctx, 5*time.Minute)
			if err != nil {
				logger.Error("midtrans poller: failed to get pending orders", slog.String("error", err.Error()))
				continue
			}
			if len(orders) == 0 {
				continue
			}

			for _, order := range orders {
				sem <- struct{}{}
				go func() {
					defer func() { <-sem }()
					txStatus, fraudStatus, err := paymentSvc.CheckTransactionStatus(order.ID)
					if err != nil {
						logger.Error("midtrans poller: failed to check status", slog.String("order_id", order.ID), slog.String("error", err.Error()))
						return
					}

					switch txStatus {
					case "settlement", "capture":
						if fraudStatus == "deny" && txStatus == "capture" {
							logger.Info("midtrans poller: order captured but denied by FDS", slog.String("order_id", order.ID))
							return
						}
						if order.Status == constants.StatusPaid {
							return
						}
						if err := paymentSvc.UpdateOrderStatus(ctx, order.ID, constants.StatusPaid); err != nil {
							logger.Error("midtrans poller: failed to update order to paid", slog.String("order_id", order.ID), slog.String("error", err.Error()))
							return
						}
						paymentSvc.RecordStatusChange(ctx, order.ID, order.Status, constants.StatusPaid, "midtrans poller")
						logger.Info("midtrans poller: order marked as paid", slog.String("order_id", order.ID))
						if err := topupSvc.ProcessOrder(ctx, order.ID); err != nil {
							logger.Error("midtrans poller: failed to process order", slog.String("order_id", order.ID), slog.String("error", err.Error()))
						}
					case "expire", "cancel", "deny":
						if order.Status == constants.StatusExpired || order.Status == constants.StatusFailed {
							return
						}
						newStatus := constants.StatusFailed
						if txStatus == "expire" || txStatus == "cancel" {
							newStatus = constants.StatusExpired
						}
						if err := paymentSvc.UpdateOrderStatus(ctx, order.ID, newStatus); err != nil {
							logger.Error("midtrans poller: failed to update order", slog.String("order_id", order.ID), slog.String("error", err.Error()))
							return
						}
						paymentSvc.RecordStatusChange(ctx, order.ID, order.Status, newStatus, "midtrans poller")
						logger.Info("midtrans poller: order marked as "+newStatus, slog.String("order_id", order.ID))
					case "pending":
						logger.Info("midtrans poller: order still pending", slog.String("order_id", order.ID))
					}
				}()
			}
		}
	}
}

func mustParseDuration(s string) time.Duration {
	d, err := time.ParseDuration(s)
	if err != nil {
		return 30 * time.Second
	}
	return d
}

func runSeedProducts() {
	dsn := os.Getenv("DATABASE_URL")
	if dsn == "" {
		slog.Error("DATABASE_URL is required")
		os.Exit(1)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		slog.Error("failed to connect to database", slog.String("error", err.Error()))
		os.Exit(1)
	}
	defer pool.Close()

	if err := pool.Ping(ctx); err != nil {
		slog.Error("failed to ping database", slog.String("error", err.Error()))
		os.Exit(1)
	}

	if err := seed.Products(ctx, pool, os.Stdout); err != nil {
		slog.Error("failed to seed products", slog.String("error", err.Error()))
		os.Exit(1)
	}
}

var allowedStaticExts = map[string]bool{
	".css": true, ".js": true, ".svg": true, ".ico": true,
	".png": true, ".jpg": true, ".jpeg": true, ".gif": true,
	".woff": true, ".woff2": true, ".ttf": true,
	".txt": true, ".xml": true,
}

var staticCachePolicy = map[string]string{
	".css":   "public, max-age=3600",
	".js":    "public, max-age=3600",
	".svg":   "public, max-age=86400",
	".ico":   "public, max-age=86400",
	".png":   "public, max-age=86400",
	".jpg":   "public, max-age=86400",
	".jpeg":  "public, max-age=86400",
	".gif":   "public, max-age=86400",
	".woff":  "public, max-age=86400",
	".woff2": "public, max-age=86400",
	".ttf":   "public, max-age=86400",
	".txt":   "public, max-age=86400",
	".xml":   "public, max-age=3600",
}

func newStaticFileHandler(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path := r.URL.Path
		for ext := range allowedStaticExts {
			if strings.HasSuffix(path, ext) {
				if policy, ok := staticCachePolicy[ext]; ok {
					w.Header().Set("Cache-Control", policy)
				}
				next.ServeHTTP(w, r)
				return
			}
		}
		http.NotFound(w, r)
	})
}
