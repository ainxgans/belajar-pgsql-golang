// Package geo implements Case 05: nearest sellers via cube/earthdistance.
package geo

import (
	"net/http"
	"strconv"

	"github.com/jackc/pgx/v5/pgxpool"
	"pgsql-playground/internal/web"
)

// RegisterRoutes wires /api/sellers/nearby and the /ui/nearby page.
func RegisterRoutes(mux *http.ServeMux, pool *pgxpool.Pool) {
	mux.HandleFunc("GET /ui/nearby", func(w http.ResponseWriter, r *http.Request) {
		web.RenderPage(w, "nearby", nil)
	})
	mux.HandleFunc("GET /api/sellers/nearby", nearby(pool))
}

type nearbySeller struct {
	ID   int64   `json:"id"`
	Name string  `json:"name"`
	Km   float64 `json:"km"`
}

func nearby(pool *pgxpool.Pool) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		q := r.URL.Query()

		lat, err := strconv.ParseFloat(q.Get("lat"), 64)
		if err != nil {
			web.Error(w, http.StatusBadRequest, "lat is required and must be numeric")
			return
		}
		lng, err := strconv.ParseFloat(q.Get("lng"), 64)
		if err != nil {
			web.Error(w, http.StatusBadRequest, "lng is required and must be numeric")
			return
		}

		radiusKm := 50.0
		if v := q.Get("radius_km"); v != "" {
			radiusKm, err = strconv.ParseFloat(v, 64)
			if err != nil {
				web.Error(w, http.StatusBadRequest, "radius_km must be numeric")
				return
			}
		}

		limit := 20
		if v := q.Get("limit"); v != "" {
			limit, err = strconv.Atoi(v)
			if err != nil {
				web.Error(w, http.StatusBadRequest, "limit must be an integer")
				return
			}
		}

		rows, err := pool.Query(r.Context(), `
			SELECT id, name, earth_distance(ll_to_earth($1,$2), ll_to_earth(lat,lng))/1000 AS km
			FROM sellers
			WHERE earth_box(ll_to_earth($1,$2), $3*1000) @> ll_to_earth(lat,lng)
			  AND earth_distance(ll_to_earth($1,$2), ll_to_earth(lat,lng)) <= $3*1000
			ORDER BY km LIMIT $4`, lat, lng, radiusKm, limit)
		if err != nil {
			web.Error(w, http.StatusInternalServerError, err.Error())
			return
		}
		defer rows.Close()

		sellers := []nearbySeller{}
		for rows.Next() {
			var s nearbySeller
			if err := rows.Scan(&s.ID, &s.Name, &s.Km); err != nil {
				web.Error(w, http.StatusInternalServerError, err.Error())
				return
			}
			sellers = append(sellers, s)
		}
		if err := rows.Err(); err != nil {
			web.Error(w, http.StatusInternalServerError, err.Error())
			return
		}
		web.JSON(w, http.StatusOK, sellers)
	}
}
