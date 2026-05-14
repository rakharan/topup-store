package seed

import (
	"context"
	"fmt"
	"io"

	"github.com/jackc/pgx/v5/pgxpool"
)

type Product struct {
	Game        string
	Name        string
	Description string
	PriceIDR    int
	Diamonds    int
	SKU         string
}

func DefaultProducts() []Product {
	return []Product{
		{Game: "free_fire", Name: "100 Diamonds", Description: "100 Free Fire Diamonds", PriceIDR: 15000, Diamonds: 100, SKU: "FF-100"},
		{Game: "free_fire", Name: "310 Diamonds", Description: "310 Free Fire Diamonds", PriceIDR: 45000, Diamonds: 310, SKU: "FF-310"},
		{Game: "free_fire", Name: "520 Diamonds", Description: "520 Free Fire Diamonds", PriceIDR: 75000, Diamonds: 520, SKU: "FF-520"},
		{Game: "mobile_legends", Name: "86 Weekly Diamonds", Description: "86 Mobile Legends Weekly Diamonds", PriceIDR: 20000, Diamonds: 86, SKU: "ML-86"},
		{Game: "mobile_legends", Name: "172 Weekly Diamonds", Description: "172 Mobile Legends Weekly Diamonds", PriceIDR: 40000, Diamonds: 172, SKU: "ML-172"},
		{Game: "mobile_legends", Name: "257 Weekly Diamonds", Description: "257 Mobile Legends Weekly Diamonds", PriceIDR: 60000, Diamonds: 257, SKU: "ML-257"},
		{Game: "pubg_mobile", Name: "60 UC", Description: "60 PUBG Mobile UC", PriceIDR: 15000, Diamonds: 60, SKU: "PUBG-60"},
		{Game: "pubg_mobile", Name: "180 UC", Description: "180 PUBG Mobile UC", PriceIDR: 45000, Diamonds: 180, SKU: "PUBG-180"},
		{Game: "pubg_mobile", Name: "325 UC", Description: "325 PUBG Mobile UC", PriceIDR: 75000, Diamonds: 325, SKU: "PUBG-325"},
	}
}

func Products(ctx context.Context, pool *pgxpool.Pool, out io.Writer) error {
	query := `INSERT INTO products (game, name, description, price_idr, diamonds, sku, is_active)
		VALUES ($1, $2, $3, $4, $5, $6, true)
		ON CONFLICT (sku) DO UPDATE SET
			game = EXCLUDED.game,
			name = EXCLUDED.name,
			description = EXCLUDED.description,
			price_idr = EXCLUDED.price_idr,
			diamonds = EXCLUDED.diamonds,
			is_active = true,
			deleted_at = NULL,
			updated_at = NOW()`

	for _, p := range DefaultProducts() {
		if _, err := pool.Exec(ctx, query, p.Game, p.Name, p.Description, p.PriceIDR, p.Diamonds, p.SKU); err != nil {
			return fmt.Errorf("seed product %s: %w", p.SKU, err)
		}
		if out != nil {
			fmt.Fprintf(out, "Inserted/updated product: %s (%s)\n", p.Name, p.SKU)
		}
	}

	if out != nil {
		fmt.Fprintln(out, "Seed completed successfully")
	}
	return nil
}
