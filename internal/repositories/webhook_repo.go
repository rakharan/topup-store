package repositories

import (
	"context"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/topup-store/internal/models"
)

type PGWebhookRepository struct {
	pool *pgxpool.Pool
}

func NewWebhookRepository(pool *pgxpool.Pool) *PGWebhookRepository {
	return &PGWebhookRepository{pool: pool}
}

func (r *PGWebhookRepository) Log(ctx context.Context, log *models.WebhookLog) error {
	_, err := r.pool.Exec(ctx, `
		INSERT INTO webhooks_log (source, ref_id, payload, signature, user_agent, status, error)
		VALUES ($1, $2, $3, $4, $5, $6, $7)
	`, log.Source, log.RefID, log.Payload, log.Signature, log.UserAgent, log.Status, log.Error)
	return err
}

func (r *PGWebhookRepository) List(ctx context.Context, source string, page, perPage int) ([]models.WebhookLog, int, error) {
	if page < 1 {
		page = 1
	}
	if perPage < 1 || perPage > 100 {
		perPage = 20
	}
	offset := (page - 1) * perPage

	var total int
	var err error
	if source != "" {
		err = r.pool.QueryRow(ctx, `SELECT COUNT(*) FROM webhooks_log WHERE source = $1`, source).Scan(&total)
	} else {
		err = r.pool.QueryRow(ctx, `SELECT COUNT(*) FROM webhooks_log`).Scan(&total)
	}
	if err != nil {
		return nil, 0, err
	}

	var rows pgx.Rows
	if source != "" {
		rows, err = r.pool.Query(ctx, `
			SELECT id, source, ref_id, payload, signature, user_agent, status, error, created_at
			FROM webhooks_log WHERE source = $1 ORDER BY created_at DESC LIMIT $2 OFFSET $3
		`, source, perPage, offset)
	} else {
		rows, err = r.pool.Query(ctx, `
			SELECT id, source, ref_id, payload, signature, user_agent, status, error, created_at
			FROM webhooks_log ORDER BY created_at DESC LIMIT $1 OFFSET $2
		`, perPage, offset)
	}
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()

	var logs []models.WebhookLog
	for rows.Next() {
		var l models.WebhookLog
		if err := rows.Scan(&l.ID, &l.Source, &l.RefID, &l.Payload, &l.Signature, &l.UserAgent, &l.Status, &l.Error, &l.CreatedAt); err != nil {
			return nil, 0, err
		}
		logs = append(logs, l)
	}
	if err := rows.Err(); err != nil {
		return nil, 0, err
	}
	return logs, total, nil
}
