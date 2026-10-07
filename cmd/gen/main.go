// Command gen bulk-generates reproducible fake data into the playground database.
package main

import (
	"context"
	"flag"
	"log"
	"math/rand"

	"pgsql-playground/internal/db"
	"pgsql-playground/internal/gen"
)

func main() {
	table := flag.String("table", "", "table to generate: all|users|categories|sellers|products|reviews|orders|order_items|events|embeddings")
	rows := flag.Int("rows", 1000, "number of rows to generate")
	seed := flag.Int64("seed", 1, "random seed (same seed = same data)")
	truncate := flag.Bool("truncate", false, "truncate the table before generating")
	flag.Parse()

	if *table == "" {
		log.Fatal("gen: -table is required")
	}

	ctx := context.Background()
	pool, err := db.Connect(ctx)
	if err != nil {
		log.Fatalf("gen: %v", err)
	}
	defer pool.Close()

	if err := db.Migrate(ctx, pool); err != nil {
		log.Fatalf("gen: migrate: %v", err)
	}

	rnd := rand.New(rand.NewSource(*seed))
	if *table == "all" {
		if err := gen.SeedAll(ctx, pool, rnd); err != nil {
			log.Fatalf("gen: %v", err)
		}
		return
	}

	if err := gen.Generate(ctx, pool, rnd, *table, *rows, *truncate); err != nil {
		log.Fatalf("gen: %v", err)
	}
}
