package handlers

import (
	"log/slog"
	"net/http"
	"strconv"

	"github.com/topup-store/internal/apperrors"
	"github.com/topup-store/internal/middleware"
	"github.com/topup-store/internal/models"
	"github.com/topup-store/internal/services"
)

type ProductHandler struct {
	topupSvc services.TopupServiceInterface
	logger   *slog.Logger
}

func NewProductHandler(topupSvc services.TopupServiceInterface, logger *slog.Logger) *ProductHandler {
	return &ProductHandler{topupSvc: topupSvc, logger: logger}
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

	var products []models.Product
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

	apperrors.WriteSuccess(w, http.StatusOK, products, middleware.GetRequestID(r.Context()))
}

func (h *ProductHandler) GetProduct(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	product, err := h.topupSvc.GetProduct(r.Context(), id)
	if err != nil {
		apperrors.WriteError(w, http.StatusNotFound, apperrors.ErrNotFound, middleware.GetRequestID(r.Context()))
		return
	}

	apperrors.WriteSuccess(w, http.StatusOK, product, middleware.GetRequestID(r.Context()))
}
