// Package jsonb implements Case 02: dynamic product attributes via JSONB
// containment queries and facet counts.
package jsonb

import (
	"encoding/json"
	"net/http"
	"strconv"
	"strings"

	"github.com/jackc/pgx/v5/pgxpool"
	"pgsql-playground/internal/web"
)

// RegisterRoutes wires the /api/products* endpoints and the /ui/products page.
func RegisterRoutes(mux *http.ServeMux, pool *pgxpool.Pool) {
	mux.HandleFunc("GET /ui/products", func(w http.ResponseWriter, r *http.Request) {
		web.RenderPage(w, "products", nil)
	})
	mux.HandleFunc("GET /api/products", listProducts(pool))
	mux.HandleFunc("GET /api/products/facets", facets(pool))
}

type product struct {
	ID         int64          `json:"id"`
	Name       string         `json:"name"`
	Price      float64        `json:"price"`
	Attributes map[string]any `json:"attributes"`
}

func listProducts(pool *pgxpool.Pool) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		attrs := map[string]string{}
		for key, values := range r.URL.Query() {
			if rest, ok := strings.CutPrefix(key, "attr."); ok && len(values) > 0 && values[0] != "" {
				attrs[rest] = values[0]
			}
		}
		attrsJSON, err := json.Marshal(attrs)
		if err != nil {
			web.Error(w, http.StatusInternalServerError, err.Error())
			return
		}

		query := `SELECT id, name, price, attributes FROM products WHERE attributes @> $1::jsonb`
		args := []any{attrsJSON}

		if v := r.URL.Query().Get("min_price"); v != "" {
			min, err := strconv.ParseFloat(v, 64)
			if err != nil {
				web.Error(w, http.StatusBadRequest, "min_price must be numeric")
				return
			}
			args = append(args, min)
			query += " AND price >= $" + strconv.Itoa(len(args))
		}
		if v := r.URL.Query().Get("max_price"); v != "" {
			max, err := strconv.ParseFloat(v, 64)
			if err != nil {
				web.Error(w, http.StatusBadRequest, "max_price must be numeric")
				return
			}
			args = append(args, max)
			query += " AND price <= $" + strconv.Itoa(len(args))
		}
		query += " ORDER BY id LIMIT 50"

		rows, err := pool.Query(r.Context(), query, args...)
		if err != nil {
			web.Error(w, http.StatusInternalServerError, err.Error())
			return
		}
		defer rows.Close()

		var products []product
		for rows.Next() {
			var p product
			var raw []byte
			if err := rows.Scan(&p.ID, &p.Name, &p.Price, &raw); err != nil {
				web.Error(w, http.StatusInternalServerError, err.Error())
				return
			}
			if err := json.Unmarshal(raw, &p.Attributes); err != nil {
				web.Error(w, http.StatusInternalServerError, err.Error())
				return
			}
			products = append(products, p)
		}
		web.JSON(w, http.StatusOK, products)
	}
}

type facetValue struct {
	Value string `json:"value"`
	Count int64  `json:"count"`
}

func facets(pool *pgxpool.Pool) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		categoryID, err := strconv.ParseInt(r.URL.Query().Get("category_id"), 10, 64)
		if err != nil {
			web.Error(w, http.StatusBadRequest, "category_id is required")
			return
		}

		rows, err := pool.Query(r.Context(), `
			SELECT key, value, count(*)
			FROM products, jsonb_each_text(attributes)
			WHERE category_id = $1
			GROUP BY key, value
			ORDER BY key, count(*) DESC`, categoryID)
		if err != nil {
			web.Error(w, http.StatusInternalServerError, err.Error())
			return
		}
		defer rows.Close()

		result := map[string][]facetValue{}
		for rows.Next() {
			var key, value string
			var count int64
			if err := rows.Scan(&key, &value, &count); err != nil {
				web.Error(w, http.StatusInternalServerError, err.Error())
				return
			}
			result[key] = append(result[key], facetValue{Value: value, Count: count})
		}
		web.JSON(w, http.StatusOK, result)
	}
}
