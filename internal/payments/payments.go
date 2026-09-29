package payments

import (
	"context"
	"encoding/json"
	"fmt"
	"math/rand"
	"net/http"

	"github.com/go-chi/chi/v5"

	"github.com/Mitesh0007/orderflow-go/pkg/events"
)

const highValueThreshold = 100000 

type Handler struct {
	Publisher   *events.Publisher
	FailureRate float64 
}

func NewHandler(
	publisher *events.Publisher,
	failureRate float64,
) *Handler {
	return &Handler{
		Publisher:   publisher,
		FailureRate: failureRate,
	}
}

func (h *Handler) Routes() http.Handler {
	r := chi.NewRouter()

	r.Get("/health", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	return r
}

func (h *Handler) OnStockReserved(
	ctx context.Context,
	msg events.Message,
) error {
	var event events.StockReserved
	if err := json.Unmarshal(msg.Payload, &event); err != nil {
		return fmt.Errorf("unmarshal stock_reserved: %w", err)
	}

	if event.Amount >= highValueThreshold {
		_, err := h.Publisher.Publish(ctx, events.StreamPaymentFailed, events.PaymentFailed{
			OrderID: event.OrderID,
			Reason:  "amount exceeds allowed threshold",
		})

		return err
	}

	if h.FailureRate > 0 && rand.Float64() < h.FailureRate {
		_, err := h.Publisher.Publish(ctx, events.StreamPaymentFailed, events.PaymentFailed{
			OrderID: event.OrderID,
			Reason:  "simulated random payment failure",
		})

		return err
	}

	_, err := h.Publisher.Publish(ctx, events.StreamPaymentCaptured, events.PaymentCaptured{
		OrderID: event.OrderID,
		Amount:  event.Amount,
	})

	return err
}
