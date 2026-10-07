// Package db wires up the pgxpool connection and runs SQL migrations.
package db

import (
	"context"
	"fmt"
	"os"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

// Connect opens a pgxpool using DATABASE_URL from the environment.
func Connect(ctx context.Context) (*pgxpool.Pool, error) {
	url := os.Getenv("DATABASE_URL")
	if url == "" {
		url = "postgres://app:app@localhost:5432/playground"
	}
	pool, err := pgxpool.New(ctx, url)
	if err != nil {
		return nil, fmt.Errorf("db: connect: %w", err)
	}
	var pingErr error
	for i := 0; i < 30; i++ {
		if err := pool.Ping(ctx); err == nil {
			return pool, nil
		} else {
			pingErr = err
			time.Sleep(1 * time.Second)
		}
	}
	pool.Close()
	return nil, fmt.Errorf("db: ping: %w", pingErr)
}
