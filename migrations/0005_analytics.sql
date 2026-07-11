CREATE MATERIALIZED VIEW mv_daily_revenue AS
SELECT date_trunc('day', created_at) AS day,
       count(*) AS orders, sum(total) AS revenue
FROM orders GROUP BY 1;
CREATE UNIQUE INDEX ON mv_daily_revenue(day);
