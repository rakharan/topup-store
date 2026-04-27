package repositories

import (
	"context"

	"github.com/jackc/pgx/v5/pgxpool"
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
		SELECT id, game, name, description, price_idr, cost_price_idr, diamonds, sku, is_active, created_at, updated_at, deleted_at
		FROM products WHERE id = $1
	`, id).Scan(&p.ID, &p.Game, &p.Name, &p.Description, &p.PriceIDR, &p.CostPriceIDR, &p.Diamonds, &p.SKU, &p.IsActive, &p.CreatedAt, &p.UpdatedAt, &p.DeletedAt)
	if err != nil {
		return nil, err
	}
	return &p, nil
}

func (r *PGProductRepository) GetByGameAndDiamonds(ctx context.Context, game string, diamonds int) (*models.Product, error) {
	var p models.Product
	err := r.pool.QueryRow(ctx, `
		SELECT id, game, name, description, price_idr, cost_price_idr, diamonds, sku, is_active, created_at, updated_at, deleted_at
		FROM products WHERE game = $1 AND diamonds = $2 AND is_active = true AND deleted_at IS NULL
	`, game, diamonds).Scan(&p.ID, &p.Game, &p.Name, &p.Description, &p.PriceIDR, &p.CostPriceIDR, &p.Diamonds, &p.SKU, &p.IsActive, &p.CreatedAt, &p.UpdatedAt, &p.DeletedAt)
	if err != nil {
		return nil, err
	}
	return &p, nil
}

func (r *PGProductRepository) ListByGame(ctx context.Context, game string) ([]models.Product, error) {
	rows, err := r.pool.Query(ctx, `
		SELECT id, game, name, description, price_idr, cost_price_idr, diamonds, sku, is_active, created_at, updated_at, deleted_at
		FROM products WHERE game = $1 AND is_active = true AND deleted_at IS NULL ORDER BY price_idr
	`, game)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var products []models.Product
	for rows.Next() {
		var p models.Product
		if err := rows.Scan(&p.ID, &p.Game, &p.Name, &p.Description, &p.PriceIDR, &p.CostPriceIDR, &p.Diamonds, &p.SKU, &p.IsActive, &p.CreatedAt, &p.UpdatedAt, &p.DeletedAt); err != nil {
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
		SELECT id, game, name, description, price_idr, cost_price_idr, diamonds, sku, is_active, created_at, updated_at, deleted_at
		FROM products WHERE deleted_at IS NULL ORDER BY game, price_idr
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var products []models.Product
	for rows.Next() {
		var p models.Product
		if err := rows.Scan(&p.ID, &p.Game, &p.Name, &p.Description, &p.PriceIDR, &p.CostPriceIDR, &p.Diamonds, &p.SKU, &p.IsActive, &p.CreatedAt, &p.UpdatedAt, &p.DeletedAt); err != nil {
			return nil, err
		}
		products = append(products, p)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return products, nil
}

func (r *PGProductRepository) Create(ctx context.Context, p *models.Product) error {
	_, err := r.pool.Exec(ctx, `
		INSERT INTO products (id, game, name, description, price_idr, cost_price_idr, diamonds, sku, is_active)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)
	`, p.ID, p.Game, p.Name, p.Description, p.PriceIDR, p.CostPriceIDR, p.Diamonds, p.SKU, p.IsActive)
	return err
}

func (r *PGProductRepository) Update(ctx context.Context, p *models.Product) error {
	_, err := r.pool.Exec(ctx, `
		UPDATE products SET game=$1, name=$2, description=$3, price_idr=$4, cost_price_idr=$5, diamonds=$6, sku=$7, is_active=$8, updated_at=NOW()
		WHERE id=$9
	`, p.Game, p.Name, p.Description, p.PriceIDR, p.CostPriceIDR, p.Diamonds, p.SKU, p.IsActive, p.ID)
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
