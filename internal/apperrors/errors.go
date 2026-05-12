package apperrors

import (
	"encoding/json"
	"log/slog"
	"net/http"
)

type AppError struct {
	Code    string `json:"code"`
	Message string `json:"message"`
	Field   string `json:"field,omitempty"`
}

func (e *AppError) Error() string {
	return e.Message
}

var (
	ErrNotFound      = &AppError{Code: "NOT_FOUND", Message: "Resource not found"}
	ErrInvalidInput  = &AppError{Code: "INVALID_INPUT", Message: "Invalid input"}
	ErrPaymentFailed = &AppError{Code: "PAYMENT_FAILED", Message: "Payment processing failed"}
	ErrUnauthorized  = &AppError{Code: "UNAUTHORIZED", Message: "Unauthorized"}
	ErrRateLimited   = &AppError{Code: "RATE_LIMITED", Message: "Rate limit exceeded"}
	ErrInternal      = &AppError{Code: "INTERNAL_ERROR", Message: "Internal server error"}
)

type APIResponse struct {
	Success   bool      `json:"success"`
	Data      any       `json:"data,omitempty"`
	Error     *AppError `json:"error,omitempty"`
	RequestID string    `json:"request_id,omitempty"`
}

func WriteError(w http.ResponseWriter, status int, appErr *AppError, requestID string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(APIResponse{
		Success:   false,
		Error:     appErr,
		RequestID: requestID,
	}); err != nil {
		slog.Error("WriteError: failed to encode response", slog.String("error", err.Error()))
	}
}

func WriteSuccess(w http.ResponseWriter, status int, data any, requestID string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(APIResponse{
		Success:   true,
		Data:      data,
		RequestID: requestID,
	}); err != nil {
		slog.Error("WriteSuccess: failed to encode response", slog.String("error", err.Error()))
	}
}

func FieldError(field, message string) *AppError {
	return &AppError{Code: "INVALID_INPUT", Message: message, Field: field}
}

func NewError(code, message string) *AppError {
	return &AppError{Code: code, Message: message}
}
