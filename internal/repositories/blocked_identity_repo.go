package repositories

import (
	"context"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/topup-store/internal/models"
)

type PGBlockedIdentityRepository struct {
	pool *pgxpool.Pool
}

func NewBlockedIdentityRepository(pool *pgxpool.Pool) *PGBlockedIdentityRepository {
	return &PGBlockedIdentityRepository{pool: pool}
}

func (r *PGBlockedIdentityRepository) Create(ctx context.Context, b *models.BlockedIdentity) error {
	_, err := r.pool.Exec(ctx, `
		INSERT INTO blocked_identities (id, phone, game_uid, ip_address, reason, blocked_by)
		VALUES ($1, $2, $3, $4, $5, $6)
	`, b.ID, nullIfEmptyPtr(b.Phone), nullIfEmptyPtr(b.GameUID), nullIfEmptyPtr(b.IPAddress), b.Reason, b.BlockedBy)
	return err
}

func (r *PGBlockedIdentityRepository) Delete(ctx context.Context, id string) error {
	_, err := r.pool.Exec(ctx, `DELETE FROM blocked_identities WHERE id = $1`, id)
	return err
}

func (r *PGBlockedIdentityRepository) List(ctx context.Context) ([]models.BlockedIdentity, error) {
	rows, err := r.pool.Query(ctx, `
		SELECT id, phone, game_uid, ip_address, reason, blocked_by, created_at
		FROM blocked_identities
		ORDER BY created_at DESC
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var list []models.BlockedIdentity
	for rows.Next() {
		var b models.BlockedIdentity
		var phone, gameUID, ipAddress *string
		if err := rows.Scan(&b.ID, &phone, &gameUID, &ipAddress, &b.Reason, &b.BlockedBy, &b.CreatedAt); err != nil {
			return nil, err
		}
		b.Phone = phone
		b.GameUID = gameUID
		b.IPAddress = ipAddress
		list = append(list, b)
	}
	return list, rows.Err()
}

func (r *PGBlockedIdentityRepository) IsBlocked(ctx context.Context, phone, gameUID, ipAddress string) (bool, string, error) {
	var reason string
	var conditions []string
	var args []any
	argIdx := 1

	if phone != "" {
		conditions = append(conditions, fmt.Sprintf("phone = $%d", argIdx))
		args = append(args, phone)
		argIdx++
	}
	if gameUID != "" {
		conditions = append(conditions, fmt.Sprintf("game_uid = $%d", argIdx))
		args = append(args, gameUID)
		argIdx++
	}
	if ipAddress != "" {
		conditions = append(conditions, fmt.Sprintf("ip_address = $%d", argIdx))
		args = append(args, ipAddress)
		argIdx++
	}

	if len(conditions) == 0 {
		return false, "", nil
	}

	query := "SELECT reason FROM blocked_identities WHERE " + strings.Join(conditions, " OR ") + " LIMIT 1"
	err := r.pool.QueryRow(ctx, query, args...).Scan(&reason)
	if err != nil {
		if err.Error() == "no rows in result set" {
			return false, "", nil
		}
		return false, "", err
	}
	return true, reason, nil
}

func nullIfEmptyPtr(s *string) any {
	if s == nil || *s == "" {
		return nil
	}
	return *s
}
