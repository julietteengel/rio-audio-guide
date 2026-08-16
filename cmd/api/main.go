package main

import (
	"context"
	"log"
	"os"

	"github.com/aws/aws-sdk-go-v2/config"
	awss3 "github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/jackc/pgx/v5/pgxpool"
	amqp "github.com/rabbitmq/amqp091-go"
	goredis "github.com/redis/go-redis/v9"

	httpadapter "rioaudioguide/backend/internal/adapters/http"
	"rioaudioguide/backend/internal/adapters/postgres"
	"rioaudioguide/backend/internal/adapters/rabbitmq"
	"rioaudioguide/backend/internal/adapters/redis"
	"rioaudioguide/backend/internal/adapters/s3"
)

func main() {
	// context.Context porte une deadline/annulation à travers un appel — ici on n'a
	// ni timeout ni annulation à propager (le process tourne jusqu'à ce qu'on le tue),
	// donc context.Background() (contexte "racine", vide) suffit. pgxpool.New(ctx, ...)
	// l'utilise seulement pour pouvoir annuler la connexion initiale si besoin.
	ctx := context.Background()

	// pgxpool.Pool n'est PAS une connexion unique : c'est un pool de connexions
	// PostgreSQL réutilisables. Chaque requête HTTP gérée par le serveur Echo emprunte
	// une connexion du pool le temps de sa requête SQL, puis la rend au pool — sans pool,
	// il faudrait ouvrir/fermer une connexion TCP par requête HTTP, beaucoup plus lent.
	// pgxpool gère aussi le nombre max de connexions simultanées vers Postgres.
	pool, err := pgxpool.New(ctx, envOr("DATABASE_URL", "postgres://postgres:postgres@localhost:5432/postgres"))
	if err != nil {
		log.Fatalf("connect postgres: %v", err)
	}
	defer pool.Close() // ferme toutes les connexions du pool à l'arrêt du process

	// amqp.Dial ouvre la connexion TCP vers RabbitMQ (une seule, coûteuse à établir).
	conn, err := amqp.Dial(envOr("RABBITMQ_URL", "amqp://guest:guest@localhost:5672/"))
	if err != nil {
		log.Fatalf("connect rabbitmq: %v", err)
	}
	defer func() { _ = conn.Close() }()

	// amqp.Channel : un canal logique multiplexé sur la connexion TCP unique — c'est LUI
	// qu'on utilise pour publier/consommer des messages, jamais conn directement. Une
	// seule connexion peut porter plusieurs channels (un par usage), ce qui évite de
	// rouvrir une connexion TCP à chaque fois. Ce Channel AMQP n'a rien à voir avec un
	// `chan` Go — c'est un concept du protocole RabbitMQ, pas une primitive du langage.
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

	awsCfg, err := config.LoadDefaultConfig(ctx)
	if err != nil {
		log.Fatalf("load aws config: %v", err)
	}
	s3Client := awss3.NewFromConfig(awsCfg)
	storage := s3.NewAudioStorage(s3Client, envOr("S3_BUCKET", "rio-audioguide-bucket"))

	redisClient := goredis.NewClient(&goredis.Options{Addr: envOr("REDIS_URL", "localhost:6379")})
	cache := redis.NewCache(redisClient)

	server := httpadapter.NewServer(placeRepo, scriptRepo, audioFileRepo, publisher, storage, cache)
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
