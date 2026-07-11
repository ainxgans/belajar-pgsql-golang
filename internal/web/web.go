// Package web renders the html/template frontend shell shared by all cases.
package web

import (
	"embed"
	"encoding/json"
	"html/template"
	"net/http"
)

//go:embed templates/*.html
var templatesFS embed.FS

//go:embed static
var StaticFS embed.FS

// Case describes one learning case for the index page and nav.
type Case struct {
	Slug  string
	Title string
	Desc  string
}

// Cases lists every case page, appended to as each case is implemented.
var Cases = []Case{
	{Slug: "search", Title: "Full-Text Search", Desc: "tsvector ranking, trigram fuzzy match, autocomplete"},
	{Slug: "products", Title: "JSONB Atribut Dinamis", Desc: "filter atribut dinamis via containment (@>), facet count, generated column"},
	{Slug: "explain", Title: "Partisi & EXPLAIN Playground", Desc: "orders dipartisi per bulan, BRIN vs BTREE, jalankan EXPLAIN ANALYZE dari browser"},
	{Slug: "analytics", Title: "Analitik & Window Functions", Desc: "revenue time-series, top products per kategori, funnel, RFM, ringkasan GROUPING SETS"},
	{Slug: "nearby", Title: "Geospasial (earthdistance)", Desc: "cari seller terdekat dari koordinat via cube/earthdistance, GiST index"},
	{Slug: "realtime", Title: "Realtime LISTEN/NOTIFY", Desc: "push order baru ke browser via SSE, trigger pg_notify"},
	{Slug: "semantic", Title: "Vector Similarity (pgvector)", Desc: "produk mirip via embedding cosine distance, index HNSW"},
}

// RenderPage renders templates/<name>.html inside the shared layout.
func RenderPage(w http.ResponseWriter, name string, data any) {
	tmpl, err := template.ParseFS(templatesFS, "templates/layout.html", "templates/"+name+".html")
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if err := tmpl.ExecuteTemplate(w, "layout", data); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}

// IndexHandler renders the case list at "/".
func IndexHandler(w http.ResponseWriter, r *http.Request) {
	RenderPage(w, "index", Cases)
}

// JSON writes data as a JSON response body.
func JSON(w http.ResponseWriter, status int, data any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(data)
}

// Error writes a JSON {"error": msg} response.
func Error(w http.ResponseWriter, status int, msg string) {
	JSON(w, status, map[string]string{"error": msg})
}
