package repositories

import (
	"context"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/topup-store/internal/constants"
	"github.com/topup-store/internal/models"
)

type PGProductRepository struct {
	pool *pgxpool.Pool
}

func NewProductRepository(pool *pgxpool.Pool) *PGProductRepository {
	return &PGProductRepository{pool: pool}
}

func (r *PGProductRepository) GetByID(ctx context.Context, id string) (*models.Product, error) {
	var p models.Product
	err := r.pool.QueryRow(ctx, `
		SELECT id, game, name, description, price_idr, cost_price_idr, diamonds, product_type, sku, customer_no_format, is_active, stock, created_at, updated_at, deleted_at
		FROM products WHERE id = $1
	`, id).Scan(&p.ID, &p.Game, &p.Name, &p.Description, &p.PriceIDR, &p.CostPriceIDR, &p.Diamonds, &p.ProductType, &p.SKU, &p.CustomerNoFormat, &p.IsActive, &p.Stock, &p.CreatedAt, &p.UpdatedAt, &p.DeletedAt)
	if err != nil {
		return nil, err
	}
	return &p, nil
}

func (r *PGProductRepository) GetByGameAndDiamonds(ctx context.Context, game string, diamonds int) (*models.Product, error) {
	var p models.Product
	err := r.pool.QueryRow(ctx, `
		SELECT id, game, name, description, price_idr, cost_price_idr, diamonds, product_type, sku, customer_no_format, is_active, stock, created_at, updated_at, deleted_at
		FROM products WHERE game = $1 AND diamonds = $2 AND is_active = true AND deleted_at IS NULL
	`, game, diamonds).Scan(&p.ID, &p.Game, &p.Name, &p.Description, &p.PriceIDR, &p.CostPriceIDR, &p.Diamonds, &p.ProductType, &p.SKU, &p.CustomerNoFormat, &p.IsActive, &p.Stock, &p.CreatedAt, &p.UpdatedAt, &p.DeletedAt)
	if err != nil {
		return nil, err
	}
	return &p, nil
}

func (r *PGProductRepository) ListByGame(ctx context.Context, game string) ([]models.Product, error) {
	rows, err := r.pool.Query(ctx, `
		SELECT id, game, name, description, price_idr, cost_price_idr, diamonds, product_type, sku, customer_no_format, is_active, stock, created_at, updated_at, deleted_at
		FROM products WHERE game = $1 AND is_active = true AND deleted_at IS NULL ORDER BY price_idr
	`, game)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var products []models.Product
	for rows.Next() {
		var p models.Product
		if err := rows.Scan(&p.ID, &p.Game, &p.Name, &p.Description, &p.PriceIDR, &p.CostPriceIDR, &p.Diamonds, &p.ProductType, &p.SKU, &p.CustomerNoFormat, &p.IsActive, &p.Stock, &p.CreatedAt, &p.UpdatedAt, &p.DeletedAt); err != nil {
			return nil, err
		}
		products = append(products, p)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return products, nil
}

func (r *PGProductRepository) ListAll(ctx context.Context) ([]models.Product, error) {
	rows, err := r.pool.Query(ctx, `
		SELECT id, game, name, description, price_idr, cost_price_idr, diamonds, product_type, sku, customer_no_format, is_active, stock, created_at, updated_at, deleted_at
		FROM products WHERE deleted_at IS NULL ORDER BY game, price_idr
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var products []models.Product
	for rows.Next() {
		var p models.Product
		if err := rows.Scan(&p.ID, &p.Game, &p.Name, &p.Description, &p.PriceIDR, &p.CostPriceIDR, &p.Diamonds, &p.ProductType, &p.SKU, &p.CustomerNoFormat, &p.IsActive, &p.Stock, &p.CreatedAt, &p.UpdatedAt, &p.DeletedAt); err != nil {
			return nil, err
		}
		products = append(products, p)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return products, nil
}

func (r *PGProductRepository) DecrementStock(ctx context.Context, id string) (bool, error) {
	result, err := r.pool.Exec(ctx, `
		UPDATE products SET stock = stock - 1 WHERE id = $1 AND stock > 0
	`, id)
	if err != nil {
		return false, err
	}
	return result.RowsAffected() > 0, nil
}

func (r *PGProductRepository) IncrementStock(ctx context.Context, id string) error {
	_, err := r.pool.Exec(ctx, `
		UPDATE products SET stock = stock + 1 WHERE id = $1 AND stock >= 0
	`, id)
	return err
}

func (r *PGProductRepository) Create(ctx context.Context, p *models.Product) error {
	_, err := r.pool.Exec(ctx, `
		INSERT INTO products (id, game, name, description, price_idr, cost_price_idr, diamonds, product_type, sku, customer_no_format, is_active, stock)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12)
	`, p.ID, p.Game, p.Name, p.Description, p.PriceIDR, p.CostPriceIDR, p.Diamonds, p.ProductType, p.SKU, p.CustomerNoFormat, p.IsActive, p.Stock)
	return err
}

func (r *PGProductRepository) Update(ctx context.Context, p *models.Product) error {
	_, err := r.pool.Exec(ctx, `
		UPDATE products SET game=$1, name=$2, description=$3, price_idr=$4, cost_price_idr=$5, diamonds=$6, product_type=$7, sku=$8, customer_no_format=$9, is_active=$10, stock=$11, updated_at=NOW()
		WHERE id=$12
	`, p.Game, p.Name, p.Description, p.PriceIDR, p.CostPriceIDR, p.Diamonds, p.ProductType, p.SKU, p.CustomerNoFormat, p.IsActive, p.Stock, p.ID)
	return err
}

func (r *PGProductRepository) Delete(ctx context.Context, id string) error {
	_, err := r.pool.Exec(ctx, `UPDATE products SET deleted_at = NOW(), is_active = false WHERE id = $1`, id)
	return err
}

func (r *PGProductRepository) ExistsBySKU(ctx context.Context, sku string, excludeID string) (bool, error) {
	var exists bool
	var err error
	if excludeID != "" {
		err = r.pool.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM products WHERE sku = $1 AND id != $2 AND deleted_at IS NULL)`, sku, excludeID).Scan(&exists)
	} else {
		err = r.pool.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM products WHERE sku = $1 AND deleted_at IS NULL)`, sku).Scan(&exists)
	}
	return exists, err
}

func (r *PGProductRepository) UpdateCostPrice(ctx context.Context, sku string, costPrice int) error {
	_, err := r.pool.Exec(ctx, `UPDATE products SET cost_price_idr = $1, updated_at = NOW() WHERE sku = $2`, costPrice, sku)
	return err
}

func (r *PGProductRepository) SyncPrice(ctx context.Context, sku string, costPrice, sellingPrice int) error {
	_, err := r.pool.Exec(ctx, `UPDATE products SET cost_price_idr = $1, price_idr = $2, updated_at = NOW() WHERE sku = $3`, costPrice, sellingPrice, sku)
	return err
}

func (r *PGProductRepository) CreateFromDigiflazz(ctx context.Context, sku, name, game, productType string, priceIDR, costPriceIDR, diamonds int, description string) error {
	customerNoFormat := ""
	if constants.PipeServerCustomerNoGames[game] {
		customerNoFormat = "uid_server_pipe"
	}

	_, err := r.pool.Exec(ctx, `
		INSERT INTO products (id, game, name, description, price_idr, cost_price_idr, diamonds, product_type, sku, customer_no_format, is_active)
		VALUES (gen_random_uuid(), $1, $2, $3, $4, $5, $6, $7, $8, $9, true)
	`, game, name, description, priceIDR, costPriceIDR, diamonds, productType, sku, customerNoFormat)
	return err
}
