// Package perf implements Case 03: partitioned orders, index playground, and
// an EXPLAIN ANALYZE runner.
package perf

import (
	"encoding/json"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"pgsql-playground/internal/web"
)

// RegisterRoutes wires /api/explain, /api/orders, and the /ui/explain page.
func RegisterRoutes(mux *http.ServeMux, pool *pgxpool.Pool) {
	mux.HandleFunc("GET /ui/explain", func(w http.ResponseWriter, r *http.Request) {
		web.RenderPage(w, "explain", nil)
	})
	mux.HandleFunc("POST /api/explain", explain(pool))
	mux.HandleFunc("GET /api/orders", listOrders(pool))
}

type explainRequest struct {
	SQL string `json:"sql"`
}

// isSingleSelect rejects anything but one SELECT statement — this is a local
// learning tool with one operator, not a multi-tenant service, so a simple
// prefix/statement-count check is enough.
func isSingleSelect(sql string) bool {
	trimmed := strings.TrimSpace(sql)
	trimmed = strings.TrimSuffix(trimmed, ";")
	if !strings.HasPrefix(strings.ToUpper(trimmed), "SELECT ") && strings.ToUpper(trimmed) != "SELECT" {
		return false
	}
	return !strings.Contains(trimmed, ";")
}

func explain(pool *pgxpool.Pool) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req explainRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			web.Error(w, http.StatusBadRequest, "invalid JSON body")
			return
		}
		if !isSingleSelect(req.SQL) {
			web.Error(w, http.StatusBadRequest, "only a single SELECT statement is allowed")
			return
		}

		var plan json.RawMessage
		err := pool.QueryRow(r.Context(), "EXPLAIN (ANALYZE, BUFFERS, FORMAT JSON) "+req.SQL).Scan(&plan)
		if err != nil {
			web.Error(w, http.StatusBadRequest, err.Error())
			return
		}
		web.JSON(w, http.StatusOK, plan)
	}
}

type order struct {
	ID        int64     `json:"id"`
	UserID    int64     `json:"user_id"`
	Status    string    `json:"status"`
	Total     float64   `json:"total"`
	CreatedAt time.Time `json:"created_at"`
}

func listOrders(pool *pgxpool.Pool) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		from := time.Now().AddDate(0, 0, -30)
		to := time.Now()
		if v := r.URL.Query().Get("from"); v != "" {
			parsed, err := parseDate(v)
			if err != nil {
				web.Error(w, http.StatusBadRequest, "from must be YYYY-MM-DD or RFC3339")
				return
			}
			from = parsed
		}
		if v := r.URL.Query().Get("to"); v != "" {
			parsed, err := parseDate(v)
			if err != nil {
				web.Error(w, http.StatusBadRequest, "to must be YYYY-MM-DD or RFC3339")
				return
			}
			to = parsed
		}

		query := `SELECT id, user_id, status, total, created_at FROM orders WHERE created_at >= $1 AND created_at < $2`
		args := []any{from, to}

		if v := r.URL.Query().Get("user_id"); v != "" {
			userID, err := strconv.ParseInt(v, 10, 64)
			if err != nil {
				web.Error(w, http.StatusBadRequest, "user_id must be an integer")
				return
			}
			args = append(args, userID)
			query += " AND user_id = $" + strconv.Itoa(len(args))
		}
		query += " ORDER BY created_at DESC LIMIT 100"

		rows, err := pool.Query(r.Context(), query, args...)
		if err != nil {
			web.Error(w, http.StatusInternalServerError, err.Error())
			return
		}
		defer rows.Close()

		var orders []order
		for rows.Next() {
			var o order
			if err := rows.Scan(&o.ID, &o.UserID, &o.Status, &o.Total, &o.CreatedAt); err != nil {
				web.Error(w, http.StatusInternalServerError, err.Error())
				return
			}
			orders = append(orders, o)
		}
		web.JSON(w, http.StatusOK, orders)
	}
}

func parseDate(v string) (time.Time, error) {
	if t, err := time.Parse(time.RFC3339, v); err == nil {
		return t, nil
	}
	return time.Parse("2006-01-02", v)
}
