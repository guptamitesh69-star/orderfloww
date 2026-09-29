package orders

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"sync"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"github.com/Mitesh0007/orderflow-go/pkg/events"
)

const (
	StatusPending   = "pending"
	StatusConfirmed = "confirmed"
	StatusCancelled = "cancelled"
)

type Order struct {
	ID       string `json:"id"`
	ItemID   string `json:"item_id"`
	Quantity int    `json:"quantity"`
	Amount   int64  `json:"amount"`
	Currency string `json:"currency"`
	Status   string `json:"status"`
	Reason   string `json:"reason,omitempty"`
}

// Store is deliberately in-memory. OrderFlow's point is demonstrating
// event choreography and Saga-style rollback, not database persistence.
type Store struct {
	mu     sync.RWMutex
	orders map[string]*Order
}

func NewStore() *Store {
	return &Store{orders: make(map[string]*Order)}
}

func (s *Store) Create(order *Order) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.orders[order.ID] = order
}

func (s *Store) Get(id string) (*Order, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	order, ok := s.orders[id]
	return order, ok
}

func (s *Store) UpdateStatus(id, status, reason string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	order, ok := s.orders[id]
	if !ok {
		return false
	}
	order.Status = status
	order.Reason = reason
	return true
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

	r.Post("/orders", h.CreateOrder)
	r.Get("/orders/{id}", h.GetOrder)

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

type createOrderRequest struct {
	ItemID   string `json:"item_id"`
	Quantity int    `json:"quantity"`
	Amount   int64  `json:"amount"`
	Currency string `json:"currency"`
}

// CreateOrder records the order as pending and publishes OrderCreated.
func (h *Handler) CreateOrder(w http.ResponseWriter, r *http.Request) {
	body, err := io.ReadAll(r.Body)
	if err != nil {
		writeError(w, http.StatusBadRequest, "could not read request body")
		return
	}
	defer r.Body.Close()

	var req createOrderRequest
	if err := json.Unmarshal(body, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request payload")
		return
	}

	order := &Order{
		ID:       uuid.NewString(),
		ItemID:   req.ItemID,
		Quantity: req.Quantity,
		Amount:   req.Amount,
		Currency: req.Currency,
		Status:   StatusPending,
	}

	h.Store.Create(order)

	_, err = h.Publisher.Publish(r.Context(), events.StreamOrderCreated, events.OrderCreated{
		OrderID:  order.ID,
		ItemID:   order.ItemID,
		Quantity: order.Quantity,
		Amount:   order.Amount,
		Currency: order.Currency,
	})
	if err != nil {
		writeError(w, http.StatusInternalServerError, "could not publish order.created")
		return
	}

	writeJSON(w, http.StatusAccepted, order)
}

func (h *Handler) GetOrder(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	order, ok := h.Store.Get(id)
	if !ok {
		writeError(w, http.StatusNotFound, "order not found")
		return
	}
	writeJSON(w, http.StatusOK, order)
}

func (h *Handler) OnPaymentCaptured(ctx context.Context, msg events.Message) error {
	var event events.PaymentCaptured
	if err := json.Unmarshal(msg.Payload, &event); err != nil {
		return fmt.Errorf("unmarshal payment.captured: %w", err)
	}
	if !h.Store.UpdateStatus(event.OrderID, StatusConfirmed, "") {
		return nil
	}
	_, err := h.Publisher.Publish(ctx, events.StreamOrderConfirmed, events.OrderConfirmed{OrderID: event.OrderID})
	return err
}

func (h *Handler) OnPaymentFailed(ctx context.Context, msg events.Message) error {
	var event events.PaymentFailed
	if err := json.Unmarshal(msg.Payload, &event); err != nil {
		return fmt.Errorf("unmarshal payment.failed: %w", err)
	}
	if !h.Store.UpdateStatus(event.OrderID, StatusCancelled, event.Reason) {
		return nil
	}
	_, err := h.Publisher.Publish(ctx, events.StreamOrderCancelled, events.OrderCancelled{
		OrderID: event.OrderID,
		Reason:  event.Reason,
	})
	return err
}

func (h *Handler) OnStockUnavailable(ctx context.Context, msg events.Message) error {
	var event events.StockUnavailable
	if err := json.Unmarshal(msg.Payload, &event); err != nil {
		return fmt.Errorf("unmarshal stock_unavailable: %w", err)
	}
	if !h.Store.UpdateStatus(event.OrderID, StatusCancelled, event.Reason) {
		return nil
	}
	_, err := h.Publisher.Publish(ctx, events.StreamOrderCancelled, events.OrderCancelled{
		OrderID: event.OrderID,
		Reason:  event.Reason,
	})
	return err
}

func writeJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}

func writeError(w http.ResponseWriter, status int, message string) {
	writeJSON(w, status, map[string]string{"error": message})
}
