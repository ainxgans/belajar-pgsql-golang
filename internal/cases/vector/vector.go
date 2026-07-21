// Package vector implements Case 07: semantic "similar products" search via
// pgvector cosine distance over a synthetic per-category embedding.
package vector

import (
	"net/http"
	"strconv"

	"github.com/jackc/pgx/v5/pgxpool"
	"pgsql-playground/internal/web"
)

// RegisterRoutes wires the /api/search/semantic and /api/products/list
// endpoints and the /ui/semantic page.
func RegisterRoutes(mux *http.ServeMux, pool *pgxpool.Pool) {
	mux.HandleFunc("GET /ui/semantic", func(w http.ResponseWriter, r *http.Request) {
		web.RenderPage(w, "semantic", nil)
	})
	mux.HandleFunc("GET /api/search/semantic", semanticSearch(pool))
	mux.HandleFunc("GET /api/products/list", listProducts(pool))
}

type similarProduct struct {
	ID   int64   `json:"id"`
	Name string  `json:"name"`
	Dist float64 `json:"dist"`
}

func semanticSearch(pool *pgxpool.Pool) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		limit := 10
		if v := r.URL.Query().Get("limit"); v != "" {
			n, err := strconv.Atoi(v)
			if err != nil {
				web.Error(w, http.StatusBadRequest, "limit must be an integer")
				return
			}
			limit = n
		}

		var query string
		var arg any
		if q := r.URL.Query().Get("q"); q != "" {
			// ponytail: tanpa model embedding nyata; keyword → centroid (avg embedding) kategori yang namanya cocok
			query = `
				WITH cat AS (SELECT id FROM categories WHERE name ILIKE '%' || $1 || '%' ORDER BY id LIMIT 1),
				     centroid AS (SELECT avg(embedding) AS e FROM products WHERE category_id = (SELECT id FROM cat))
				SELECT id, name, embedding <=> (SELECT e FROM centroid) AS dist
				FROM products WHERE (SELECT e FROM centroid) IS NOT NULL ORDER BY dist LIMIT $2`
			arg = q
		} else {
			productID, err := strconv.ParseInt(r.URL.Query().Get("product_id"), 10, 64)
			if err != nil {
				web.Error(w, http.StatusBadRequest, "product_id or q is required")
				return
			}
			query = `
				SELECT id, name, embedding <=> (SELECT embedding FROM products WHERE id = $1) AS dist
				FROM products WHERE id <> $1 ORDER BY dist LIMIT $2`
			arg = productID
		}

		rows, err := pool.Query(r.Context(), query, arg, limit)
		if err != nil {
			web.Error(w, http.StatusInternalServerError, err.Error())
			return
		}
		defer rows.Close()

		var results []similarProduct
		for rows.Next() {
			var p similarProduct
			if err := rows.Scan(&p.ID, &p.Name, &p.Dist); err != nil {
				web.Error(w, http.StatusInternalServerError, err.Error())
				return
			}
			results = append(results, p)
		}
		if err := rows.Err(); err != nil {
			web.Error(w, http.StatusInternalServerError, err.Error())
			return
		}
		web.JSON(w, http.StatusOK, results)
	}
}

type productListItem struct {
	ID         int64  `json:"id"`
	Name       string `json:"name"`
	CategoryID int64  `json:"category_id"`
}

func listProducts(pool *pgxpool.Pool) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		rows, err := pool.Query(r.Context(), `SELECT id, name, category_id FROM products ORDER BY id LIMIT 100`)
		if err != nil {
			web.Error(w, http.StatusInternalServerError, err.Error())
			return
		}
		defer rows.Close()

		var products []productListItem
		for rows.Next() {
			var p productListItem
			if err := rows.Scan(&p.ID, &p.Name, &p.CategoryID); err != nil {
				web.Error(w, http.StatusInternalServerError, err.Error())
				return
			}
			products = append(products, p)
		}
		if err := rows.Err(); err != nil {
			web.Error(w, http.StatusInternalServerError, err.Error())
			return
		}
		web.JSON(w, http.StatusOK, products)
	}
}
