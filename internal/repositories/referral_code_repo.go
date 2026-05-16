package repositories

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/topup-store/internal/models"
)

type PGReferralCodeRepository struct {
	pool *pgxpool.Pool
}

func NewReferralCodeRepository(pool *pgxpool.Pool) *PGReferralCodeRepository {
	return &PGReferralCodeRepository{pool: pool}
}

func (r *PGReferralCodeRepository) Create(ctx context.Context, code *models.ReferralCode) error {
	query := `
		INSERT INTO referral_codes (code, discount_idr, max_uses, is_active)
		VALUES ($1, $2, $3, $4)
		RETURNING id, created_at
	`
	return r.pool.QueryRow(ctx, query,
		code.Code, code.DiscountIDR, code.MaxUses, code.IsActive,
	).Scan(&code.ID, &code.CreatedAt)
}

func (r *PGReferralCodeRepository) GetByCode(ctx context.Context, code string) (*models.ReferralCode, error) {
	var rc models.ReferralCode
	query := `
		SELECT id, code, discount_idr, max_uses, used_count, is_active, created_at
		FROM referral_codes
		WHERE code = $1 AND is_active = true
	`
	err := r.pool.QueryRow(ctx, query, code).Scan(
		&rc.ID, &rc.Code, &rc.DiscountIDR, &rc.MaxUses, &rc.UsedCount, &rc.IsActive, &rc.CreatedAt,
	)
	if err != nil {
		if err == pgx.ErrNoRows {
			return nil, nil
		}
		return nil, fmt.Errorf("get referral code: %w", err)
	}
	return &rc, nil
}

func (r *PGReferralCodeRepository) List(ctx context.Context) ([]models.ReferralCode, error) {
	query := `
		SELECT id, code, discount_idr, max_uses, used_count, is_active, created_at
		FROM referral_codes
		ORDER BY created_at DESC
	`
	rows, err := r.pool.Query(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("list referral codes: %w", err)
	}
	defer rows.Close()

	var codes []models.ReferralCode
	for rows.Next() {
		var rc models.ReferralCode
		if err := rows.Scan(&rc.ID, &rc.Code, &rc.DiscountIDR, &rc.MaxUses, &rc.UsedCount, &rc.IsActive, &rc.CreatedAt); err != nil {
			return nil, err
		}
		codes = append(codes, rc)
	}
	return codes, rows.Err()
}

func (r *PGReferralCodeRepository) Delete(ctx context.Context, id string) error {
	_, err := r.pool.Exec(ctx, `DELETE FROM referral_codes WHERE id = $1`, id)
	return err
}

func (r *PGReferralCodeRepository) IncrementUsage(ctx context.Context, id string) error {
	_, err := r.pool.Exec(ctx, `
		UPDATE referral_codes SET used_count = used_count + 1 WHERE id = $1
	`, id)
	return err
}

func (r *PGReferralCodeRepository) ApplyToOrder(ctx context.Context, orderID, codeID string, discount int) error {
	_, err := r.pool.Exec(ctx, `
		INSERT INTO order_referrals (order_id, code_id, discount_idr)
		VALUES ($1, $2, $3)
	`, orderID, codeID, discount)
	return err
}
