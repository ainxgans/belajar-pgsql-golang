// Command api serves the pgsql-playground JSON API and its html/template frontend.
package main

import (
	"context"
	"log"
	"net/http"
	"os"

	"pgsql-playground/internal/cases/fts"
	"pgsql-playground/internal/cases/jsonb"
	"pgsql-playground/internal/db"
	"pgsql-playground/internal/web"
)

func main() {
	ctx := context.Background()
	pool, err := db.Connect(ctx)
	if err != nil {
		log.Fatalf("api: %v", err)
	}
	defer pool.Close()

	if err := db.Migrate(ctx, pool); err != nil {
		log.Fatalf("api: %v", err)
	}

	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, r *http.Request) {
		if err := pool.Ping(r.Context()); err != nil {
			http.Error(w, err.Error(), http.StatusServiceUnavailable)
			return
		}
		w.Write([]byte("ok"))
	})
	mux.Handle("GET /static/", http.FileServerFS(web.StaticFS))
	mux.HandleFunc("GET /{$}", web.IndexHandler)

	fts.RegisterRoutes(mux, pool)
	jsonb.RegisterRoutes(mux, pool)

	addr := os.Getenv("ADDR")
	if addr == "" {
		addr = ":8080"
	}
	log.Printf("api: listening on %s", addr)
	log.Fatal(http.ListenAndServe(addr, mux))
}
