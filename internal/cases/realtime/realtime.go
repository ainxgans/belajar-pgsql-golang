package realtime

import (
	"context"
	"fmt"
	"log"
	"math/rand"
	"net/http"
	"sync"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"pgsql-playground/internal/web"
)

// ponytail: satu listener + fan-out; cukup untuk demo
type hub struct {
	mu      sync.Mutex
	clients map[chan string]struct{}
}

func newHub() *hub {
	return &hub{clients: make(map[chan string]struct{})}
}

func (h *hub) register() chan string {
	ch := make(chan string, 8)
	h.mu.Lock()
	h.clients[ch] = struct{}{}
	h.mu.Unlock()
	return ch
}

func (h *hub) unregister(ch chan string) {
	h.mu.Lock()
	delete(h.clients, ch)
	h.mu.Unlock()
}

func (h *hub) broadcast(payload string) {
	h.mu.Lock()
	defer h.mu.Unlock()
	for ch := range h.clients {
		select {
		case ch <- payload:
		default: // slow client, drop message rather than block the listener
		}
	}
}

// listenOrders holds a single dedicated pool connection LISTENing on the
// "orders" channel for the lifetime of the process, fanning out every
// NOTIFY payload to whatever SSE clients are currently registered.
func listenOrders(ctx context.Context, pool *pgxpool.Pool, h *hub) {
	for {
		if ctx.Err() != nil {
			return
		}
		conn, err := pool.Acquire(ctx)
		if err != nil {
			if ctx.Err() != nil {
				return
			}
			log.Printf("realtime: acquire listen conn: %v", err)
			time.Sleep(time.Second)
			continue
		}

		if _, err := conn.Exec(ctx, "LISTEN orders"); err != nil {
			log.Printf("realtime: LISTEN orders: %v", err)
			conn.Release()
			if ctx.Err() != nil {
				return
			}
			time.Sleep(time.Second)
			continue
		}

		for {
			notification, err := conn.Conn().WaitForNotification(ctx)
			if err != nil {
				conn.Release()
				if ctx.Err() != nil {
					return
				}
				log.Printf("realtime: wait for notification: %v", err)
				time.Sleep(time.Second)
				break
			}
			h.broadcast(notification.Payload)
		}
	}
}

func RegisterRoutes(mux *http.ServeMux, pool *pgxpool.Pool) {
	h := newHub()
	go listenOrders(context.Background(), pool, h)

	mux.HandleFunc("GET /ui/realtime", func(w http.ResponseWriter, r *http.Request) {
		web.RenderPage(w, "realtime", nil)
	})
	mux.HandleFunc("GET /api/events/stream", stream(h))
	mux.HandleFunc("POST /api/orders/simulate", simulate(pool))
}

func stream(h *hub) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		flusher, ok := w.(http.Flusher)
		if !ok {
			web.Error(w, http.StatusInternalServerError, "streaming unsupported")
			return
		}
		w.Header().Set("Content-Type", "text/event-stream")
		w.Header().Set("Cache-Control", "no-cache")
		w.Header().Set("Connection", "keep-alive")

		ch := h.register()
		defer h.unregister(ch)

		for {
			select {
			case <-r.Context().Done():
				return
			case payload := <-ch:
				fmt.Fprintf(w, "data: %s\n\n", payload)
				flusher.Flush()
			}
		}
	}
}

var simStatuses = []string{"pending", "paid", "shipped", "delivered", "cancelled"}

type simulatedOrder struct {
	ID     int64   `json:"id"`
	Status string  `json:"status"`
	Total  float64 `json:"total"`
}

func simulate(pool *pgxpool.Pool) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		status := simStatuses[rand.Intn(len(simStatuses))]
		total := float64(rand.Intn(49_000)+1_000) / 100.0 // 10.00-500.00

		var o simulatedOrder
		err := pool.QueryRow(r.Context(), `
			INSERT INTO orders (user_id, status, total, created_at)
			VALUES ((SELECT id FROM users ORDER BY random() LIMIT 1), $1, $2, now())
			RETURNING id, status, total`, status, total).Scan(&o.ID, &o.Status, &o.Total)
		if err != nil {
			web.Error(w, http.StatusInternalServerError, err.Error())
			return
		}
		web.JSON(w, http.StatusCreated, o)
	}
}
