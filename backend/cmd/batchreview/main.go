// cmd/batchreview works through the backlog of Draft scripts one language
// at a time, with one fixed voice_id per language -- the gap between "a
// place has narration text in the pipeline" and "it's published with audio"
// was never a missing feature (POST /scripts/:id/review already does the
// real work: mark reviewed, request audio, auto-publish on completion). It
// was simply nobody clicking through hundreds of real, individually-billed
// ElevenLabs/Polly calls by hand. This tool automates the clicking, not the
// human judgment -- -dry-run (the default) only reports what WOULD run,
// since every real call costs money.
package main

import (
	"context"
	"flag"
	"log"
	"os"
	"os/signal"
	"syscall"

	"github.com/jackc/pgx/v5/pgxpool"
	amqp "github.com/rabbitmq/amqp091-go"

	"rioaudioguide/backend/internal/adapters/postgres"
	"rioaudioguide/backend/internal/adapters/rabbitmq"
	"rioaudioguide/backend/internal/application"
	"rioaudioguide/backend/internal/domain"
)

func main() {
	voiceFR := flag.String("voice-fr", "", "ElevenLabs/Polly voice_id for French drafts")
	voiceEN := flag.String("voice-en", "", "voice_id for English drafts")
	voicePT := flag.String("voice-pt", "", "voice_id for Portuguese drafts")
	voiceES := flag.String("voice-es", "", "voice_id for Spanish drafts")
	reviewer := flag.String("reviewer", "", "user id recorded as this batch's reviewer (must exist in users)")
	dryRun := flag.Bool("dry-run", true, "list what would be reviewed without triggering any billed call (default true -- pass -dry-run=false to actually run)")
	flag.Parse()

	if *reviewer == "" {
		log.Fatal("-reviewer is required (a real users.id -- reviewer_id has a foreign key constraint)")
	}
	voiceByLanguage := map[domain.Language]string{
		domain.LanguageFR: *voiceFR,
		domain.LanguageEN: *voiceEN,
		domain.LanguagePT: *voicePT,
		domain.LanguageES: *voiceES,
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	pool, err := pgxpool.New(ctx, envOr("DATABASE_URL", "postgres://postgres:postgres@localhost:5432/postgres"))
	if err != nil {
		log.Fatalf("connect postgres: %v", err)
	}
	defer pool.Close()

	scriptRepo := postgres.NewScriptRepository(pool)
	audioFileRepo := postgres.NewAudioFileRepository(pool)

	drafts, err := scriptRepo.FindAllDraft(ctx)
	if err != nil {
		log.Fatalf("find draft scripts: %v", err)
	}
	log.Printf("found %d draft script(s)", len(drafts))

	// Group first, so a missing voice for a language fails loudly up front
	// (before any job is published) instead of partway through the batch.
	byLanguage := map[domain.Language][]*domain.Script{}
	for _, s := range drafts {
		byLanguage[s.Language()] = append(byLanguage[s.Language()], s)
	}
	for language, scripts := range byLanguage {
		if voiceByLanguage[language] == "" {
			log.Fatalf("%d draft script(s) in language %q, but no -voice-%s given", len(scripts), language, language)
		}
	}

	if *dryRun {
		for language, scripts := range byLanguage {
			log.Printf("[dry-run] would review %d script(s) in %q with voice %q -- no call made, pass -dry-run=false to actually run", len(scripts), language, voiceByLanguage[language])
		}
		return
	}

	// Only connect to RabbitMQ (and start spending money) once dry-run is
	// confirmed off -- a dry-run needs no broker at all.
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
	publisher, err := rabbitmq.NewAudioJobPublisher(channel)
	if err != nil {
		log.Fatalf("set up publisher: %v", err)
	}

	var succeeded, failed int
	for _, script := range drafts {
		voiceID := voiceByLanguage[script.Language()]
		err := application.ReviewAndRequestAudio(ctx, scriptRepo, audioFileRepo, publisher, script.ID(), *reviewer, voiceID)
		if err != nil {
			log.Printf("script %s (place %s, %s): FAILED: %v", script.ID(), script.PlaceID(), script.Language(), err)
			failed++
			continue
		}
		succeeded++
	}
	log.Printf("done: %d reviewed and queued for audio generation, %d failed", succeeded, failed)
}

func envOr(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
