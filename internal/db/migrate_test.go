package db

import "testing"

// TestMigrateIdempotent checks that running Migrate twice against the same
// database is safe (second run applies nothing new). Requires DATABASE_URL
// pointing at a running Postgres; skips otherwise.
func TestMigrateIdempotent(t *testing.T) {
	ctx := t.Context()
	pool, err := Connect(ctx)
	if err != nil {
		t.Skipf("no database available: %v", err)
	}
	defer pool.Close()

	if err := Migrate(ctx, pool); err != nil {
		t.Fatalf("first migrate: %v", err)
	}
	if err := Migrate(ctx, pool); err != nil {
		t.Fatalf("second migrate: %v", err)
	}
}
