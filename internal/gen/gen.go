// Package gen generates reproducible fake e-commerce data and bulk-loads it
// via pgx.CopyFrom.
package gen

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"math/rand"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// dateSpread returns a random timestamp uniformly within the last spreadMonths
// months, anchored to now — matching the partition range built by 0004_orders.sql.
func dateSpread(rnd *rand.Rand, spreadMonths int) time.Time {
	hours := int64(spreadMonths) * 30 * 24
	return time.Now().Add(-time.Duration(rnd.Int63n(hours)) * time.Hour)
}

const batchSize = 50_000

// Table names supported by Generate.
const (
	TableUsers      = "users"
	TableCategories = "categories"
	TableSellers    = "sellers"
	TableProducts   = "products"
	TableReviews    = "reviews"
	TableOrders     = "orders"
	TableOrderItems = "order_items"
	TableEvents     = "events"
	TableEmbeddings = "embeddings"
)

// Generate creates `rows` rows of `table` using rnd for randomness, optionally
// truncating the table first, and bulk-loads them via CopyFrom.
func Generate(ctx context.Context, pool *pgxpool.Pool, rnd *rand.Rand, table string, rows int, truncate bool) error {
	if truncate {
		if _, err := pool.Exec(ctx, fmt.Sprintf("TRUNCATE TABLE %s RESTART IDENTITY CASCADE", table)); err != nil {
			return fmt.Errorf("gen: truncate %s: %w", table, err)
		}
	}

	switch table {
	case TableUsers:
		return genUsers(ctx, pool, rnd, rows)
	case TableCategories:
		return genCategories(ctx, pool, rnd, rows)
	case TableSellers:
		return genSellers(ctx, pool, rnd, rows)
	case TableProducts:
		return genProducts(ctx, pool, rnd, rows)
	case TableReviews:
		return genReviews(ctx, pool, rnd, rows)
	case TableOrders:
		return genOrders(ctx, pool, rnd, rows)
	case TableOrderItems:
		return genOrderItems(ctx, pool, rnd, rows)
	case TableEvents:
		return genEvents(ctx, pool, rnd, rows)
	case TableEmbeddings:
		return genEmbeddings(ctx, pool, rnd) // backfills an existing column; rows/truncate are ignored
	default:
		return fmt.Errorf("gen: unknown table %q", table)
	}
}

func copyBatches(ctx context.Context, pool *pgxpool.Pool, table string, columns []string, rows int, build func(i int) []any) error {
	for start := 0; start < rows; start += batchSize {
		end := min(start+batchSize, rows)
		batch := make([][]any, 0, end-start)
		for i := start; i < end; i++ {
			batch = append(batch, build(i))
		}
		n, err := pool.CopyFrom(ctx, pgx.Identifier{table}, columns, pgx.CopyFromRows(batch))
		if err != nil {
			return fmt.Errorf("gen: copy %s rows [%d,%d): %w", table, start, end, err)
		}
		fmt.Printf("gen: %s +%d (%d/%d)\n", table, n, end, rows)
	}
	return nil
}

func genUsers(ctx context.Context, pool *pgxpool.Pool, _ *rand.Rand, rows int) error {
	return copyBatches(ctx, pool, TableUsers, []string{"email"}, rows, func(i int) []any {
		return []any{fmt.Sprintf("user%d@example.com", i+1)}
	})
}

func genCategories(ctx context.Context, pool *pgxpool.Pool, _ *rand.Rand, rows int) error {
	// First len(categoryNames) rows are roots (parent NULL); extras nest under their
	// base category. Empty table + RESTART IDENTITY means row i gets id i+1, so a child
	// can safely reference its already-inserted base at id (i%len)+1. // ponytail: 2-level tree, cukup buat recursive CTE demo
	base := len(categoryNames)
	return copyBatches(ctx, pool, TableCategories, []string{"name", "parent_id"}, rows, func(i int) []any {
		name := categoryNames[i%base]
		var parentID any
		if i >= base {
			name = fmt.Sprintf("%s %d", name, i/base+1)
			parentID = int64(i%base + 1)
		}
		return []any{name, parentID}
	})
}

func genSellers(ctx context.Context, pool *pgxpool.Pool, rnd *rand.Rand, rows int) error {
	return copyBatches(ctx, pool, TableSellers, []string{"name", "lat", "lng"}, rows, func(i int) []any {
		city := cityNames[rnd.Intn(len(cityNames))]
		base := cityCoords[city]
		lat := base[0] + (rnd.Float64()-0.5)*0.2
		lng := base[1] + (rnd.Float64()-0.5)*0.2
		return []any{fmt.Sprintf("%s Store %d", city, i+1), lat, lng}
	})
}

func genProducts(ctx context.Context, pool *pgxpool.Pool, rnd *rand.Rand, rows int) error {
	sellerIDs, err := tableIDs(ctx, pool, TableSellers)
	if err != nil {
		return err
	}
	categoryIDs, err := tableIDs(ctx, pool, TableCategories)
	if err != nil {
		return err
	}
	if len(sellerIDs) == 0 || len(categoryIDs) == 0 {
		return fmt.Errorf("gen: products needs sellers and categories generated first")
	}

	ramOptions := []int{4, 8, 16, 32}
	storageOptions := []int{128, 256, 512, 1024}

	return copyBatches(ctx, pool, TableProducts,
		[]string{"seller_id", "category_id", "name", "description", "price", "attributes"}, rows, func(i int) []any {
			adj := adjectives[rnd.Intn(len(adjectives))]
			noun := nouns[rnd.Intn(len(nouns))]
			desc := descriptors[rnd.Intn(len(descriptors))]
			name := fmt.Sprintf("%s %s", adj, noun)
			description := fmt.Sprintf("%s, %s.", name, desc)
			price := float64(rnd.Intn(200_00)+1_00) / 100.0
			sellerID := sellerIDs[rnd.Intn(len(sellerIDs))]
			categoryID := categoryIDs[rnd.Intn(len(categoryIDs))]

			attrs := map[string]any{"brand": brands[rnd.Intn(len(brands))]}
			if categoryID%2 == 0 {
				attrs["color"] = colors[rnd.Intn(len(colors))]
				attrs["size"] = sizes[rnd.Intn(len(sizes))]
			} else {
				attrs["ram_gb"] = ramOptions[rnd.Intn(len(ramOptions))]
				attrs["storage_gb"] = storageOptions[rnd.Intn(len(storageOptions))]
			}
			attrsJSON, _ := json.Marshal(attrs) // map of string/int only, cannot fail

			return []any{sellerID, categoryID, name, description, price, attrsJSON}
		})
}

func genReviews(ctx context.Context, pool *pgxpool.Pool, rnd *rand.Rand, rows int) error {
	productIDs, err := tableIDs(ctx, pool, TableProducts)
	if err != nil {
		return err
	}
	userIDs, err := tableIDs(ctx, pool, TableUsers)
	if err != nil {
		return err
	}
	if len(productIDs) == 0 || len(userIDs) == 0 {
		return fmt.Errorf("gen: reviews needs products and users generated first")
	}

	return copyBatches(ctx, pool, TableReviews, []string{"product_id", "user_id", "body", "rating"}, rows, func(i int) []any {
		w1 := reviewWords[rnd.Intn(len(reviewWords))]
		w2 := reviewWords[rnd.Intn(len(reviewWords))]
		body := fmt.Sprintf("%s. %s.", w1, w2)
		rating := rnd.Intn(5) + 1
		productID := productIDs[rnd.Intn(len(productIDs))]
		userID := userIDs[rnd.Intn(len(userIDs))]
		return []any{productID, userID, body, rating}
	})
}

func genOrders(ctx context.Context, pool *pgxpool.Pool, rnd *rand.Rand, rows int) error {
	userIDs, err := tableIDs(ctx, pool, TableUsers)
	if err != nil {
		return err
	}
	if len(userIDs) == 0 {
		return fmt.Errorf("gen: orders needs users generated first")
	}

	return copyBatches(ctx, pool, TableOrders, []string{"user_id", "status", "total", "created_at"}, rows, func(i int) []any {
		userID := userIDs[rnd.Intn(len(userIDs))]
		status := orderStatuses[rnd.Intn(len(orderStatuses))]
		total := float64(rnd.Intn(500_00)+5_00) / 100.0
		createdAt := dateSpread(rnd, 24) // stays within the 24-months-back partition range
		return []any{userID, status, total, createdAt}
	})
}

func genOrderItems(ctx context.Context, pool *pgxpool.Pool, rnd *rand.Rand, rows int) error {
	orderIDs, err := tableIDs(ctx, pool, TableOrders)
	if err != nil {
		return err
	}
	productIDs, err := tableIDs(ctx, pool, TableProducts)
	if err != nil {
		return err
	}
	if len(orderIDs) == 0 || len(productIDs) == 0 {
		return fmt.Errorf("gen: order_items needs orders and products generated first")
	}

	return copyBatches(ctx, pool, TableOrderItems, []string{"order_id", "product_id", "qty", "price"}, rows, func(i int) []any {
		orderID := orderIDs[rnd.Intn(len(orderIDs))]
		productID := productIDs[rnd.Intn(len(productIDs))]
		qty := rnd.Intn(5) + 1
		price := float64(rnd.Intn(200_00)+1_00) / 100.0
		return []any{orderID, productID, qty, price}
	})
}

func genEvents(ctx context.Context, pool *pgxpool.Pool, rnd *rand.Rand, rows int) error {
	userIDs, err := tableIDs(ctx, pool, TableUsers)
	if err != nil {
		return err
	}
	productIDs, err := tableIDs(ctx, pool, TableProducts)
	if err != nil {
		return err
	}
	if len(userIDs) == 0 || len(productIDs) == 0 {
		return fmt.Errorf("gen: events needs users and products generated first")
	}

	return copyBatches(ctx, pool, TableEvents, []string{"user_id", "product_id", "kind", "created_at"}, rows, func(i int) []any {
		userID := userIDs[rnd.Intn(len(userIDs))]

		var kind string
		switch roll := rnd.Intn(10); {
		case roll <= 5:
			kind = eventKinds[0] // view
		case roll <= 8:
			kind = eventKinds[1] // cart
		default:
			kind = eventKinds[2] // purchase
		}

		var productID any
		if rnd.Intn(10) != 0 {
			productID = productIDs[rnd.Intn(len(productIDs))]
		}

		createdAt := dateSpread(rnd, 26)
		return []any{userID, productID, kind, createdAt}
	})
}

const embeddingDim = 128

// normalize L2-normalizes v in place.
func normalize(v []float64) {
	var sumSq float64
	for _, x := range v {
		sumSq += x * x
	}
	norm := math.Sqrt(sumSq)
	if norm == 0 {
		return
	}
	for i := range v {
		v[i] /= norm
	}
}

// vectorLiteral formats v as a pgvector text literal, e.g. "[0.1,0.2,...]".
func vectorLiteral(v []float64) string {
	parts := make([]string, len(v))
	for i, x := range v {
		parts[i] = fmt.Sprintf("%.6f", x)
	}
	return "[" + strings.Join(parts, ",") + "]"
}

// genEmbeddings backfills products.embedding: each category gets a random
// L2-normalized centroid, and each product's embedding is its category's
// centroid plus small gaussian noise, re-normalized. This clusters products
// by category in vector space so cosine similarity search returns
// same-category "similar products" without a real embedding model.
func genEmbeddings(ctx context.Context, pool *pgxpool.Pool, rnd *rand.Rand) error {
	catRows, err := pool.Query(ctx, "SELECT id FROM categories")
	if err != nil {
		return fmt.Errorf("gen: embeddings: select categories: %w", err)
	}
	centroids := map[int64][]float64{}
	for catRows.Next() {
		var id int64
		if err := catRows.Scan(&id); err != nil {
			catRows.Close()
			return fmt.Errorf("gen: embeddings: scan category: %w", err)
		}
		centroid := make([]float64, embeddingDim)
		for i := range centroid {
			centroid[i] = rnd.NormFloat64()
		}
		normalize(centroid)
		centroids[id] = centroid
	}
	catRows.Close()
	if err := catRows.Err(); err != nil {
		return fmt.Errorf("gen: embeddings: categories: %w", err)
	}

	prodRows, err := pool.Query(ctx, "SELECT id, category_id FROM products")
	if err != nil {
		return fmt.Errorf("gen: embeddings: select products: %w", err)
	}
	type product struct {
		id, categoryID int64
	}
	var products []product
	for prodRows.Next() {
		var p product
		if err := prodRows.Scan(&p.id, &p.categoryID); err != nil {
			prodRows.Close()
			return fmt.Errorf("gen: embeddings: scan product: %w", err)
		}
		products = append(products, p)
	}
	prodRows.Close()
	if err := prodRows.Err(); err != nil {
		return fmt.Errorf("gen: embeddings: products: %w", err)
	}

	batch := &pgx.Batch{}
	queuedCount := 0
	for _, p := range products {
		centroid, ok := centroids[p.categoryID]
		if !ok {
			continue
		}
		emb := make([]float64, embeddingDim)
		for i := range emb {
			emb[i] = centroid[i] + rnd.NormFloat64()*0.15
		}
		normalize(emb)
		batch.Queue("UPDATE products SET embedding = $1::vector WHERE id = $2", vectorLiteral(emb), p.id)
		queuedCount++
	}
	if queuedCount == 0 {
		return nil
	}
	br := pool.SendBatch(ctx, batch)
	defer func() { _ = br.Close() }()
	for i := 0; i < queuedCount; i++ {
		if _, err := br.Exec(); err != nil {
			return fmt.Errorf("gen: embeddings: update: %w", err)
		}
	}
	fmt.Printf("gen: embeddings +%d products\n", queuedCount)
	return nil
}

func tableIDs(ctx context.Context, pool *pgxpool.Pool, table string) ([]int64, error) {
	rows, err := pool.Query(ctx, fmt.Sprintf("SELECT id FROM %s", table))
	if err != nil {
		return nil, fmt.Errorf("gen: query ids %s: %w", table, err)
	}
	defer rows.Close()
	var ids []int64
	for rows.Next() {
		var id int64
		if err := rows.Scan(&id); err != nil {
			return nil, fmt.Errorf("gen: scan id %s: %w", table, err)
		}
		ids = append(ids, id)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("gen: ids %s: %w", table, err)
	}
	return ids, nil
}
