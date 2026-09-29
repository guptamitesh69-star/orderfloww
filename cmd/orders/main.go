package main

import (
	"context"
	"errors"
	"log"
	"net"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/redis/go-redis/v9"

	"github.com/Mitesh0007/orderflow-go/internal/orders"
	"github.com/Mitesh0007/orderflow-go/pkg/events"
)

func getEnv(key, fallback string) string {
	value := os.Getenv(key)
	if value == "" {
		return fallback
	}
	return value
}

const consumerGroup = "orders-service"

func main() {
	ctx, stop := signal.NotifyContext(
		context.Background(),
		os.Interrupt,
		syscall.SIGTERM,
	)
	defer stop()

	redisAddr := getEnv("REDIS_ADDR", "redis://localhost:6379")
	redisOptions, err := redis.ParseURL(redisAddr)
	if err != nil {
		log.Fatalf("invalid Redis address: %v", err)
	}

	redisClient := redis.NewClient(redisOptions)
	defer redisClient.Close()

	if err := redisClient.Ping(ctx).Err(); err != nil {
		log.Fatalf("redis connection failed at %s: %v", redisAddr, err)
	}

	publisher := events.NewPublisher(redisClient)
	store := orders.NewStore()
	handler := orders.NewHandler(store, publisher)

	consumers := []struct {
		stream string
		handle func(context.Context, events.Message) error
	}{
		{events.StreamPaymentCaptured, handler.OnPaymentCaptured},
		{events.StreamPaymentFailed, handler.OnPaymentFailed},
		{events.StreamStockUnavailable, handler.OnStockUnavailable},
	}

	for _, c := range consumers {
		consumer := events.NewConsumer(redisClient, c.stream, consumerGroup, "orders-1")
		if err := consumer.EnsureGroup(ctx); err != nil {
			log.Fatal(err)
		}
		go consumer.Run(ctx, c.handle)
	}

	port := getEnv("PORT", "8100")
	server := &http.Server{
		Addr:    ":" + port,
		Handler: handler.Routes(),
		BaseContext: func(_ net.Listener) context.Context {
			return ctx
		},
	}

	go func() {
		log.Printf("orders service listening on :%s", port)
		if err := server.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			log.Printf("server error: %v", err)
			stop()
		}
	}()

	<-ctx.Done()
	log.Println("shutdown signal received")

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := server.Shutdown(shutdownCtx); err != nil {
		log.Printf("server shutdown failed: %v", err)
	}
	log.Println("orders service stopped")
}
