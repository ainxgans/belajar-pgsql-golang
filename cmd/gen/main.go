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
	table := flag.String("table", "", "table to generate: users|categories|sellers|products|reviews")
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

	rnd := rand.New(rand.NewSource(*seed))
	if err := gen.Generate(ctx, pool, rnd, *table, *rows, *truncate); err != nil {
		log.Fatalf("gen: %v", err)
	}
}
