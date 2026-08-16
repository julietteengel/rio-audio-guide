package main

import (
	"context"
	"log"
	"os"
	"os/signal"
	"syscall"

	"github.com/aws/aws-sdk-go-v2/config"
	awss3 "github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/jackc/pgx/v5/pgxpool"
	amqp "github.com/rabbitmq/amqp091-go"

	"rioaudioguide/backend/internal/adapters/elevenlabs"
	"rioaudioguide/backend/internal/adapters/postgres"
	"rioaudioguide/backend/internal/adapters/rabbitmq"
	"rioaudioguide/backend/internal/adapters/s3"
)

func main() {
	// signal.NotifyContext dérive un context.Context qui s'annule tout seul dès que le
	// process reçoit SIGINT (Ctrl+C, os.Interrupt) ou SIGTERM (envoyé par Kubernetes
	// avant de tuer un pod). worker.Run(ctx) plus bas surveille ctx.Done() dans sa boucle
	// de consommation — c'est ce qui permet un arrêt propre ("graceful shutdown") : finir
	// le message en cours plutôt que de couper une génération audio en plein milieu.
	// stop() (deferred) désabonne les signaux une fois main() terminé, sinon ils
	// resteraient interceptés même après l'arrêt du worker.
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	// pgxpool.Pool : pool de connexions Postgres réutilisables, pas une connexion unique.
	// Le worker ne sert qu'une requête à la fois (pas de concurrence HTTP comme l'API),
	// mais on garde pgxpool quand même — mêmes repositories Postgres que l'API, pas
	// d'implémentation à dupliquer, et ça encaisse une éventuelle reconnexion automatique
	// en cas de coupure réseau vers Postgres.
	pool, err := pgxpool.New(ctx, envOr("DATABASE_URL", "postgres://postgres:postgres@localhost:5432/postgres"))
	if err != nil {
		log.Fatalf("connect postgres: %v", err)
	}
	defer pool.Close()

	// amqp.Dial : la connexion TCP unique vers RabbitMQ.
	conn, err := amqp.Dial(envOr("RABBITMQ_URL", "amqp://guest:guest@localhost:5672/"))
	if err != nil {
		log.Fatalf("connect rabbitmq: %v", err)
	}
	defer func() { _ = conn.Close() }()

	// amqp.Channel : canal logique sur cette connexion, utilisé pour consommer la queue
	// tts_jobs (rabbitmq.NewWorker en a besoin plus bas) — jamais conn directement, comme
	// côté API. Concept du protocole AMQP, distinct d'un `chan` Go.
	channel, err := conn.Channel()
	if err != nil {
		log.Fatalf("open rabbitmq channel: %v", err)
	}
	defer func() { _ = channel.Close() }()

	// Vrai AWS : LoadDefaultConfig lit AWS_ACCESS_KEY_ID/AWS_SECRET_ACCESS_KEY/
	// AWS_SESSION_TOKEN/AWS_REGION depuis l'environnement (ou ~/.aws/credentials)
	// automatiquement — rien à coder en dur, pas d'endpoint local à forcer.
	awsCfg, err := config.LoadDefaultConfig(ctx)
	if err != nil {
		log.Fatalf("load aws config: %v", err)
	}
	s3Client := awss3.NewFromConfig(awsCfg)
	storage := s3.NewAudioStorage(s3Client, envOr("S3_BUCKET", "rio-audioguide-bucket"))

	scriptRepo := postgres.NewScriptRepository(pool)
	audioFileRepo := postgres.NewAudioFileRepository(pool)
	ttsGenerator := elevenlabs.NewGenerator(mustEnv("ELEVENLABS_API_KEY"))

	worker, err := rabbitmq.NewWorker(channel, scriptRepo, audioFileRepo, storage, ttsGenerator)
	if err != nil {
		log.Fatalf("set up worker: %v", err)
	}

	log.Println("worker ready, consuming tts_jobs")
	if err := worker.Run(ctx); err != nil && ctx.Err() == nil {
		log.Fatalf("worker stopped unexpectedly: %v", err)
	}
	log.Println("worker shut down cleanly")
}

func envOr(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

func mustEnv(key string) string {
	v := os.Getenv(key)
	if v == "" {
		log.Fatalf("%s is required", key)
	}
	return v
}
