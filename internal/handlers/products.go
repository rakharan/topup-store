package handlers

import (
	"log/slog"
	"net/http"
	"strconv"
	"time"

	"github.com/topup-store/internal/apperrors"
	"github.com/topup-store/internal/cache"
	"github.com/topup-store/internal/middleware"
	"github.com/topup-store/internal/models"
	"github.com/topup-store/internal/services"
)

type ProductHandler struct {
	topupSvc services.TopupServiceInterface
	cache    *cache.Cache
	logger   *slog.Logger
}

func NewProductHandler(topupSvc services.TopupServiceInterface, cache *cache.Cache, logger *slog.Logger) *ProductHandler {
	return &ProductHandler{topupSvc: topupSvc, cache: cache, logger: logger}
}

func (h *ProductHandler) ListProducts(w http.ResponseWriter, r *http.Request) {
	game := r.URL.Query().Get("game")
	page, _ := strconv.Atoi(r.URL.Query().Get("page"))
	perPage, _ := strconv.Atoi(r.URL.Query().Get("per_page"))

	if page < 1 {
		page = 1
	}
	if perPage < 1 {
		perPage = 50
	}

	cacheKey := "products:" + game
	var products []models.Product

	if h.cache.Get(r.Context(), cacheKey, &products) {
		apperrors.WriteSuccess(w, http.StatusOK, products, middleware.GetRequestID(r.Context()))
		return
	}

	var err error
	if game != "" {
		products, err = h.topupSvc.ListProducts(r.Context(), game)
	} else {
		products, err = h.topupSvc.ListAllProducts(r.Context())
	}

	if err != nil {
		h.logger.Error("list products", slog.String("error", err.Error()))
		apperrors.WriteError(w, http.StatusInternalServerError, apperrors.ErrInternal, middleware.GetRequestID(r.Context()))
		return
	}

	h.cache.Set(r.Context(), cacheKey, products, 5*time.Minute)
	apperrors.WriteSuccess(w, http.StatusOK, products, middleware.GetRequestID(r.Context()))
}

func (h *ProductHandler) GetProduct(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	cacheKey := "product:" + id
	var product *models.Product

	if h.cache.Get(r.Context(), cacheKey, &product) {
		apperrors.WriteSuccess(w, http.StatusOK, product, middleware.GetRequestID(r.Context()))
		return
	}

	product, err := h.topupSvc.GetProduct(r.Context(), id)
	if err != nil {
		apperrors.WriteError(w, http.StatusNotFound, apperrors.ErrNotFound, middleware.GetRequestID(r.Context()))
		return
	}

	h.cache.Set(r.Context(), cacheKey, product, 5*time.Minute)
	apperrors.WriteSuccess(w, http.StatusOK, product, middleware.GetRequestID(r.Context()))
}
