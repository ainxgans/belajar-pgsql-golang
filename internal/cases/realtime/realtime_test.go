package realtime

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"pgsql-playground/internal/db"
)

func TestNotifyOrderDeliversWithinOneSecond(t *testing.T) {
	ctx := t.Context()
	pool, err := db.Connect(ctx)
	if err != nil {
		t.Skipf("no database available: %v", err)
	}
	defer pool.Close()
	if err := db.Migrate(ctx, pool); err != nil {
		t.Fatalf("migrate: %v", err)
	}

	conn, err := pool.Acquire(ctx)
	if err != nil {
		t.Fatalf("acquire: %v", err)
	}
	defer conn.Release()

	if _, err := conn.Exec(ctx, "LISTEN orders"); err != nil {
		t.Fatalf("listen: %v", err)
	}

	var userID int64
	err = pool.QueryRow(ctx, `
		INSERT INTO users (email, name, created_at)
		VALUES ('test-realtime@example.com', 'Test User', now())
		ON CONFLICT DO NOTHING
		RETURNING id`).Scan(&userID)
	if err != nil {
		if err := pool.QueryRow(ctx, "SELECT id FROM users LIMIT 1").Scan(&userID); err != nil {
			t.Fatalf("no user available: %v", err)
		}
	}

	var orderID int64
	err = pool.QueryRow(ctx, `
		INSERT INTO orders (user_id, status, total, created_at)
		VALUES ($1, 'pending', 42.00, now())
		RETURNING id`, userID).Scan(&orderID)
	if err != nil {
		t.Fatalf("insert order: %v", err)
	}

	waitCtx, cancel := context.WithTimeout(ctx, time.Second)
	defer cancel()

	notification, err := conn.Conn().WaitForNotification(waitCtx)
	if err != nil {
		t.Fatalf("expected notification within 1s, got error: %v", err)
	}
	if notification.Channel != "orders" {
		t.Fatalf("channel = %q, want %q", notification.Channel, "orders")
	}

	var payload struct {
		ID     int64   `json:"id"`
		Status string  `json:"status"`
		Total  float64 `json:"total"`
	}
	if err := json.Unmarshal([]byte(notification.Payload), &payload); err != nil {
		t.Fatalf("decode payload %q: %v", notification.Payload, err)
	}
	if payload.ID != orderID {
		t.Fatalf("payload id = %d, want %d", payload.ID, orderID)
	}
	if !strings.EqualFold(payload.Status, "pending") {
		t.Fatalf("payload status = %q, want pending", payload.Status)
	}
}
