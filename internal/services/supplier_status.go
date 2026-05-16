package services

import (
	"sync"
	"time"
)

type SupplierError struct {
	Supplier  string    `json:"supplier"`
	Endpoint  string    `json:"endpoint"`
	Error     string    `json:"error"`
	Timestamp time.Time `json:"timestamp"`
}

type SupplierStatusTracker struct {
	mu     sync.RWMutex
	errors []SupplierError
	limit  int
}

func NewSupplierStatusTracker(limit int) *SupplierStatusTracker {
	return &SupplierStatusTracker{limit: limit}
}

func (t *SupplierStatusTracker) RecordError(supplier, endpoint, err string) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.errors = append(t.errors, SupplierError{
		Supplier:  supplier,
		Endpoint:  endpoint,
		Error:     err,
		Timestamp: time.Now(),
	})
	if len(t.errors) > t.limit {
		t.errors = t.errors[len(t.errors)-t.limit:]
	}
}

func (t *SupplierStatusTracker) RecentErrors() []SupplierError {
	t.mu.RLock()
	defer t.mu.RUnlock()
	result := make([]SupplierError, len(t.errors))
	copy(result, t.errors)
	return result
}

func (t *SupplierStatusTracker) IsHealthy(supplier string, window time.Duration) bool {
	t.mu.RLock()
	defer t.mu.RUnlock()
	cutoff := time.Now().Add(-window)
	for _, e := range t.errors {
		if e.Supplier == supplier && e.Timestamp.After(cutoff) {
			return false
		}
	}
	return true
}
