// Package fts implements Case 01: full-text search, trigram fuzzy match, and autocomplete.
package fts

import (
	"net/http"
	"strconv"

	"github.com/jackc/pgx/v5/pgxpool"
	"pgsql-playground/internal/web"
)

// RegisterRoutes wires the /api/search* endpoints and the /ui/search page.
func RegisterRoutes(mux *http.ServeMux, pool *pgxpool.Pool) {
	mux.HandleFunc("GET /ui/search", func(w http.ResponseWriter, r *http.Request) {
		web.RenderPage(w, "search", nil)
	})
	mux.HandleFunc("GET /api/search", search(pool))
	mux.HandleFunc("GET /api/search/fuzzy", searchFuzzy(pool))
	mux.HandleFunc("GET /api/autocomplete", autocomplete(pool))
}

type hit struct {
	ID        int64   `json:"id"`
	Name      string  `json:"name"`
	Headline  string  `json:"headline,omitempty"`
	Price     float64 `json:"price"`
	Rank      float64 `json:"rank,omitempty"`
	Similarity float64 `json:"similarity,omitempty"`
}

func limitParam(r *http.Request, def int) int {
	if v, err := strconv.Atoi(r.URL.Query().Get("limit")); err == nil && v > 0 && v <= 100 {
		return v
	}
	return def
}

func search(pool *pgxpool.Pool) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		q := r.URL.Query().Get("q")
		if q == "" {
			web.Error(w, http.StatusBadRequest, "q is required")
			return
		}
		rows, err := pool.Query(r.Context(), `
			SELECT id, name, price,
			       ts_headline('simple', name, websearch_to_tsquery('simple', $1)) AS headline,
			       ts_rank(search, websearch_to_tsquery('simple', $1)) AS rank
			FROM products
			WHERE search @@ websearch_to_tsquery('simple', $1)
			ORDER BY rank DESC
			LIMIT $2`, q, limitParam(r, 20))
		if err != nil {
			web.Error(w, http.StatusInternalServerError, err.Error())
			return
		}
		defer rows.Close()

		var hits []hit
		for rows.Next() {
			var h hit
			if err := rows.Scan(&h.ID, &h.Name, &h.Price, &h.Headline, &h.Rank); err != nil {
				web.Error(w, http.StatusInternalServerError, err.Error())
				return
			}
			hits = append(hits, h)
		}
		if err := rows.Err(); err != nil {
			web.Error(w, http.StatusInternalServerError, err.Error())
			return
		}
		web.JSON(w, http.StatusOK, hits)
	}
}

func searchFuzzy(pool *pgxpool.Pool) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		q := r.URL.Query().Get("q")
		if q == "" {
			web.Error(w, http.StatusBadRequest, "q is required")
			return
		}
		rows, err := pool.Query(r.Context(), `
			SELECT id, name, price, similarity(name, $1) AS similarity
			FROM products
			WHERE name % $1
			ORDER BY similarity DESC
			LIMIT $2`, q, limitParam(r, 20))
		if err != nil {
			web.Error(w, http.StatusInternalServerError, err.Error())
			return
		}
		defer rows.Close()

		var hits []hit
		for rows.Next() {
			var h hit
			if err := rows.Scan(&h.ID, &h.Name, &h.Price, &h.Similarity); err != nil {
				web.Error(w, http.StatusInternalServerError, err.Error())
				return
			}
			hits = append(hits, h)
		}
		if err := rows.Err(); err != nil {
			web.Error(w, http.StatusInternalServerError, err.Error())
			return
		}
		web.JSON(w, http.StatusOK, hits)
	}
}

func autocomplete(pool *pgxpool.Pool) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		prefix := r.URL.Query().Get("prefix")
		if prefix == "" {
			web.Error(w, http.StatusBadRequest, "prefix is required")
			return
		}
		rows, err := pool.Query(r.Context(), `
			SELECT id, name, price
			FROM products
			WHERE name ILIKE $1 || '%'
			ORDER BY name
			LIMIT 10`, prefix)
		if err != nil {
			web.Error(w, http.StatusInternalServerError, err.Error())
			return
		}
		defer rows.Close()

		var hits []hit
		for rows.Next() {
			var h hit
			if err := rows.Scan(&h.ID, &h.Name, &h.Price); err != nil {
				web.Error(w, http.StatusInternalServerError, err.Error())
				return
			}
			hits = append(hits, h)
		}
		if err := rows.Err(); err != nil {
			web.Error(w, http.StatusInternalServerError, err.Error())
			return
		}
		web.JSON(w, http.StatusOK, hits)
	}
}
