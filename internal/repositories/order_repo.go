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
		INSERT INTO orders (id, order_number, product_id, user_phone, game_uid, game_server, amount_idr, status, channel, digiflazz_ref_id, stock_reserved)
		VALUES ($1, $2, $3, $4, $5, $6, $7, 'pending', $8, $9, $10)
	`, order.ID, order.OrderNumber, order.ProductID, order.UserPhone, order.GameUID, order.GameServer, order.AmountIDR, order.Channel, order.DigiflazzRefID, order.StockReserved)
	return err
}

func (r *PGOrderRepository) GetByID(ctx context.Context, id string) (*models.Order, error) {
	var order models.Order
	err := r.pool.QueryRow(ctx, `
		SELECT id, order_number, product_id, user_phone, game_uid, game_server, amount_idr, status,
		       midtrans_order_id, channel, serial_number, digiflazz_ref_id, stock_reserved, created_at, updated_at
		FROM orders WHERE id = $1
	`, id).Scan(
		&order.ID, &order.OrderNumber, &order.ProductID, &order.UserPhone, &order.GameUID, &order.GameServer,
		&order.AmountIDR, &order.Status, &order.MidtransOrderID, &order.Channel,
		&order.SerialNumber, &order.DigiflazzRefID, &order.StockReserved, &order.CreatedAt, &order.UpdatedAt,
	)
	if err != nil {
		return nil, err
	}
	return &order, nil
}

func (r *PGOrderRepository) GetByMidtransID(ctx context.Context, midtransID string) (*models.Order, error) {
	var order models.Order
	err := r.pool.QueryRow(ctx, `
		SELECT id, order_number, product_id, user_phone, game_uid, game_server, amount_idr, status,
		       midtrans_order_id, channel, serial_number, digiflazz_ref_id, created_at, updated_at
		FROM orders WHERE midtrans_order_id = $1
	`, midtransID).Scan(
		&order.ID, &order.OrderNumber, &order.ProductID, &order.UserPhone, &order.GameUID, &order.GameServer,
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
		SELECT id, order_number, product_id, user_phone, game_uid, game_server, amount_idr, status,
		       midtrans_order_id, channel, serial_number, digiflazz_ref_id, created_at, updated_at
		FROM orders WHERE digiflazz_ref_id = $1
	`, refID).Scan(
		&order.ID, &order.OrderNumber, &order.ProductID, &order.UserPhone, &order.GameUID, &order.GameServer,
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
		SELECT id, order_number, product_id, user_phone, game_uid, game_server, amount_idr, status,
		       midtrans_order_id, channel, serial_number, digiflazz_ref_id, created_at, updated_at
		FROM orders WHERE game_uid = $1 AND user_phone = $2 ORDER BY created_at DESC LIMIT 1
	`, gameUID, phone).Scan(
		&order.ID, &order.OrderNumber, &order.ProductID, &order.UserPhone, &order.GameUID, &order.GameServer,
		&order.AmountIDR, &order.Status, &order.MidtransOrderID, &order.Channel,
		&order.SerialNumber, &order.DigiflazzRefID, &order.CreatedAt, &order.UpdatedAt,
	)
	if err != nil {
		return nil, err
	}
	return &order, nil
}

func (r *PGOrderRepository) GetByOrderNumber(ctx context.Context, orderNumber string) (*models.Order, error) {
	var order models.Order
	err := r.pool.QueryRow(ctx, `
		SELECT id, order_number, product_id, user_phone, game_uid, game_server, amount_idr, status,
		       midtrans_order_id, channel, serial_number, digiflazz_ref_id, created_at, updated_at
		FROM orders WHERE order_number = $1
	`, orderNumber).Scan(
		&order.ID, &order.OrderNumber, &order.ProductID, &order.UserPhone, &order.GameUID, &order.GameServer,
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

func (r *PGOrderRepository) UpsertQRIS(ctx context.Context, orderID, qrisURL, qrString, qrisImageBase64 string, expiryTime *time.Time) error {
	_, err := r.pool.Exec(ctx, `
		INSERT INTO order_qris (order_id, qris_url, qr_string, qris_image_base64, expiry_time)
		VALUES ($1, $2, $3, $4, $5)
		ON CONFLICT (order_id) DO UPDATE SET qris_url = $2, qr_string = $3, qris_image_base64 = $4, expiry_time = $5
	`, orderID, qrisURL, qrString, qrisImageBase64, expiryTime)
	return err
}

func (r *PGOrderRepository) GetQRIS(ctx context.Context, orderID string) (*models.OrderQRIS, error) {
	var qris models.OrderQRIS
	err := r.pool.QueryRow(ctx, `
		SELECT order_id, qris_url, qr_string, qris_image_base64, expiry_time, created_at
		FROM order_qris WHERE order_id = $1
	`, orderID).Scan(&qris.OrderID, &qris.QRISURL, &qris.QRString, &qris.QRISImageBase64, &qris.ExpiryTime, &qris.CreatedAt)
	if err != nil {
		return nil, err
	}
	return &qris, nil
}

func (r *PGOrderRepository) UpsertSnap(ctx context.Context, orderID, snapToken, snapRedirectURL string) error {
	_, err := r.pool.Exec(ctx, `
		INSERT INTO order_snap (order_id, snap_token, snap_redirect_url)
		VALUES ($1, $2, $3)
		ON CONFLICT (order_id) DO UPDATE SET snap_token = $2, snap_redirect_url = $3, updated_at = NOW()
	`, orderID, snapToken, snapRedirectURL)
	return err
}

func (r *PGOrderRepository) GetSnap(ctx context.Context, orderID string) (*models.OrderSnap, error) {
	var snap models.OrderSnap
	err := r.pool.QueryRow(ctx, `
		SELECT order_id, snap_token, snap_redirect_url, created_at, updated_at
		FROM order_snap WHERE order_id = $1
	`, orderID).Scan(&snap.OrderID, &snap.SnapToken, &snap.SnapRedirectURL, &snap.CreatedAt, &snap.UpdatedAt)
	if err != nil {
		return nil, err
	}
	return &snap, nil
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
		SELECT id, order_number, product_id, user_phone, game_uid, game_server, amount_idr, status,
		       midtrans_order_id, channel, serial_number, digiflazz_ref_id, stock_reserved, created_at, updated_at
		FROM orders ORDER BY created_at DESC LIMIT $1 OFFSET $2
	`, perPage, offset)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()

	var orders []models.Order
	for rows.Next() {
		var o models.Order
		if err := rows.Scan(&o.ID, &o.OrderNumber, &o.ProductID, &o.UserPhone, &o.GameUID, &o.GameServer,
			&o.AmountIDR, &o.Status, &o.MidtransOrderID, &o.Channel,
			&o.SerialNumber, &o.DigiflazzRefID, &o.StockReserved, &o.CreatedAt, &o.UpdatedAt); err != nil {
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
		SELECT id, order_number, product_id, user_phone, game_uid, game_server, amount_idr, status,
		       midtrans_order_id, channel, serial_number, digiflazz_ref_id, stock_reserved, created_at, updated_at
		FROM orders WHERE status = 'processing' AND created_at > NOW() - INTERVAL '24 hours'
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var orders []models.Order
	for rows.Next() {
		var o models.Order
		if err := rows.Scan(&o.ID, &o.OrderNumber, &o.ProductID, &o.UserPhone, &o.GameUID, &o.GameServer,
			&o.AmountIDR, &o.Status, &o.MidtransOrderID, &o.Channel,
			&o.SerialNumber, &o.DigiflazzRefID, &o.StockReserved, &o.CreatedAt, &o.UpdatedAt); err != nil {
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
		RETURNING id, order_number, product_id, user_phone, game_uid, game_server, amount_idr, status,
		          midtrans_order_id, channel, serial_number, digiflazz_ref_id, stock_reserved, created_at, updated_at
	`)
	if err != nil {
		return nil, fmt.Errorf("expire old pending: %w", err)
	}
	defer rows.Close()

	var orders []models.Order
	for rows.Next() {
		var o models.Order
		if err := rows.Scan(&o.ID, &o.OrderNumber, &o.ProductID, &o.UserPhone, &o.GameUID, &o.GameServer,
			&o.AmountIDR, &o.Status, &o.MidtransOrderID, &o.Channel,
			&o.SerialNumber, &o.DigiflazzRefID, &o.StockReserved, &o.CreatedAt, &o.UpdatedAt); err != nil {
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
		SELECT id, order_number, product_id, user_phone, game_uid, game_server, amount_idr, status,
		       midtrans_order_id, channel, serial_number, digiflazz_ref_id, stock_reserved, created_at, updated_at
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
		if err := rows.Scan(&o.ID, &o.OrderNumber, &o.ProductID, &o.UserPhone, &o.GameUID, &o.GameServer,
			&o.AmountIDR, &o.Status, &o.MidtransOrderID, &o.Channel,
			&o.SerialNumber, &o.DigiflazzRefID, &o.StockReserved, &o.CreatedAt, &o.UpdatedAt); err != nil {
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
		SELECT id, order_number, product_id, game_uid, game_server, user_phone, status, amount_idr,
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
		if err := rows.Scan(&o.ID, &o.OrderNumber, &o.ProductID, &o.GameUID, &o.GameServer, &o.UserPhone,
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

type OrderExportRow struct {
	OrderNumber  string
	OrderID      string
	Game         string
	ProductName  string
	GameUID      string
	GameServer   string
	Phone        string
	AmountIDR    int
	Status       string
	SerialNumber string
	Channel      string
	CreatedAt    time.Time
}

func (r *PGOrderRepository) ListAllForExport(ctx context.Context) ([]OrderExportRow, error) {
	rows, err := r.pool.Query(ctx, `
		SELECT o.order_number, o.id, p.game, p.name, o.game_uid, o.game_server, o.user_phone,
		       o.amount_idr, o.status, COALESCE(o.serial_number, ''), o.channel, o.created_at
		FROM orders o
		JOIN products p ON o.product_id = p.id
		ORDER BY o.created_at DESC
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var results []OrderExportRow
	for rows.Next() {
		var row OrderExportRow
		if err := rows.Scan(&row.OrderNumber, &row.OrderID, &row.Game, &row.ProductName, &row.GameUID,
			&row.GameServer, &row.Phone, &row.AmountIDR, &row.Status,
			&row.SerialNumber, &row.Channel, &row.CreatedAt); err != nil {
			return nil, err
		}
		results = append(results, row)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return results, nil
}

type DailyRevenue struct {
	Date    string `json:"date"`
	Orders  int    `json:"orders"`
	Revenue int    `json:"revenue"`
}

type GameStats struct {
	Game    string `json:"game"`
	Orders  int    `json:"orders"`
	Revenue int    `json:"revenue"`
}

type OverallStats struct {
	TotalOrders    int     `json:"total_orders"`
	SuccessOrders  int     `json:"success_orders"`
	ConversionRate float64 `json:"conversion_rate"`
	TotalRevenue   int     `json:"total_revenue"`
	AvgOrderValue  float64 `json:"avg_order_value"`
}

func (r *PGOrderRepository) GetDailyRevenue(ctx context.Context, startDate, endDate time.Time) ([]DailyRevenue, error) {
	rows, err := r.pool.Query(ctx, `
		SELECT DATE(o.created_at)::text as date,
		       COUNT(*) FILTER (WHERE o.status = 'success') as orders,
		       COALESCE(SUM(CASE WHEN o.status = 'success' THEN o.amount_idr - COALESCE(p.cost_price_idr, 0) ELSE 0 END), 0) as revenue
		FROM orders o
		JOIN products p ON o.product_id = p.id
		WHERE o.created_at >= $1 AND o.created_at < $2
		GROUP BY DATE(o.created_at)
		ORDER BY date ASC
	`, startDate, endDate)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var results []DailyRevenue
	for rows.Next() {
		var row DailyRevenue
		if err := rows.Scan(&row.Date, &row.Orders, &row.Revenue); err != nil {
			return nil, err
		}
		results = append(results, row)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return results, nil
}

func (r *PGOrderRepository) GetTopGamesByRevenue(ctx context.Context, startDate, endDate time.Time) ([]GameStats, error) {
	rows, err := r.pool.Query(ctx, `
		SELECT p.game,
		       COUNT(*) as orders,
		       COALESCE(SUM(o.amount_idr - COALESCE(p.cost_price_idr, 0)), 0) as revenue
		FROM orders o
		JOIN products p ON o.product_id = p.id
		WHERE o.created_at >= $1 AND o.created_at < $2 AND o.status = 'success'
		GROUP BY p.game
		ORDER BY revenue DESC
	`, startDate, endDate)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var results []GameStats
	for rows.Next() {
		var row GameStats
		if err := rows.Scan(&row.Game, &row.Orders, &row.Revenue); err != nil {
			return nil, err
		}
		results = append(results, row)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return results, nil
}

func (r *PGOrderRepository) GetOverallStats(ctx context.Context, startDate, endDate time.Time) (*OverallStats, error) {
	var stats OverallStats
	err := r.pool.QueryRow(ctx, `
		SELECT COUNT(*) as total_orders,
		       COALESCE(SUM(CASE WHEN o.status = 'success' THEN 1 ELSE 0 END), 0) as success_orders,
		       COALESCE(SUM(CASE WHEN o.status = 'success' THEN o.amount_idr - COALESCE(p.cost_price_idr, 0) ELSE 0 END), 0) as total_revenue
		FROM orders o
		LEFT JOIN products p ON o.product_id = p.id
		WHERE o.created_at >= $1 AND o.created_at < $2
	`, startDate, endDate).Scan(&stats.TotalOrders, &stats.SuccessOrders, &stats.TotalRevenue)
	if err != nil {
		return nil, err
	}

	if stats.TotalOrders > 0 {
		stats.ConversionRate = float64(stats.SuccessOrders) / float64(stats.TotalOrders) * 100
	}
	if stats.SuccessOrders > 0 {
		stats.AvgOrderValue = float64(stats.TotalRevenue) / float64(stats.SuccessOrders)
	}
	return &stats, nil
}

func (r *PGOrderRepository) ListFailedForRetry(ctx context.Context, maxRetries int) ([]models.Order, error) {
	rows, err := r.pool.Query(ctx, `
		SELECT id, order_number, product_id, user_phone, game_uid, game_server, amount_idr, status,
		       midtrans_order_id, channel, serial_number, digiflazz_ref_id, created_at, updated_at
		FROM orders
		WHERE status = 'failed' AND retry_count < $1
		ORDER BY created_at DESC
		LIMIT 50
	`, maxRetries)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var orders []models.Order
	for rows.Next() {
		var o models.Order
		if err := rows.Scan(&o.ID, &o.OrderNumber, &o.ProductID, &o.UserPhone, &o.GameUID, &o.GameServer,
			&o.AmountIDR, &o.Status, &o.MidtransOrderID, &o.Channel,
			&o.SerialNumber, &o.DigiflazzRefID, &o.CreatedAt, &o.UpdatedAt); err != nil {
			return nil, err
		}
		orders = append(orders, o)
	}
	return orders, rows.Err()
}

func (r *PGOrderRepository) IncrementRetryCount(ctx context.Context, id string) error {
	_, err := r.pool.Exec(ctx, `UPDATE orders SET retry_count = retry_count + 1 WHERE id = $1`, id)
	return err
}

func nullIfEmpty(s string) any {
	if s == "" {
		return nil
	}
	return s
}
