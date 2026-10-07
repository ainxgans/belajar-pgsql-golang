// Command api serves the pgsql-playground JSON API and its html/template frontend.
package main

import (
	"context"
	"log"
	"math/rand"
	"net/http"
	"os"
	"strings"

	"pgsql-playground/internal/cases/analytics"
	"pgsql-playground/internal/cases/fts"
	"pgsql-playground/internal/cases/geo"
	"pgsql-playground/internal/cases/jsonb"
	"pgsql-playground/internal/cases/perf"
	"pgsql-playground/internal/cases/realtime"
	"pgsql-playground/internal/cases/vector"
	"pgsql-playground/internal/db"
	"pgsql-playground/internal/gen"
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

	if os.Getenv("AUTO_SEED") == "true" {
		var count int
		_ = pool.QueryRow(ctx, "SELECT count(*) FROM products").Scan(&count)
		if count == 0 {
			log.Println("api: seeding database tables...")
			rnd := rand.New(rand.NewSource(1))
			if err := gen.Generate(ctx, pool, rnd, gen.TableUsers, 200, false); err != nil {
				log.Printf("api seed users err: %v", err)
			}
			if err := gen.Generate(ctx, pool, rnd, gen.TableCategories, 20, false); err != nil {
				log.Printf("api seed categories err: %v", err)
			}
			if err := gen.Generate(ctx, pool, rnd, gen.TableSellers, 50, false); err != nil {
				log.Printf("api seed sellers err: %v", err)
			}
			if err := gen.Generate(ctx, pool, rnd, gen.TableProducts, 300, false); err != nil {
				log.Printf("api seed products err: %v", err)
			}
			if err := gen.Generate(ctx, pool, rnd, gen.TableOrders, 500, false); err != nil {
				log.Printf("api seed orders err: %v", err)
			}
			if err := gen.Generate(ctx, pool, rnd, gen.TableOrderItems, 1500, false); err != nil {
				log.Printf("api seed order_items err: %v", err)
			}
			if err := gen.Generate(ctx, pool, rnd, gen.TableEvents, 2000, false); err != nil {
				log.Printf("api seed events err: %v", err)
			}
			if err := gen.Generate(ctx, pool, rnd, gen.TableEmbeddings, 0, false); err != nil {
				log.Printf("api seed embeddings err: %v", err)
			}
			log.Println("api: database seeding completed")
		}
	}

	basePath := strings.TrimSpace(os.Getenv("BASE_PATH"))
	basePath = strings.TrimSuffix(basePath, "/")
	if basePath != "" && !strings.HasPrefix(basePath, "/") {
		basePath = "/" + basePath
	}
	web.BasePath = basePath

	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, r *http.Request) {
		if err := pool.Ping(r.Context()); err != nil {
			http.Error(w, err.Error(), http.StatusServiceUnavailable)
			return
		}
		_, _ = w.Write([]byte("ok"))
	})
	mux.Handle("GET /static/", http.FileServerFS(web.StaticFS))
	mux.HandleFunc("GET /{$}", web.IndexHandler)

	fts.RegisterRoutes(mux, pool)
	jsonb.RegisterRoutes(mux, pool)
	perf.RegisterRoutes(mux, pool)
	analytics.RegisterRoutes(mux, pool)
	geo.RegisterRoutes(mux, pool)
	realtime.RegisterRoutes(mux, pool)
	vector.RegisterRoutes(mux, pool)

	var handler http.Handler = mux
	if basePath != "" {
		rootMux := http.NewServeMux()
		rootMux.Handle(basePath+"/", http.StripPrefix(basePath, mux))
		rootMux.HandleFunc("GET "+basePath, func(w http.ResponseWriter, r *http.Request) {
			http.Redirect(w, r, basePath+"/", http.StatusMovedPermanently)
		})
		rootMux.Handle("/", mux)
		handler = rootMux
	}

	addr := os.Getenv("ADDR")
	if addr == "" {
		addr = ":8080"
	}
	log.Printf("api: listening on %s (base path: %q)", addr, basePath)
	log.Fatal(http.ListenAndServe(addr, handler))
}
