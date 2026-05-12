package apperrors

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestAppError_Error(t *testing.T) {
	err := &AppError{Code: "TEST", Message: "test message"}
	if err.Error() != "test message" {
		t.Fatalf("expected 'test message', got %s", err.Error())
	}
}

func TestWriteError(t *testing.T) {
	w := httptest.NewRecorder()
	WriteError(w, http.StatusBadRequest, ErrInvalidInput, "req-123")

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected status %d, got %d", http.StatusBadRequest, w.Code)
	}

	ct := w.Header().Get("Content-Type")
	if ct != "application/json" {
		t.Fatalf("expected Content-Type application/json, got %s", ct)
	}

	var resp APIResponse
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("failed to decode: %v", err)
	}
	if resp.Success != false {
		t.Fatalf("expected success=false")
	}
	if resp.Error == nil {
		t.Fatalf("expected error field")
	}
	if resp.Error.Code != "INVALID_INPUT" {
		t.Fatalf("expected INVALID_INPUT, got %s", resp.Error.Code)
	}
	if resp.RequestID != "req-123" {
		t.Fatalf("expected request_id=req-123, got %s", resp.RequestID)
	}
}

func TestWriteSuccess(t *testing.T) {
	w := httptest.NewRecorder()
	WriteSuccess(w, http.StatusOK, map[string]string{"key": "value"}, "req-456")

	if w.Code != http.StatusOK {
		t.Fatalf("expected status %d, got %d", http.StatusOK, w.Code)
	}

	var resp APIResponse
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("failed to decode: %v", err)
	}
	if resp.Success != true {
		t.Fatalf("expected success=true")
	}
	if resp.Data == nil {
		t.Fatalf("expected data field")
	}
	if resp.RequestID != "req-456" {
		t.Fatalf("expected request_id=req-456, got %s", resp.RequestID)
	}
}

func TestFieldError(t *testing.T) {
	err := FieldError("game_uid", "required field")
	if err.Code != "INVALID_INPUT" {
		t.Fatalf("expected INVALID_INPUT, got %s", err.Code)
	}
	if err.Message != "required field" {
		t.Fatalf("expected 'required field', got %s", err.Message)
	}
	if err.Field != "game_uid" {
		t.Fatalf("expected field=game_uid, got %s", err.Field)
	}
}

func TestNewError(t *testing.T) {
	err := NewError("CUSTOM", "custom error")
	if err.Code != "CUSTOM" {
		t.Fatalf("expected CUSTOM, got %s", err.Code)
	}
	if err.Message != "custom error" {
		t.Fatalf("expected 'custom error', got %s", err.Message)
	}
}

func TestPredefinedErrors(t *testing.T) {
	tests := []struct {
		err  *AppError
		code string
	}{
		{ErrNotFound, "NOT_FOUND"},
		{ErrInvalidInput, "INVALID_INPUT"},
		{ErrPaymentFailed, "PAYMENT_FAILED"},
		{ErrUnauthorized, "UNAUTHORIZED"},
		{ErrRateLimited, "RATE_LIMITED"},
		{ErrInternal, "INTERNAL_ERROR"},
	}

	for _, tt := range tests {
		t.Run(tt.code, func(t *testing.T) {
			if tt.err.Code != tt.code {
				t.Fatalf("expected %s, got %s", tt.code, tt.err.Code)
			}
		})
	}
}
