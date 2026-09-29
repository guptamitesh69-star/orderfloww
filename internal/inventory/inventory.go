package inventory

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"sync"

	"github.com/go-chi/chi/v5"

	"github.com/Mitesh0007/orderflow-go/pkg/events"
)

type reservation struct {
	ItemID   string
	Quantity int
}

// Store holds current stock levels and outstanding reservations.
type Store struct {
	mu           sync.Mutex
	stock        map[string]int
	reservations map[string]reservation
}

func NewStore(seed map[string]int) *Store {
	stock := make(map[string]int, len(seed))
	for item, quantity := range seed {
		stock[item] = quantity
	}
	return &Store{
		stock:        stock,
		reservations: make(map[string]reservation),
	}
}

func (s *Store) TryReserve(orderID, itemID string, quantity int) bool {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.stock[itemID] < quantity {
		return false
	}

	s.stock[itemID] -= quantity
	s.reservations[orderID] = reservation{
		ItemID:   itemID,
		Quantity: quantity,
	}
	return true
}

func (s *Store) Release(orderID string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()

	res, ok := s.reservations[orderID]
	if !ok {
		return false
	}

	s.stock[res.ItemID] += res.Quantity
	delete(s.reservations, orderID)
	return true
}

func (s *Store) Snapshot() map[string]int {
	s.mu.Lock()
	defer s.mu.Unlock()

	snapshot := make(map[string]int, len(s.stock))
	for item, quantity := range s.stock {
		snapshot[item] = quantity
	}
	return snapshot
}

type Handler struct {
	Store     *Store
	Publisher *events.Publisher
}

func NewHandler(store *Store, publisher *events.Publisher) *Handler {
	return &Handler{Store: store, Publisher: publisher}
}

func (h *Handler) Routes() http.Handler {
	r := chi.NewRouter()

	r.Get("/health", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	r.Get("/stock", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(h.Store.Snapshot())
	})

	return cors(r)
}

func cors(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type")

		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}

		next.ServeHTTP(w, r)
	})
}

func (h *Handler) OnOrderCreated(ctx context.Context, msg events.Message) error {
	var event events.OrderCreated
	if err := json.Unmarshal(msg.Payload, &event); err != nil {
		return fmt.Errorf("unmarshal order.created: %w", err)
	}

	if h.Store.TryReserve(event.OrderID, event.ItemID, event.Quantity) {
		_, err := h.Publisher.Publish(ctx, events.StreamStockReserved, events.StockReserved{
			OrderID:  event.OrderID,
			ItemID:   event.ItemID,
			Quantity: event.Quantity,
			Amount:   event.Amount,
			Currency: event.Currency,
		})
		return err
	}

	_, err := h.Publisher.Publish(ctx, events.StreamStockUnavailable, events.StockUnavailable{
		OrderID: event.OrderID,
		ItemID:  event.ItemID,
		Reason:  "insufficient stock",
	})
	return err
}

func (h *Handler) OnPaymentFailed(ctx context.Context, msg events.Message) error {
	var event events.PaymentFailed
	if err := json.Unmarshal(msg.Payload, &event); err != nil {
		return fmt.Errorf("unmarshal payment.failed: %w", err)
	}

	h.Store.Release(event.OrderID)
	return nil
}
