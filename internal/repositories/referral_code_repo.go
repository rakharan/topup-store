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
		INSERT INTO referral_codes (code, owner_phone, discount_idr, reward_points, min_order_idr, max_uses, is_active)
		VALUES ($1, $2, $3, $4, $5, $6, $7)
		RETURNING id, created_at
	`
	return r.pool.QueryRow(ctx, query,
		code.Code, code.OwnerPhone, code.DiscountIDR, code.RewardPoints, code.MinOrderIDR, code.MaxUses, code.IsActive,
	).Scan(&code.ID, &code.CreatedAt)
}

func (r *PGReferralCodeRepository) GetByCode(ctx context.Context, code string) (*models.ReferralCode, error) {
	var rc models.ReferralCode
	query := `
		SELECT id, code, owner_phone, discount_idr, reward_points, min_order_idr, max_uses, used_count, is_active, created_at
		FROM referral_codes
		WHERE code = $1 AND is_active = true
	`
	err := r.pool.QueryRow(ctx, query, code).Scan(
		&rc.ID, &rc.Code, &rc.OwnerPhone, &rc.DiscountIDR, &rc.RewardPoints, &rc.MinOrderIDR, &rc.MaxUses, &rc.UsedCount, &rc.IsActive, &rc.CreatedAt,
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
		SELECT id, code, owner_phone, discount_idr, reward_points, min_order_idr, max_uses, used_count, is_active, created_at
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
		if err := rows.Scan(&rc.ID, &rc.Code, &rc.OwnerPhone, &rc.DiscountIDR, &rc.RewardPoints, &rc.MinOrderIDR, &rc.MaxUses, &rc.UsedCount, &rc.IsActive, &rc.CreatedAt); err != nil {
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

func (r *PGReferralCodeRepository) ListPointBalances(ctx context.Context) ([]models.ReferralPointBalance, error) {
	rows, err := r.pool.Query(ctx, `
		SELECT owner_phone,
		       COALESCE(SUM(points), 0) AS points,
		       MAX(created_at) FILTER (WHERE event_type = 'earn') AS last_earned
		FROM referral_point_ledger
		GROUP BY owner_phone
		HAVING COALESCE(SUM(points), 0) > 0
		ORDER BY points DESC, owner_phone ASC
	`)
	if err != nil {
		return nil, fmt.Errorf("list referral point balances: %w", err)
	}
	defer rows.Close()

	var balances []models.ReferralPointBalance
	for rows.Next() {
		var b models.ReferralPointBalance
		if err := rows.Scan(&b.OwnerPhone, &b.Points, &b.LastEarned); err != nil {
			return nil, err
		}
		balances = append(balances, b)
	}
	return balances, rows.Err()
}

func (r *PGReferralCodeRepository) RedeemPoints(ctx context.Context, ownerPhone, couponCode string, points int) error {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)

	var balance int
	if err := tx.QueryRow(ctx, `
		SELECT COALESCE(SUM(points), 0)
		FROM referral_point_ledger
		WHERE owner_phone = $1
	`, ownerPhone).Scan(&balance); err != nil {
		return fmt.Errorf("check referral point balance: %w", err)
	}
	if balance < points {
		return fmt.Errorf("insufficient referral points")
	}

	if _, err := tx.Exec(ctx, `
		INSERT INTO coupons (code, description, discount_type, discount_value, max_uses, is_active)
		VALUES ($1, $2, 'fixed', $3, 1, true)
	`, couponCode, "Referral points redemption for "+ownerPhone, points); err != nil {
		return fmt.Errorf("create redemption coupon: %w", err)
	}

	if _, err := tx.Exec(ctx, `
		INSERT INTO referral_point_ledger (owner_phone, event_type, points, note)
		VALUES ($1, 'redeem', $2, $3)
	`, ownerPhone, -points, "Redeemed to coupon "+couponCode); err != nil {
		return fmt.Errorf("record referral redemption: %w", err)
	}

	return tx.Commit(ctx)
}
