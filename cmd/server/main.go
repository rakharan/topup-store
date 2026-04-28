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
	"github.com/topup-store/internal/config"
	"github.com/topup-store/internal/db"
	"github.com/topup-store/internal/handlers"
	"github.com/topup-store/internal/middleware"
	"github.com/topup-store/internal/models"
	"github.com/topup-store/internal/repositories"
	"github.com/topup-store/internal/services"
)

var startTime = time.Now()

func main() {
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

	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{
		Level: logLevel,
	}))

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

	rootCtx, rootCancel := context.WithCancel(context.Background())
	defer rootCancel()

	orderRepo := repositories.NewOrderRepository(pool)
	productRepo := repositories.NewProductRepository(pool)
	webhookRepo := repositories.NewWebhookRepository(pool)

	paymentSvc := services.NewPaymentService(orderRepo, cfg.MidtransServerKey, cfg.MidtransIsProd)
	topupSvc := services.NewTopupService(orderRepo, productRepo, cfg.DigiflazzUsername, cfg.DigiflazzAPIKey, cfg.DigiflazzAPIURL, cfg.DigiflazzTesting, logger)
	notifySvc := services.NewNotifyService(cfg.WhatsappNumber, cfg.WhatsappToken, cfg.WhatsappPhoneID, cfg.WaBotBaseURL, cfg.WaBotToken, logger)

	pages := handlers.NewPageHandler(topupSvc, paymentSvc, notifySvc, cfg.WhatsappNumber, cfg.AdminPassword, cfg.MidtransIsProd, logger)
	orders := handlers.NewOrderHandler(paymentSvc, topupSvc, notifySvc, rootCtx, logger)
	products := handlers.NewProductHandler(topupSvc, logger)
	webhook := handlers.NewWebhookHandler(paymentSvc, topupSvc, notifySvc, webhookRepo, cfg.MidtransServerKey, cfg.DigiflazzUsername, cfg.DigiflazzAPIKey, cfg.DigiflazzWebhookSecret, rootCtx, logger)
	admin := handlers.NewAdminHandler(paymentSvc, topupSvc, notifySvc, productRepo, cfg.AdminPassword, logger)
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

	rateLimiter := middleware.NewRateLimiter(100, time.Minute)
	r.Use(rateLimiter.Middleware)
	r.Use(middleware.MaxBodyMiddleware(1 << 20)) // 1MB
	r.Use(middleware.SecurityHeaders)

	r.Get("/health", healthHandler(pool))
	r.Get("/metrics", metricsMW.Handler())

	r.Get("/", pages.Home)
	r.With(csrfMW).Get("/order", pages.OrderForm)
	r.Get("/status", pages.Status)
	r.With(csrfMW).Route("/admin", func(r chi.Router) {
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
		orderRateLimiter := middleware.NewRateLimiter(5, time.Minute)
		r.With(orderRateLimiter.Middleware, csrfMW).Post("/orders", orders.CreateOrder)
		r.Get("/orders", orders.ListOrders)
		r.Get("/orders/{id}", orders.GetOrder)
		r.Post("/orders/lookup", orders.LookupOrder)
		r.Get("/products", products.ListProducts)
		r.Get("/products/{id}", products.GetProduct)
	})

	r.Post("/webhook/midtrans", webhook.Midtrans)
	r.Post("/webhook/digiflazz", webhook.Digiflazz)

	adminRateLimiter := middleware.NewRateLimiter(30, time.Minute)
	r.With(middleware.AdminAuth(cfg.AdminPassword), adminRateLimiter.Middleware, csrfMW).Post("/admin/process-order", admin.ProcessOrder)
	r.With(middleware.AdminAuth(cfg.AdminPassword), adminRateLimiter.Middleware, csrfMW).Post("/admin/retry-order", admin.RetryOrder)

	r.Route("/admin/products", func(r chi.Router) {
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

	fs := http.FileServer(http.Dir("web/static"))
	r.Handle("/static/*", http.StripPrefix("/static/", fs))

	shutdownCtx, shutdownCancel := context.WithCancel(context.Background())
	defer shutdownCancel()

	go func() {
		defer func() {
			if r := recover(); r != nil {
				logger.Error("order expiry ticker panicked", slog.Any("panic", r))
			}
		}()
		startOrderExpiryTickerWithNotify(shutdownCtx, orderRepo, notifySvc, logger)
	}()
	go func() {
		defer func() {
			if r := recover(); r != nil {
				logger.Error("digiflazz poller panicked", slog.Any("panic", r))
			}
		}()
		startDigiflazzPoller(shutdownCtx, orderRepo, topupSvc, notifySvc, logger)
	}()
	logger.Info("Background tickers started (expiry + digiflazz poller)")

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
		status := "ok"
		dbStatus := "connected"

		ctx, cancel := context.WithTimeout(r.Context(), 3*time.Second)
		defer cancel()

		if err := pool.Ping(ctx); err != nil {
			status = "degraded"
			dbStatus = "error"
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]any{
			"status":   status,
			"version":  "1.0.0",
			"database": dbStatus,
			"uptime":   time.Since(startTime).String(),
		})
	}
}

func startOrderExpiryTickerWithNotify(ctx context.Context, orderRepo repositories.OrderRepository, notifySvc services.NotifyServiceInterface, logger *slog.Logger) {
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
					if o.UserPhone != "" {
						sem <- struct{}{}
						go func(order models.Order) {
							defer func() { <-sem }()
							msg := "Order " + order.ID + " telah kadaluarsa karena belum dibayar dalam 30 menit."
							if err := notifySvc.SendNotification(order.UserPhone, msg); err != nil {
								logger.Error("order expiry ticker: failed to notify", slog.String("order_id", order.ID), slog.String("error", err.Error()))
							}
						}(o)
					}
				}
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
				go func(o models.Order) {
					defer func() { <-sem }()
					status, sn, err := topupSvc.CheckTransactionStatus(o.ID)
					if err != nil {
						logger.Error("digiflazz poller: failed to check order", slog.String("order_id", o.ID), slog.String("error", err.Error()))
						return
					}

					if status == "Pending" {
						logger.Info("digiflazz poller: order still pending", slog.String("order_id", o.ID))
						return
					}

					if status == "Sukses" {
						if err := topupSvc.CompleteOrder(ctx, o.ID, sn); err != nil {
							logger.Error("digiflazz poller: failed to complete order", slog.String("order_id", o.ID), slog.String("error", err.Error()))
							return
						}
						logger.Info("digiflazz poller: order completed via polling", slog.String("order_id", o.ID), slog.String("sn", sn))
						if err := notifySvc.SendTopupSuccess(context.Background(), &o, o.UserPhone); err != nil {
							logger.Error("digiflazz poller: failed to notify", slog.String("order_id", o.ID), slog.String("error", err.Error()))
						}
					} else if status == "Gagal" || status == "Fail" {
						if err := topupSvc.FailOrder(ctx, o.ID); err != nil {
							logger.Error("digiflazz poller: failed to fail order", slog.String("order_id", o.ID), slog.String("error", err.Error()))
						}
						logger.Info("digiflazz poller: order failed via polling", slog.String("order_id", o.ID))
						if err := notifySvc.SendTopupFailure(context.Background(), &o, o.UserPhone); err != nil {
							logger.Error("digiflazz poller: failed to notify failure", slog.String("order_id", o.ID), slog.String("error", err.Error()))
						}
					}
				}(order)
			}
		}
	}
}

func mustParseDuration(s string) time.Duration {
	d, err := time.ParseDuration(s)
	if err != nil {
		return 30 * time.Minute
	}
	return d
}
