package services

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

type WebhookRetryEntry struct {
	ID           string    `json:"id"`
	WebhookLogID string    `json:"webhook_log_id"`
	Source       string    `json:"source"`
	RefID        *string   `json:"ref_id"`
	Payload      string    `json:"payload"`
	Attempt      int       `json:"attempt"`
	MaxAttempts  int       `json:"max_attempts"`
	NextRetry    time.Time `json:"next_retry"`
	LastError    *string   `json:"last_error"`
	Status       string    `json:"status"`
	CreatedAt    time.Time `json:"created_at"`
}

type WebhookRetryService struct {
	pool        *pgxpool.Pool
	logger      *slog.Logger
	midtransFn  func(ctx context.Context, payload []byte) error
	digiflazzFn func(ctx context.Context, payload []byte) error
}

func NewWebhookRetryService(pool *pgxpool.Pool, logger *slog.Logger) *WebhookRetryService {
	return &WebhookRetryService{pool: pool, logger: logger}
}

func (s *WebhookRetryService) SetHandlers(midtransFn, digiflazzFn func(ctx context.Context, payload []byte) error) {
	s.midtransFn = midtransFn
	s.digiflazzFn = digiflazzFn
}

func (s *WebhookRetryService) Enqueue(ctx context.Context, source, refID string, payload []byte, webhookLogID string) error {
	payloadStr := string(payload)
	_, err := s.pool.Exec(ctx, `
		INSERT INTO webhook_retry_queue (webhook_log_id, source, ref_id, payload, next_retry)
		VALUES ($1, $2, $3, $4, NOW() + INTERVAL '30 seconds')
	`, webhookLogID, source, refID, payloadStr)
	return err
}

func (s *WebhookRetryService) GetDueItems(ctx context.Context, limit int) ([]WebhookRetryEntry, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT id, webhook_log_id, source, ref_id, payload, attempt, max_attempts,
		       next_retry, last_error, status, created_at
		FROM webhook_retry_queue
		WHERE status = 'pending' AND next_retry <= NOW()
		ORDER BY next_retry ASC
		LIMIT $1
	`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var items []WebhookRetryEntry
	for rows.Next() {
		var item WebhookRetryEntry
		if err := rows.Scan(&item.ID, &item.WebhookLogID, &item.Source, &item.RefID,
			&item.Payload, &item.Attempt, &item.MaxAttempts, &item.NextRetry,
			&item.LastError, &item.Status, &item.CreatedAt); err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return items, nil
}

func (s *WebhookRetryService) MarkProcessing(ctx context.Context, id string) error {
	_, err := s.pool.Exec(ctx, `
		UPDATE webhook_retry_queue SET status = 'processing', updated_at = NOW() WHERE id = $1
	`, id)
	return err
}

func (s *WebhookRetryEntry) NextBackoff() time.Duration {
	backoffs := []time.Duration{30 * time.Second, 1 * time.Minute, 5 * time.Minute, 15 * time.Minute, 1 * time.Hour}
	if s.Attempt < len(backoffs) {
		return backoffs[s.Attempt]
	}
	return backoffs[len(backoffs)-1]
}

func (s *WebhookRetryService) MarkCompleted(ctx context.Context, id string) error {
	_, err := s.pool.Exec(ctx, `
		UPDATE webhook_retry_queue SET status = 'completed', updated_at = NOW() WHERE id = $1
	`, id)
	return err
}

func (s *WebhookRetryService) MarkRetry(ctx context.Context, id string, attempt int, nextRetry time.Time, lastErr string) error {
	_, err := s.pool.Exec(ctx, `
		UPDATE webhook_retry_queue SET attempt = $1, next_retry = $2, last_error = $3, status = 'pending', updated_at = NOW()
		WHERE id = $4
	`, attempt, nextRetry, lastErr, id)
	return err
}

func (s *WebhookRetryService) MarkDead(ctx context.Context, id string, lastErr string) error {
	_, err := s.pool.Exec(ctx, `
		UPDATE webhook_retry_queue SET status = 'dead', last_error = $1, updated_at = NOW() WHERE id = $2
	`, lastErr, id)
	return err
}

func (s *WebhookRetryService) ProcessItem(ctx context.Context, item WebhookRetryEntry) error {
	if err := s.MarkProcessing(ctx, item.ID); err != nil {
		return fmt.Errorf("mark processing: %w", err)
	}

	payload := []byte(item.Payload)
	var handler func(ctx context.Context, payload []byte) error
	switch item.Source {
	case "midtrans":
		handler = s.midtransFn
	case "digiflazz":
		handler = s.digiflazzFn
	default:
		return fmt.Errorf("unknown source: %s", item.Source)
	}

	if handler == nil {
		return fmt.Errorf("no handler registered for %s", item.Source)
	}

	if err := handler(ctx, payload); err != nil {
		return err
	}

	return s.MarkCompleted(ctx, item.ID)
}

func (s *WebhookRetryService) RunWorker(ctx context.Context, interval time.Duration) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	s.logger.Info("Webhook retry worker started", slog.String("interval", interval.String()))

	for {
		select {
		case <-ctx.Done():
			s.logger.Info("Webhook retry worker stopped")
			return
		case <-ticker.C:
			s.processDueItems(ctx)
		}
	}
}

func (s *WebhookRetryService) processDueItems(ctx context.Context) {
	items, err := s.GetDueItems(ctx, 20)
	if err != nil {
		s.logger.Error("retry worker: failed to get due items", slog.String("error", err.Error()))
		return
	}

	for _, item := range items {
		if err := s.ProcessItem(ctx, item); err != nil {
			nextAttempt := item.Attempt + 1
			if nextAttempt >= item.MaxAttempts {
				s.logger.Error("retry worker: item dead after max attempts",
					slog.String("id", item.ID),
					slog.String("source", item.Source),
					slog.String("error", err.Error()),
				)
				if markErr := s.MarkDead(ctx, item.ID, err.Error()); markErr != nil {
					s.logger.Error("retry worker: failed to mark dead", slog.String("error", markErr.Error()))
				}
			} else {
				backoff := item.NextBackoff()
				nextRetry := time.Now().Add(backoff)
				s.logger.Warn("retry worker: scheduling retry",
					slog.String("id", item.ID),
					slog.Int("attempt", nextAttempt),
					slog.String("backoff", backoff.String()),
					slog.String("error", err.Error()),
				)
				if markErr := s.MarkRetry(ctx, item.ID, nextAttempt, nextRetry, err.Error()); markErr != nil {
					s.logger.Error("retry worker: failed to mark retry", slog.String("error", markErr.Error()))
				}
			}
		}
	}
}

func (s *WebhookRetryService) GetStats(ctx context.Context) (map[string]int, error) {
	stats := make(map[string]int)
	rows, err := s.pool.Query(ctx, `SELECT status, COUNT(*) FROM webhook_retry_queue GROUP BY status`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	for rows.Next() {
		var status string
		var count int
		if err := rows.Scan(&status, &count); err != nil {
			return nil, err
		}
		stats[status] = count
	}
	return stats, nil
}

func (s *WebhookRetryService) ListDeadItems(ctx context.Context, limit int) ([]WebhookRetryEntry, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT id, webhook_log_id, source, ref_id, payload, attempt, max_attempts,
		       next_retry, last_error, status, created_at
		FROM webhook_retry_queue
		WHERE status = 'dead'
		ORDER BY created_at DESC
		LIMIT $1
	`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var items []WebhookRetryEntry
	for rows.Next() {
		var item WebhookRetryEntry
		if err := rows.Scan(&item.ID, &item.WebhookLogID, &item.Source, &item.RefID,
			&item.Payload, &item.Attempt, &item.MaxAttempts, &item.NextRetry,
			&item.LastError, &item.Status, &item.CreatedAt); err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return items, nil
}

func (s *WebhookRetryService) RetryDeadItem(ctx context.Context, id string) error {
	_, err := s.pool.Exec(ctx, `
		UPDATE webhook_retry_queue SET status = 'pending', attempt = 0, next_retry = NOW(), updated_at = NOW()
		WHERE id = $1 AND status = 'dead'
	`, id)
	return err
}

func (s *WebhookRetryEntry) MarshalJSON() ([]byte, error) {
	type Alias WebhookRetryEntry
	return json.Marshal((*Alias)(s))
}
