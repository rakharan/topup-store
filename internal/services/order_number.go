package services

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

func GenerateOrderNumber(ctx context.Context, pool *pgxpool.Pool) (string, error) {
	today := time.Now().Format("2006-01-02")
	dateKey := time.Now().Format("20060102")

	var counter int
	err := pool.QueryRow(ctx, `
		INSERT INTO daily_counters (date, counter) VALUES ($1, 0)
		ON CONFLICT (date) DO UPDATE SET counter = daily_counters.counter + 1
		RETURNING counter
	`, today).Scan(&counter)
	if err != nil {
		return "", fmt.Errorf("generate order number: %w", err)
	}

	return fmt.Sprintf("FT-%s-%04d", dateKey, counter), nil
}
