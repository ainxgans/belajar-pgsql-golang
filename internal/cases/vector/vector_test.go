package vector

import (
	"encoding/json"
	"math/rand"
	"net/http/httptest"
	"testing"

	"pgsql-playground/internal/db"
	"pgsql-playground/internal/gen"
)

// TestSemanticSearchMajoritySameCategory checks the plan's acceptance
// criterion: for a source product, the majority of its top-5 "similar"
// results share its category_id (embeddings cluster by category centroid).
func TestSemanticSearchMajoritySameCategory(t *testing.T) {
	ctx := t.Context()
	pool, err := db.Connect(ctx)
	if err != nil {
		t.Skipf("no database available: %v", err)
	}
	defer pool.Close()
	if err := db.Migrate(ctx, pool); err != nil {
		t.Fatalf("migrate: %v", err)
	}

	const sourceID = 1
	var categoryID int64
	if err := pool.QueryRow(ctx, "SELECT category_id FROM products WHERE id = $1", sourceID).Scan(&categoryID); err != nil {
		rnd := rand.New(rand.NewSource(1))
		_ = gen.Generate(ctx, pool, rnd, gen.TableCategories, 20, false)
		_ = gen.Generate(ctx, pool, rnd, gen.TableSellers, 50, false)
		_ = gen.Generate(ctx, pool, rnd, gen.TableProducts, 300, false)
		_ = gen.Generate(ctx, pool, rnd, gen.TableEmbeddings, 0, false)
		if err := pool.QueryRow(ctx, "SELECT category_id FROM products WHERE id = $1", sourceID).Scan(&categoryID); err != nil {
			t.Skipf("product %d not available: %v", sourceID, err)
		}
	}

	req := httptest.NewRequest("GET", "/api/search/semantic?product_id=1&limit=5", nil)
	rec := httptest.NewRecorder()
	semanticSearch(pool)(rec, req)

	if rec.Code != 200 {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}
	var results []similarProduct
	if err := json.Unmarshal(rec.Body.Bytes(), &results); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(results) == 0 {
		t.Fatal("expected results, got none")
	}

	same := 0
	for _, r := range results {
		var cat int64
		if err := pool.QueryRow(ctx, "SELECT category_id FROM products WHERE id = $1", r.ID).Scan(&cat); err != nil {
			t.Fatalf("lookup category for product %d: %v", r.ID, err)
		}
		if cat == categoryID {
			same++
		}
	}
	if same < 3 {
		t.Fatalf("expected at least 3 of %d results in category %d, got %d", len(results), categoryID, same)
	}
}
