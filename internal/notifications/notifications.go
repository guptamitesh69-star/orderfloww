package notifications

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"

	"github.com/go-chi/chi/v5"

	"github.com/Mitesh0007/orderflow-go/pkg/events"
)

type Handler struct{}

func NewHandler() *Handler {
	return &Handler{}
}

func (h *Handler) Routes() http.Handler {
	r := chi.NewRouter()

	r.Get("/health", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	return r
}


func (h *Handler) OnOrderConfirmed(
	ctx context.Context,
	msg events.Message,
) error {
	var event events.OrderConfirmed
	if err := json.Unmarshal(msg.Payload, &event); err != nil {
		return fmt.Errorf("unmarshal order.confirmed: %w", err)
	}

	log.Printf("notification: order %s confirmed — would notify customer", event.OrderID)

	return nil
}

func (h *Handler) OnOrderCancelled(
	ctx context.Context,
	msg events.Message,
) error {
	var event events.OrderCancelled
	if err := json.Unmarshal(msg.Payload, &event); err != nil {
		return fmt.Errorf("unmarshal order.cancelled: %w", err)
	}

	log.Printf(
		"notification: order %s cancelled (%s) — would notify customer",
		event.OrderID,
		event.Reason,
	)

	return nil
}
