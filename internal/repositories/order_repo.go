package repositories

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/topup-store/internal/models"
)

type PGOrderRepository struct {
	pool *pgxpool.Pool
}

func NewOrderRepository(pool *pgxpool.Pool) *PGOrderRepository {
	return &PGOrderRepository{pool: pool}
}

func (r *PGOrderRepository) Create(ctx context.Context, order *models.Order) error {
	_, err := r.pool.Exec(ctx, `
		INSERT INTO orders (id, product_id, user_phone, game_uid, game_server, amount_idr, status, channel, digiflazz_ref_id)
		VALUES ($1, $2, $3, $4, $5, $6, 'pending', $7, $8)
	`, order.ID, order.ProductID, order.UserPhone, order.GameUID, order.GameServer, order.AmountIDR, order.Channel, order.DigiflazzRefID)
	return err
}

func (r *PGOrderRepository) GetByID(ctx context.Context, id string) (*models.Order, error) {
	var order models.Order
	err := r.pool.QueryRow(ctx, `
		SELECT id, product_id, user_phone, game_uid, game_server, amount_idr, status,
		       midtrans_order_id, channel, serial_number, digiflazz_ref_id, created_at, updated_at
		FROM orders WHERE id = $1
	`, id).Scan(
		&order.ID, &order.ProductID, &order.UserPhone, &order.GameUID, &order.GameServer,
		&order.AmountIDR, &order.Status, &order.MidtransOrderID, &order.Channel,
		&order.SerialNumber, &order.DigiflazzRefID, &order.CreatedAt, &order.UpdatedAt,
	)
	if err != nil {
		return nil, err
	}
	return &order, nil
}

func (r *PGOrderRepository) GetByMidtransID(ctx context.Context, midtransID string) (*models.Order, error) {
	var order models.Order
	err := r.pool.QueryRow(ctx, `
		SELECT id, product_id, user_phone, game_uid, game_server, amount_idr, status,
		       midtrans_order_id, channel, serial_number, digiflazz_ref_id, created_at, updated_at
		FROM orders WHERE midtrans_order_id = $1
	`, midtransID).Scan(
		&order.ID, &order.ProductID, &order.UserPhone, &order.GameUID, &order.GameServer,
		&order.AmountIDR, &order.Status, &order.MidtransOrderID, &order.Channel,
		&order.SerialNumber, &order.DigiflazzRefID, &order.CreatedAt, &order.UpdatedAt,
	)
	if err != nil {
		return nil, err
	}
	return &order, nil
}

func (r *PGOrderRepository) GetByDigiflazzRefID(ctx context.Context, refID string) (*models.Order, error) {
	var order models.Order
	err := r.pool.QueryRow(ctx, `
		SELECT id, product_id, user_phone, game_uid, game_server, amount_idr, status,
		       midtrans_order_id, channel, serial_number, digiflazz_ref_id, created_at, updated_at
		FROM orders WHERE digiflazz_ref_id = $1
	`, refID).Scan(
		&order.ID, &order.ProductID, &order.UserPhone, &order.GameUID, &order.GameServer,
		&order.AmountIDR, &order.Status, &order.MidtransOrderID, &order.Channel,
		&order.SerialNumber, &order.DigiflazzRefID, &order.CreatedAt, &order.UpdatedAt,
	)
	if err != nil {
		return nil, err
	}
	return &order, nil
}

func (r *PGOrderRepository) GetByUIDAndPhone(ctx context.Context, gameUID, phone string) (*models.Order, error) {
	var order models.Order
	err := r.pool.QueryRow(ctx, `
		SELECT id, product_id, user_phone, game_uid, game_server, amount_idr, status,
		       midtrans_order_id, channel, serial_number, digiflazz_ref_id, created_at, updated_at
		FROM orders WHERE game_uid = $1 AND user_phone = $2 ORDER BY created_at DESC LIMIT 1
	`, gameUID, phone).Scan(
		&order.ID, &order.ProductID, &order.UserPhone, &order.GameUID, &order.GameServer,
		&order.AmountIDR, &order.Status, &order.MidtransOrderID, &order.Channel,
		&order.SerialNumber, &order.DigiflazzRefID, &order.CreatedAt, &order.UpdatedAt,
	)
	if err != nil {
		return nil, err
	}
	return &order, nil
}

func (r *PGOrderRepository) UpdateStatus(ctx context.Context, id, status string) error {
	_, err := r.pool.Exec(ctx, `UPDATE orders SET status = $1, updated_at = NOW() WHERE id = $2`, status, id)
	return err
}

func (r *PGOrderRepository) UpdateStatusIf(ctx context.Context, id, newStatus, expectedStatus string) (bool, error) {
	result, err := r.pool.Exec(ctx,
		`UPDATE orders SET status = $1, updated_at = NOW() WHERE id = $2 AND status = $3`,
		newStatus, id, expectedStatus)
	if err != nil {
		return false, err
	}
	return result.RowsAffected() > 0, nil
}

func (r *PGOrderRepository) UpdateSerialNumber(ctx context.Context, id, sn string) error {
	_, err := r.pool.Exec(ctx, `UPDATE orders SET serial_number = $1, updated_at = NOW() WHERE id = $2`, sn, id)
	return err
}

func (r *PGOrderRepository) UpdateWithQRIS(ctx context.Context, id, midtransOrderID, qrisURL string) error {
	_, err := r.pool.Exec(ctx, `
		UPDATE orders SET midtrans_order_id = $1, status = 'pending', updated_at = NOW() WHERE id = $2
	`, midtransOrderID, id)
	return err
}

func (r *PGOrderRepository) UpsertQRIS(ctx context.Context, orderID, qrisURL, qrisImageBase64 string) error {
	_, err := r.pool.Exec(ctx, `
		INSERT INTO order_qris (order_id, qris_url, qris_image_base64)
		VALUES ($1, $2, $3)
		ON CONFLICT (order_id) DO UPDATE SET qris_url = $2, qris_image_base64 = $3
	`, orderID, qrisURL, qrisImageBase64)
	return err
}

func (r *PGOrderRepository) GetQRIS(ctx context.Context, orderID string) (*models.OrderQRIS, error) {
	var qris models.OrderQRIS
	err := r.pool.QueryRow(ctx, `
		SELECT order_id, qris_url, qris_image_base64, created_at
		FROM order_qris WHERE order_id = $1
	`, orderID).Scan(&qris.OrderID, &qris.QRISURL, &qris.QRISImageBase64, &qris.CreatedAt)
	if err != nil {
		return nil, err
	}
	return &qris, nil
}

func (r *PGOrderRepository) List(ctx context.Context, page, perPage int) ([]models.Order, int, error) {
	if page < 1 {
		page = 1
	}
	if perPage < 1 || perPage > 100 {
		perPage = 20
	}
	offset := (page - 1) * perPage

	var total int
	err := r.pool.QueryRow(ctx, `SELECT COUNT(*) FROM orders`).Scan(&total)
	if err != nil {
		return nil, 0, err
	}

	rows, err := r.pool.Query(ctx, `
		SELECT id, product_id, user_phone, game_uid, game_server, amount_idr, status,
		       midtrans_order_id, channel, serial_number, digiflazz_ref_id, created_at, updated_at
		FROM orders ORDER BY created_at DESC LIMIT $1 OFFSET $2
	`, perPage, offset)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()

	var orders []models.Order
	for rows.Next() {
		var o models.Order
		if err := rows.Scan(&o.ID, &o.ProductID, &o.UserPhone, &o.GameUID, &o.GameServer,
			&o.AmountIDR, &o.Status, &o.MidtransOrderID, &o.Channel,
			&o.SerialNumber, &o.DigiflazzRefID, &o.CreatedAt, &o.UpdatedAt); err != nil {
			return nil, 0, err
		}
		orders = append(orders, o)
	}
	if err := rows.Err(); err != nil {
		return nil, 0, err
	}
	return orders, total, nil
}

func (r *PGOrderRepository) ListByStatus(ctx context.Context, status string, page, perPage int) ([]models.Order, int, error) {
	if page < 1 {
		page = 1
	}
	if perPage < 1 || perPage > 100 {
		perPage = 20
	}
	offset := (page - 1) * perPage

	var total int
	err := r.pool.QueryRow(ctx, `SELECT COUNT(*) FROM orders WHERE status = $1`, status).Scan(&total)
	if err != nil {
		return nil, 0, err
	}

	rows, err := r.pool.Query(ctx, `
		SELECT id, product_id, user_phone, game_uid, game_server, amount_idr, status,
		       midtrans_order_id, channel, serial_number, digiflazz_ref_id, created_at, updated_at
		FROM orders WHERE status = $1 ORDER BY created_at DESC LIMIT $2 OFFSET $3
	`, status, perPage, offset)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()

	var orders []models.Order
	for rows.Next() {
		var o models.Order
		if err := rows.Scan(&o.ID, &o.ProductID, &o.UserPhone, &o.GameUID, &o.GameServer,
			&o.AmountIDR, &o.Status, &o.MidtransOrderID, &o.Channel,
			&o.SerialNumber, &o.DigiflazzRefID, &o.CreatedAt, &o.UpdatedAt); err != nil {
			return nil, 0, err
		}
		orders = append(orders, o)
	}
	if err := rows.Err(); err != nil {
		return nil, 0, err
	}
	return orders, total, nil
}

func (r *PGOrderRepository) ListProcessing(ctx context.Context) ([]models.Order, error) {
	rows, err := r.pool.Query(ctx, `
		SELECT id, product_id, user_phone, game_uid, game_server, amount_idr, status,
		       midtrans_order_id, channel, serial_number, digiflazz_ref_id, created_at, updated_at
		FROM orders WHERE status = 'processing' AND created_at > NOW() - INTERVAL '24 hours'
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var orders []models.Order
	for rows.Next() {
		var o models.Order
		if err := rows.Scan(&o.ID, &o.ProductID, &o.UserPhone, &o.GameUID, &o.GameServer,
			&o.AmountIDR, &o.Status, &o.MidtransOrderID, &o.Channel,
			&o.SerialNumber, &o.DigiflazzRefID, &o.CreatedAt, &o.UpdatedAt); err != nil {
			return nil, err
		}
		orders = append(orders, o)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return orders, nil
}

func (r *PGOrderRepository) ExpireOldPending(ctx context.Context) ([]models.Order, error) {
	rows, err := r.pool.Query(ctx, `
		UPDATE orders SET status = 'expired', updated_at = NOW()
		WHERE status = 'pending' AND created_at < NOW() - INTERVAL '30 minutes'
		RETURNING id, product_id, user_phone, game_uid, game_server, amount_idr, status,
		          midtrans_order_id, channel, serial_number, digiflazz_ref_id, created_at, updated_at
	`)
	if err != nil {
		return nil, fmt.Errorf("expire old pending: %w", err)
	}
	defer rows.Close()

	var orders []models.Order
	for rows.Next() {
		var o models.Order
		if err := rows.Scan(&o.ID, &o.ProductID, &o.UserPhone, &o.GameUID, &o.GameServer,
			&o.AmountIDR, &o.Status, &o.MidtransOrderID, &o.Channel,
			&o.SerialNumber, &o.DigiflazzRefID, &o.CreatedAt, &o.UpdatedAt); err != nil {
			return nil, err
		}
		orders = append(orders, o)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return orders, nil
}

func (r *PGOrderRepository) GetPendingOrdersOlderThan(ctx context.Context, age time.Duration) ([]models.Order, error) {
	rows, err := r.pool.Query(ctx, `
		SELECT id, product_id, user_phone, game_uid, game_server, amount_idr, status,
		       midtrans_order_id, channel, serial_number, digiflazz_ref_id, created_at, updated_at
		FROM orders WHERE status = 'pending' AND created_at < NOW() - ($1 * INTERVAL '1 minute')
		ORDER BY created_at ASC
	`, int(age.Minutes()))
	if err != nil {
		return nil, fmt.Errorf("get pending orders older than: %w", err)
	}
	defer rows.Close()

	var orders []models.Order
	for rows.Next() {
		var o models.Order
		if err := rows.Scan(&o.ID, &o.ProductID, &o.UserPhone, &o.GameUID, &o.GameServer,
			&o.AmountIDR, &o.Status, &o.MidtransOrderID, &o.Channel,
			&o.SerialNumber, &o.DigiflazzRefID, &o.CreatedAt, &o.UpdatedAt); err != nil {
			return nil, err
		}
		orders = append(orders, o)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return orders, nil
}

func (r *PGOrderRepository) InsertStatusHistory(ctx context.Context, orderID, fromStatus, toStatus, reason string) error {
	_, err := r.pool.Exec(ctx, `
		INSERT INTO order_status_history (order_id, from_status, to_status, reason)
		VALUES ($1, $2, $3, $4)
	`, orderID, nullIfEmpty(fromStatus), toStatus, nullIfEmpty(reason))
	return err
}

func (r *PGOrderRepository) GetStatusHistory(ctx context.Context, orderID string) ([]models.OrderStatusHistory, error) {
	rows, err := r.pool.Query(ctx, `
		SELECT id, order_id, from_status, to_status, reason, created_at
		FROM order_status_history WHERE order_id = $1 ORDER BY created_at DESC
	`, orderID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var history []models.OrderStatusHistory
	for rows.Next() {
		var h models.OrderStatusHistory
		if err := rows.Scan(&h.ID, &h.OrderID, &h.FromStatus, &h.ToStatus, &h.Reason, &h.CreatedAt); err != nil {
			return nil, err
		}
		history = append(history, h)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return history, nil
}

func (r *PGOrderRepository) GetRecentByPhone(ctx context.Context, phone string, limit int) ([]models.Order, error) {
	rows, err := r.pool.Query(ctx, `
		SELECT id, product_id, game_uid, game_server, user_phone, status, amount_idr,
		       serial_number, midtrans_order_id, digiflazz_ref_id, created_at, updated_at
		FROM orders
		WHERE user_phone = $1
		ORDER BY created_at DESC
		LIMIT $2
	`, phone, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var orders []models.Order
	for rows.Next() {
		var o models.Order
		if err := rows.Scan(&o.ID, &o.ProductID, &o.GameUID, &o.GameServer, &o.UserPhone,
			&o.Status, &o.AmountIDR, &o.SerialNumber, &o.MidtransOrderID, &o.DigiflazzRefID,
			&o.CreatedAt, &o.UpdatedAt); err != nil {
			return nil, err
		}
		orders = append(orders, o)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return orders, nil
}

func nullIfEmpty(s string) any {
	if s == "" {
		return nil
	}
	return s
}
