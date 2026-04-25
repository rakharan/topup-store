package main

import (
	"context"
	"fmt"
	"log"
	"os"

	"github.com/jackc/pgx/v5/pgxpool"
)

type product struct {
	Game        string
	Name        string
	Description string
	PriceIDR    int
	Diamonds    int
	SKU         string
}

func main() {
	dsn := os.Getenv("DATABASE_URL")
	if dsn == "" {
		log.Fatal("DATABASE_URL is required")
	}

	ctx := context.Background()
	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		log.Fatalf("failed to connect to database: %v", err)
	}
	defer pool.Close()

	if err := pool.Ping(ctx); err != nil {
		log.Fatalf("failed to ping database: %v", err)
	}

	products := []product{
		// Free Fire
		{Game: "free_fire", Name: "100 Diamonds", Description: "100 Free Fire Diamonds", PriceIDR: 15000, Diamonds: 100, SKU: "FF-100"},
		{Game: "free_fire", Name: "310 Diamonds", Description: "310 Free Fire Diamonds", PriceIDR: 45000, Diamonds: 310, SKU: "FF-310"},
		{Game: "free_fire", Name: "520 Diamonds", Description: "520 Free Fire Diamonds", PriceIDR: 75000, Diamonds: 520, SKU: "FF-520"},
		// Mobile Legends (Weekly Diamonds)
		{Game: "mobile_legends", Name: "86 Weekly Diamonds", Description: "86 Mobile Legends Weekly Diamonds", PriceIDR: 20000, Diamonds: 86, SKU: "ML-86"},
		{Game: "mobile_legends", Name: "172 Weekly Diamonds", Description: "172 Mobile Legends Weekly Diamonds", PriceIDR: 40000, Diamonds: 172, SKU: "ML-172"},
		{Game: "mobile_legends", Name: "257 Weekly Diamonds", Description: "257 Mobile Legends Weekly Diamonds", PriceIDR: 60000, Diamonds: 257, SKU: "ML-257"},
		// PUBG Mobile
		{Game: "pubg_mobile", Name: "60 UC", Description: "60 PUBG Mobile UC", PriceIDR: 15000, Diamonds: 60, SKU: "PUBG-60"},
		{Game: "pubg_mobile", Name: "180 UC", Description: "180 PUBG Mobile UC", PriceIDR: 45000, Diamonds: 180, SKU: "PUBG-180"},
		{Game: "pubg_mobile", Name: "325 UC", Description: "325 PUBG Mobile UC", PriceIDR: 75000, Diamonds: 325, SKU: "PUBG-325"},
	}

	query := `INSERT INTO products (game, name, description, price_idr, diamonds, sku)
		VALUES ($1, $2, $3, $4, $5, $6)`

	for _, p := range products {
		_, err := pool.Exec(ctx, query, p.Game, p.Name, p.Description, p.PriceIDR, p.Diamonds, p.SKU)
		if err != nil {
			log.Fatalf("failed to insert product %s: %v", p.SKU, err)
		}
		fmt.Printf("Inserted product: %s (%s)\n", p.Name, p.SKU)
	}

	fmt.Println("Seed completed successfully")
}
