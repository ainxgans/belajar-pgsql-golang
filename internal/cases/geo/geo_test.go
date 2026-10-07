package geo

import (
	"encoding/json"
	"math/rand"
	"net/http/httptest"
	"strings"
	"testing"

	"pgsql-playground/internal/db"
	"pgsql-playground/internal/gen"
)

func TestNearbyOrderedByKm(t *testing.T) {
	ctx := t.Context()
	pool, err := db.Connect(ctx)
	if err != nil {
		t.Skipf("no database available: %v", err)
	}
	defer pool.Close()
	if err := db.Migrate(ctx, pool); err != nil {
		t.Fatalf("migrate: %v", err)
	}

	var count int
	_ = pool.QueryRow(ctx, "SELECT count(*) FROM sellers").Scan(&count)
	if count == 0 {
		rnd := rand.New(rand.NewSource(1))
		if err := gen.Generate(ctx, pool, rnd, gen.TableSellers, 50, false); err != nil {
			t.Fatalf("seed sellers: %v", err)
		}
	}

	// Jakarta cluster (see internal/gen/words.go cityCoords).
	req := httptest.NewRequest("GET", "/api/sellers/nearby?lat=-6.2088&lng=106.8456&radius_km=100&limit=20", nil)
	rec := httptest.NewRecorder()
	nearby(pool)(rec, req)

	if rec.Code != 200 {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}
	var sellers []nearbySeller
	if err := json.Unmarshal(rec.Body.Bytes(), &sellers); err != nil {
		t.Fatalf("decode: %v", err)
	}
	for i := 1; i < len(sellers); i++ {
		if sellers[i].Km < sellers[i-1].Km {
			t.Fatalf("km decreased at index %d: %v -> %v", i, sellers[i-1].Km, sellers[i].Km)
		}
	}
	if len(sellers) == 0 {
		t.Fatalf("expected at least one nearby seller for Jakarta cluster")
	}
	if !strings.Contains(sellers[0].Name, "Jakarta") {
		t.Fatalf("expected nearest seller to be a Jakarta Store, got %q", sellers[0].Name)
	}
}
