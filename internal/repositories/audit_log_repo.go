package repositories

import (
	"context"

	"github.com/jackc/pgx/v5/pgxpool"
)

type PGAuditLogRepository struct {
	pool *pgxpool.Pool
}

func NewAuditLogRepository(pool *pgxpool.Pool) *PGAuditLogRepository {
	return &PGAuditLogRepository{pool: pool}
}

func (r *PGAuditLogRepository) Log(ctx context.Context, entry *AuditLogEntry) error {
	_, err := r.pool.Exec(ctx, `
		INSERT INTO admin_audit_log (action, entity_type, entity_id, old_value, new_value, admin_ip, admin_user_agent)
		VALUES ($1, $2, $3, $4, $5, $6, $7)
	`, entry.Action, entry.EntityType, entry.EntityID, entry.OldValue, entry.NewValue, entry.AdminIP, entry.AdminUA)
	return err
}
