package events

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/redis/go-redis/v9"
)


const (
	StreamOrderCreated      = "orders.created"
	StreamStockReserved     = "inventory.stock_reserved"
	StreamStockUnavailable  = "inventory.stock_unavailable"
	StreamPaymentCaptured   = "payments.captured"
	StreamPaymentFailed     = "payments.failed"
	StreamOrderConfirmed    = "orders.confirmed"
	StreamOrderCancelled    = "orders.cancelled"
)

// --- Event payloads ---

type OrderCreated struct {
	OrderID  string `json:"order_id"`
	ItemID   string `json:"item_id"`
	Quantity int    `json:"quantity"`
	Amount   int64  `json:"amount"` 
	Currency string `json:"currency"`
}

type StockReserved struct {
	OrderID  string `json:"order_id"`
	ItemID   string `json:"item_id"`
	Quantity int    `json:"quantity"`
	Amount   int64  `json:"amount"`
	Currency string `json:"currency"`
}

type StockUnavailable struct {
	OrderID string `json:"order_id"`
	ItemID  string `json:"item_id"`
	Reason  string `json:"reason"`
}

type PaymentCaptured struct {
	OrderID string `json:"order_id"`
	Amount  int64  `json:"amount"`
}

type PaymentFailed struct {
	OrderID string `json:"order_id"`
	Reason  string `json:"reason"`
}

type OrderConfirmed struct {
	OrderID string `json:"order_id"`
}

type OrderCancelled struct {
	OrderID string `json:"order_id"`
	Reason  string `json:"reason"`
}


type Publisher struct {
	Client *redis.Client
}

func NewPublisher(
	client *redis.Client,
) *Publisher {
	return &Publisher{
		Client: client,
	}
}

func (p *Publisher) Publish(
	ctx context.Context,
	stream string,
	event any,
) (string, error) {
	payload, err := json.Marshal(event)
	if err != nil {
		return "", fmt.Errorf("marshal event for %s: %w", stream, err)
	}

	id, err := p.Client.XAdd(
		ctx,
		&redis.XAddArgs{
			Stream: stream,
			Values: map[string]interface{}{
				"payload": string(payload),
			},
		},
	).Result()
	if err != nil {
		return "", fmt.Errorf("publish to %s: %w", stream, err)
	}

	return id, nil
}


type Message struct {
	ID      string
	Payload []byte
}


type Consumer struct {
	Client       *redis.Client
	Stream       string
	Group        string
	ConsumerName string
}

func NewConsumer(
	client *redis.Client,
	stream string,
	group string,
	consumerName string,
) *Consumer {
	return &Consumer{
		Client:       client,
		Stream:       stream,
		Group:        group,
		ConsumerName: consumerName,
	}
}

func (c *Consumer) EnsureGroup(
	ctx context.Context,
) error {
	err := c.Client.XGroupCreateMkStream(
		ctx,
		c.Stream,
		c.Group,
		"0",
	).Err()

	if err != nil && !strings.Contains(err.Error(), "BUSYGROUP") {
		return fmt.Errorf("ensure group for %s: %w", c.Stream, err)
	}

	return nil
}

func (c *Consumer) read(
	ctx context.Context,
) ([]Message, error) {
	streams, err := c.Client.XReadGroup(
		ctx,
		&redis.XReadGroupArgs{
			Group:    c.Group,
			Consumer: c.ConsumerName,
			Streams:  []string{c.Stream, ">"},
			Count:    10,
			Block:    5 * time.Second,
		},
	).Result()

	if err != nil {
		if errors.Is(err, redis.Nil) {
			return nil, nil
		}

		return nil, err
	}

	messages := make([]Message, 0)

	for _, stream := range streams {
		for _, entry := range stream.Messages {
			payload, ok := entry.Values["payload"].(string)
			if !ok {
				continue
			}

			messages = append(messages, Message{
				ID:      entry.ID,
				Payload: []byte(payload),
			})
		}
	}

	return messages, nil
}

func (c *Consumer) ack(
	ctx context.Context,
	id string,
) error {
	return c.Client.XAck(ctx, c.Stream, c.Group, id).Err()
}

func (c *Consumer) Run(
	ctx context.Context,
	handler func(context.Context, Message) error,
) {
	for {
		select {
		case <-ctx.Done():
			return
		default:
		}

		messages, err := c.read(ctx)
		if err != nil {
			if errors.Is(err, context.Canceled) {
				return
			}

			time.Sleep(time.Second)
			continue
		}

		for _, message := range messages {
			if err := handler(ctx, message); err != nil {
				continue
			}

			_ = c.ack(ctx, message.ID)
		}
	}
}
