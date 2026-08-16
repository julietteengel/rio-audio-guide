package main

import (
	"context"
	"log"
	"os"

	"github.com/jackc/pgx/v5/pgxpool"
	amqp "github.com/rabbitmq/amqp091-go"

	httpadapter "rioaudioguide/backend/internal/adapters/http"
	"rioaudioguide/backend/internal/adapters/postgres"
	"rioaudioguide/backend/internal/adapters/rabbitmq"
)

func main() {
	ctx := context.Background()

	pool, err := pgxpool.New(ctx, envOr("DATABASE_URL", "postgres://postgres:postgres@localhost:5432/postgres"))
	if err != nil {
		log.Fatalf("connect postgres: %v", err)
	}
	defer pool.Close()

	conn, err := amqp.Dial(envOr("RABBITMQ_URL", "amqp://guest:guest@localhost:5672/"))
	if err != nil {
		log.Fatalf("connect rabbitmq: %v", err)
	}
	defer func() { _ = conn.Close() }()

	channel, err := conn.Channel()
	if err != nil {
		log.Fatalf("open rabbitmq channel: %v", err)
	}
	defer func() { _ = channel.Close() }()

	placeRepo := postgres.NewPlaceRepository(pool)
	scriptRepo := postgres.NewScriptRepository(pool)
	audioFileRepo := postgres.NewAudioFileRepository(pool)

	publisher, err := rabbitmq.NewAudioJobPublisher(channel)
	if err != nil {
		log.Fatalf("set up audio job publisher: %v", err)
	}

	server := httpadapter.NewServer(placeRepo, scriptRepo, audioFileRepo, publisher)
	log.Println("api ready, listening on :8080")
	if err := server.Start(":8080"); err != nil {
		log.Fatalf("http server: %v", err)
	}
}

func envOr(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
