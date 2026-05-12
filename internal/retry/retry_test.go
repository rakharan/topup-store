package retry

import (
	"context"
	"errors"
	"testing"
	"time"
)

func TestDo_SuccessOnFirstAttempt(t *testing.T) {
	calls := 0
	err := Do(context.Background(), DefaultConfig(), func(ctx context.Context) error {
		calls++
		return nil
	})
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if calls != 1 {
		t.Fatalf("expected 1 call, got %d", calls)
	}
}

func TestDo_SuccessOnRetry(t *testing.T) {
	calls := 0
	err := Do(context.Background(), DefaultConfig(), func(ctx context.Context) error {
		calls++
		if calls < 3 {
			return errors.New("temporary error")
		}
		return nil
	})
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if calls != 3 {
		t.Fatalf("expected 3 calls, got %d", calls)
	}
}

func TestDo_MaxAttemptsExceeded(t *testing.T) {
	cfg := Config{MaxAttempts: 3, BaseDelay: 10 * time.Millisecond, MaxDelay: 50 * time.Millisecond}
	calls := 0
	expectedErr := errors.New("persistent error")
	err := Do(context.Background(), cfg, func(ctx context.Context) error {
		calls++
		return expectedErr
	})
	if err != expectedErr {
		t.Fatalf("expected %v, got %v", expectedErr, err)
	}
	if calls != 3 {
		t.Fatalf("expected 3 calls, got %d", calls)
	}
}

func TestDo_ContextCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	calls := 0
	err := Do(ctx, DefaultConfig(), func(ctx context.Context) error {
		calls++
		return errors.New("error")
	})
	if err != context.Canceled {
		t.Fatalf("expected context.Canceled, got %v", err)
	}
	if calls != 1 {
		t.Fatalf("expected 1 call before cancellation, got %d", calls)
	}
}

func TestDo_BackoffIncreases(t *testing.T) {
	cfg := Config{MaxAttempts: 4, BaseDelay: 10 * time.Millisecond, MaxDelay: 100 * time.Millisecond}
	var delays []time.Duration
	lastCall := time.Now()

	Do(context.Background(), cfg, func(ctx context.Context) error {
		now := time.Now()
		delays = append(delays, now.Sub(lastCall))
		lastCall = now
		return errors.New("error")
	})

	// First call has no delay
	if delays[0] > 5*time.Millisecond {
		t.Fatalf("first call should have no delay, got %v", delays[0])
	}
	// Second delay should be ~BaseDelay (10ms) + jitter
	if delays[1] < 5*time.Millisecond {
		t.Fatalf("second delay too short: %v", delays[1])
	}
	// Third delay should be ~2*BaseDelay (20ms) + jitter
	if delays[2] < 10*time.Millisecond {
		t.Fatalf("third delay too short: %v", delays[2])
	}
}

func TestDo_RespectsMaxDelay(t *testing.T) {
	cfg := Config{MaxAttempts: 10, BaseDelay: 100 * time.Millisecond, MaxDelay: 50 * time.Millisecond}
	start := time.Now()
	calls := 0

	Do(context.Background(), cfg, func(ctx context.Context) error {
		calls++
		return errors.New("error")
	})

	elapsed := time.Since(start)
	// With MaxDelay=50ms, 10 attempts should take ~9*50ms = 450ms max (plus some overhead)
	// But with such short max delay, it should complete quickly
	if elapsed > 2*time.Second {
		t.Fatalf("took too long: %v", elapsed)
	}
	if calls != 10 {
		t.Fatalf("expected 10 calls, got %d", calls)
	}
}
