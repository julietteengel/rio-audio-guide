package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"os/signal"
	"syscall"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/polly"
	awss3 "github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/jackc/pgx/v5/pgxpool"
	amqp "github.com/rabbitmq/amqp091-go"

	"rioaudioguide/backend/internal/adapters/awspolly"
	"rioaudioguide/backend/internal/adapters/openaitts"
	"rioaudioguide/backend/internal/adapters/postgres"
	"rioaudioguide/backend/internal/adapters/rabbitmq"
	s3adapter "rioaudioguide/backend/internal/adapters/s3"
	"rioaudioguide/backend/internal/ports"
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
	bucket := envOr("S3_BUCKET", "rio-audio-guide")

	scriptRepo := postgres.NewScriptRepository(pool)
	audioFileRepo := postgres.NewAudioFileRepository(pool)
	ttsGenerator, err := newTTSGenerator(awsCfg, s3Client, bucket)
	if err != nil {
		log.Fatalf("set up tts provider: %v", err)
	}

	worker, err := rabbitmq.NewWorker(channel, scriptRepo, audioFileRepo, ttsGenerator)
	if err != nil {
		log.Fatalf("set up worker: %v", err)
	}

	log.Println("worker ready, consuming tts_jobs")
	if err := worker.Run(ctx); err != nil && ctx.Err() == nil {
		log.Fatalf("worker stopped unexpectedly: %v", err)
	}
	log.Println("worker shut down cleanly")
}

// newTTSGenerator picks the speech provider. TTS_PROVIDER=openai|polly wins;
// unset, it is OpenAI when OPENAI_API_KEY is present (the provider that produced
// the audio in the database) and Polly otherwise, so an existing Polly-only
// deployment keeps working untouched.
func newTTSGenerator(awsCfg aws.Config, s3Client *awss3.Client, bucket string) (ports.TTSGenerator, error) {
	openaiKey := os.Getenv("OPENAI_API_KEY")
	provider := os.Getenv("TTS_PROVIDER")
	if provider == "" {
		provider = "polly"
		if openaiKey != "" {
			provider = "openai"
		}
	}
	switch provider {
	case "openai":
		if openaiKey == "" {
			return nil, fmt.Errorf("TTS_PROVIDER=openai requires OPENAI_API_KEY")
		}
		log.Println("tts provider: openai (tts-1 + whisper-1)")
		return openaitts.NewGenerator(openaiKey, s3adapter.NewAudioStorage(s3Client, bucket)), nil
	case "polly":
		log.Println("tts provider: aws polly")
		return awspolly.NewGenerator(polly.NewFromConfig(awsCfg), s3Client, bucket), nil
	default:
		return nil, fmt.Errorf("unknown TTS_PROVIDER %q (want openai or polly)", provider)
	}
}

func envOr(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
