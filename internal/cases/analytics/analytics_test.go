package analytics

import (
	"encoding/json"
	"net/http/httptest"
	"testing"

	"pgsql-playground/internal/db"
)

func TestRevenueRunningTotalMonotonic(t *testing.T) {
	ctx := t.Context()
	pool, err := db.Connect(ctx)
	if err != nil {
		t.Skipf("no database available: %v", err)
	}
	defer pool.Close()
	if err := db.Migrate(ctx, pool); err != nil {
		t.Fatalf("migrate: %v", err)
	}

	req := httptest.NewRequest("GET", "/api/analytics/revenue", nil)
	rec := httptest.NewRecorder()
	revenue(pool)(rec, req)

	if rec.Code != 200 {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}
	var points []revenuePoint
	if err := json.Unmarshal(rec.Body.Bytes(), &points); err != nil {
		t.Fatalf("decode: %v", err)
	}
	for i := 1; i < len(points); i++ {
		if points[i].RunningRevenue < points[i-1].RunningRevenue {
			t.Fatalf("running_revenue decreased at index %d: %v -> %v", i, points[i-1].RunningRevenue, points[i].RunningRevenue)
		}
	}
}

func TestFunnelOrdering(t *testing.T) {
	ctx := t.Context()
	pool, err := db.Connect(ctx)
	if err != nil {
		t.Skipf("no database available: %v", err)
	}
	defer pool.Close()
	if err := db.Migrate(ctx, pool); err != nil {
		t.Fatalf("migrate: %v", err)
	}

	req := httptest.NewRequest("GET", "/api/analytics/funnel", nil)
	rec := httptest.NewRecorder()
	funnel(pool)(rec, req)

	if rec.Code != 200 {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}
	var f funnelResult
	if err := json.Unmarshal(rec.Body.Bytes(), &f); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if f.Views < f.Carts || f.Carts < f.Purchases {
		t.Fatalf("expected views >= carts >= purchases, got %+v", f)
	}
}
