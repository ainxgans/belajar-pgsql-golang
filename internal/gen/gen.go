// Package gen generates reproducible fake e-commerce data and bulk-loads it
// via pgx.CopyFrom.
package gen

import (
	"context"
	"encoding/json"
	"fmt"
	"math/rand"
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
	return copyBatches(ctx, pool, TableCategories, []string{"name", "parent_id"}, rows, func(i int) []any {
		name := categoryNames[i%len(categoryNames)]
		if i >= len(categoryNames) {
			name = fmt.Sprintf("%s %d", name, i/len(categoryNames)+1)
		}
		return []any{name, nil}
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
	sellerCount, err := tableCount(ctx, pool, TableSellers)
	if err != nil {
		return err
	}
	categoryCount, err := tableCount(ctx, pool, TableCategories)
	if err != nil {
		return err
	}
	if sellerCount == 0 || categoryCount == 0 {
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
			sellerID := rnd.Int63n(sellerCount) + 1
			categoryID := rnd.Int63n(categoryCount) + 1

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
	productCount, err := tableCount(ctx, pool, TableProducts)
	if err != nil {
		return err
	}
	userCount, err := tableCount(ctx, pool, TableUsers)
	if err != nil {
		return err
	}
	if productCount == 0 || userCount == 0 {
		return fmt.Errorf("gen: reviews needs products and users generated first")
	}

	return copyBatches(ctx, pool, TableReviews, []string{"product_id", "user_id", "body", "rating"}, rows, func(i int) []any {
		w1 := reviewWords[rnd.Intn(len(reviewWords))]
		w2 := reviewWords[rnd.Intn(len(reviewWords))]
		body := fmt.Sprintf("%s. %s.", w1, w2)
		rating := rnd.Intn(5) + 1
		productID := rnd.Int63n(productCount) + 1
		userID := rnd.Int63n(userCount) + 1
		return []any{productID, userID, body, rating}
	})
}

func genOrders(ctx context.Context, pool *pgxpool.Pool, rnd *rand.Rand, rows int) error {
	userCount, err := tableCount(ctx, pool, TableUsers)
	if err != nil {
		return err
	}
	if userCount == 0 {
		return fmt.Errorf("gen: orders needs users generated first")
	}

	return copyBatches(ctx, pool, TableOrders, []string{"user_id", "status", "total", "created_at"}, rows, func(i int) []any {
		userID := rnd.Int63n(userCount) + 1
		status := orderStatuses[rnd.Intn(len(orderStatuses))]
		total := float64(rnd.Intn(500_00)+5_00) / 100.0
		createdAt := dateSpread(rnd, 24) // stays within the 24-months-back partition range
		return []any{userID, status, total, createdAt}
	})
}

func genOrderItems(ctx context.Context, pool *pgxpool.Pool, rnd *rand.Rand, rows int) error {
	orderCount, err := tableCount(ctx, pool, TableOrders)
	if err != nil {
		return err
	}
	productCount, err := tableCount(ctx, pool, TableProducts)
	if err != nil {
		return err
	}
	if orderCount == 0 || productCount == 0 {
		return fmt.Errorf("gen: order_items needs orders and products generated first")
	}

	return copyBatches(ctx, pool, TableOrderItems, []string{"order_id", "product_id", "qty", "price"}, rows, func(i int) []any {
		orderID := rnd.Int63n(orderCount) + 1
		productID := rnd.Int63n(productCount) + 1
		qty := rnd.Intn(5) + 1
		price := float64(rnd.Intn(200_00)+1_00) / 100.0
		return []any{orderID, productID, qty, price}
	})
}

func genEvents(ctx context.Context, pool *pgxpool.Pool, rnd *rand.Rand, rows int) error {
	userCount, err := tableCount(ctx, pool, TableUsers)
	if err != nil {
		return err
	}
	productCount, err := tableCount(ctx, pool, TableProducts)
	if err != nil {
		return err
	}
	if userCount == 0 || productCount == 0 {
		return fmt.Errorf("gen: events needs users and products generated first")
	}

	return copyBatches(ctx, pool, TableEvents, []string{"user_id", "product_id", "kind", "created_at"}, rows, func(i int) []any {
		userID := rnd.Int63n(userCount) + 1

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
			productID = rnd.Int63n(productCount) + 1
		}

		createdAt := dateSpread(rnd, 26)
		return []any{userID, productID, kind, createdAt}
	})
}

func tableCount(ctx context.Context, pool *pgxpool.Pool, table string) (int64, error) {
	var count int64
	err := pool.QueryRow(ctx, fmt.Sprintf("SELECT count(*) FROM %s", table)).Scan(&count)
	if err != nil {
		return 0, fmt.Errorf("gen: count %s: %w", table, err)
	}
	return count, nil
}
