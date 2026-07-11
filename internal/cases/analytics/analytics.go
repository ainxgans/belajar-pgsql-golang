package analytics

import (
	"net/http"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"pgsql-playground/internal/web"
)

func RegisterRoutes(mux *http.ServeMux, pool *pgxpool.Pool) {
	mux.HandleFunc("GET /ui/analytics", func(w http.ResponseWriter, r *http.Request) {
		web.RenderPage(w, "analytics", nil)
	})
	mux.HandleFunc("GET /api/analytics/revenue", revenue(pool))
	mux.HandleFunc("GET /api/analytics/top-products", topProducts(pool))
	mux.HandleFunc("GET /api/analytics/funnel", funnel(pool))
	mux.HandleFunc("GET /api/analytics/rfm", rfm(pool))
	mux.HandleFunc("GET /api/analytics/summary", summary(pool))
}

func parseDate(v string) (time.Time, error) {
	if t, err := time.Parse(time.RFC3339, v); err == nil {
		return t, nil
	}
	return time.Parse("2006-01-02", v)
}

type revenuePoint struct {
	Bucket         time.Time `json:"bucket"`
	Orders         int64     `json:"orders"`
	Revenue        float64   `json:"revenue"`
	RunningRevenue float64   `json:"running_revenue"`
}

func revenue(pool *pgxpool.Pool) http.HandlerFunc {
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

		bucket := r.URL.Query().Get("bucket")
		if bucket == "" {
			bucket = "day"
		}
		switch bucket {
		case "day", "week", "month":
		default:
			web.Error(w, http.StatusBadRequest, "bucket must be day, week, or month")
			return
		}

		rows, err := pool.Query(r.Context(), `
			SELECT date_trunc($1, day) AS bucket,
			       SUM(orders) AS orders,
			       SUM(revenue) AS revenue,
			       SUM(SUM(revenue)) OVER (ORDER BY date_trunc($1, day)) AS running_revenue
			FROM mv_daily_revenue
			WHERE day >= $2 AND day < $3
			GROUP BY date_trunc($1, day)
			ORDER BY bucket`, bucket, from, to)
		if err != nil {
			web.Error(w, http.StatusInternalServerError, err.Error())
			return
		}
		defer rows.Close()

		points := []revenuePoint{}
		for rows.Next() {
			var p revenuePoint
			if err := rows.Scan(&p.Bucket, &p.Orders, &p.Revenue, &p.RunningRevenue); err != nil {
				web.Error(w, http.StatusInternalServerError, err.Error())
				return
			}
			points = append(points, p)
		}
		web.JSON(w, http.StatusOK, points)
	}
}

type topProduct struct {
	CategoryID   int64   `json:"category_id"`
	CategoryName string  `json:"category_name"`
	ProductID    int64   `json:"product_id"`
	ProductName  string  `json:"product_name"`
	MetricValue  float64 `json:"metric_value"`
	Rank         int64   `json:"rank"`
}

func topProducts(pool *pgxpool.Pool) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		by := r.URL.Query().Get("by")
		var metric string
		switch by {
		case "revenue":
			metric = "SUM(oi.qty * oi.price)"
		case "qty":
			metric = "SUM(oi.qty)"
		default:
			web.Error(w, http.StatusBadRequest, "by must be revenue or qty")
			return
		}

		if per := r.URL.Query().Get("per"); per != "" && per != "category" {
			// ponytail: only one partition dimension exists today, add a dimension map when a second one shows up.
			web.Error(w, http.StatusBadRequest, "per must be category")
			return
		}

		query := `
			WITH agg AS (
				SELECT p.category_id AS category_id, c.name AS category_name,
				       p.id AS product_id, p.name AS product_name,
				       ` + metric + ` AS metric_value
				FROM order_items oi
				JOIN products p ON p.id = oi.product_id
				JOIN categories c ON c.id = p.category_id
				GROUP BY p.category_id, c.name, p.id, p.name
			), ranked AS (
				SELECT *, RANK() OVER (PARTITION BY category_id ORDER BY metric_value DESC) AS rank
				FROM agg
			)
			SELECT category_id, category_name, product_id, product_name, metric_value, rank
			FROM ranked
			WHERE rank <= 10
			ORDER BY category_id, rank`

		rows, err := pool.Query(r.Context(), query)
		if err != nil {
			web.Error(w, http.StatusInternalServerError, err.Error())
			return
		}
		defer rows.Close()

		products := []topProduct{}
		for rows.Next() {
			var p topProduct
			if err := rows.Scan(&p.CategoryID, &p.CategoryName, &p.ProductID, &p.ProductName, &p.MetricValue, &p.Rank); err != nil {
				web.Error(w, http.StatusInternalServerError, err.Error())
				return
			}
			products = append(products, p)
		}
		web.JSON(w, http.StatusOK, products)
	}
}

type funnelResult struct {
	Views     int64 `json:"views"`
	Carts     int64 `json:"carts"`
	Purchases int64 `json:"purchases"`
}

func funnel(pool *pgxpool.Pool) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var f funnelResult
		err := pool.QueryRow(r.Context(), `
			SELECT count(*) FILTER (WHERE kind = 'view'),
			       count(*) FILTER (WHERE kind = 'cart'),
			       count(*) FILTER (WHERE kind = 'purchase')
			FROM events`).Scan(&f.Views, &f.Carts, &f.Purchases)
		if err != nil {
			web.Error(w, http.StatusInternalServerError, err.Error())
			return
		}
		web.JSON(w, http.StatusOK, f)
	}
}

type rfmRow struct {
	UserID         int64   `json:"user_id"`
	RecencyDays    float64 `json:"recency_days"`
	Frequency      int64   `json:"frequency"`
	Monetary       float64 `json:"monetary"`
	RecencyScore   int     `json:"recency_score"`
	FrequencyScore int     `json:"frequency_score"`
	MonetaryScore  int     `json:"monetary_score"`
}

func rfm(pool *pgxpool.Pool) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		rows, err := pool.Query(r.Context(), `
			WITH agg AS (
				SELECT user_id,
				       EXTRACT(EPOCH FROM (now() - max(created_at))) / 86400 AS recency_days,
				       count(*) AS frequency,
				       sum(total) AS monetary
				FROM orders
				GROUP BY user_id
			)
			SELECT user_id, recency_days, frequency, monetary,
			       NTILE(5) OVER (ORDER BY recency_days ASC) AS recency_score,
			       NTILE(5) OVER (ORDER BY frequency ASC) AS frequency_score,
			       NTILE(5) OVER (ORDER BY monetary ASC) AS monetary_score
			FROM agg
			ORDER BY user_id
			LIMIT 500`)
		if err != nil {
			web.Error(w, http.StatusInternalServerError, err.Error())
			return
		}
		defer rows.Close()

		result := []rfmRow{}
		for rows.Next() {
			var row rfmRow
			if err := rows.Scan(&row.UserID, &row.RecencyDays, &row.Frequency, &row.Monetary,
				&row.RecencyScore, &row.FrequencyScore, &row.MonetaryScore); err != nil {
				web.Error(w, http.StatusInternalServerError, err.Error())
				return
			}
			result = append(result, row)
		}
		web.JSON(w, http.StatusOK, result)
	}
}

type summaryRow struct {
	Status  *string `json:"status"`
	Orders  int64   `json:"orders"`
	Revenue float64 `json:"revenue"`
}

func summary(pool *pgxpool.Pool) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		rows, err := pool.Query(r.Context(), `
			SELECT status, count(*) AS orders, sum(total) AS revenue
			FROM orders
			GROUP BY GROUPING SETS ((status), ())
			ORDER BY status NULLS LAST`)
		if err != nil {
			web.Error(w, http.StatusInternalServerError, err.Error())
			return
		}
		defer rows.Close()

		result := []summaryRow{}
		for rows.Next() {
			var row summaryRow
			if err := rows.Scan(&row.Status, &row.Orders, &row.Revenue); err != nil {
				web.Error(w, http.StatusInternalServerError, err.Error())
				return
			}
			result = append(result, row)
		}
		web.JSON(w, http.StatusOK, result)
	}
}
