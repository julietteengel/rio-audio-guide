package rabbitmq

import (
	"context"
	"encoding/json"
	"errors"
	"log"
	"time"

	amqp "github.com/rabbitmq/amqp091-go"

	"rioaudioguide/backend/internal/application"
	"rioaudioguide/backend/internal/ports"
)

type Worker struct {
	channel       *amqp.Channel
	scriptRepo    ports.ScriptRepository
	audioFileRepo ports.AudioFileRepository
	storage       ports.AudioStorage
	ttsGenerator  ports.TTSGenerator
}

func NewWorker(channel *amqp.Channel, scriptRepo ports.ScriptRepository, audioFileRepo ports.AudioFileRepository, storage ports.AudioStorage, ttsGenerator ports.TTSGenerator) (*Worker, error) {
	// Même déclaration que côté publisher — idempotente, et nécessaire ici
	// aussi : rien ne garantit que le publisher aura tourné avant le worker
	// au premier démarrage (deux binaires séparés, cmd/api et cmd/worker).
	if _, err := channel.QueueDeclare(TTSJobQueue, true, false, false, false, nil); err != nil {
		return nil, err
	}
	return &Worker{channel: channel, scriptRepo: scriptRepo, audioFileRepo: audioFileRepo, storage: storage, ttsGenerator: ttsGenerator}, nil
}

// Run consomme tts_jobs jusqu'à annulation du ctx. Bloquant — à lancer dans
// sa propre goroutine ou son propre binaire (cmd/worker).
func (w *Worker) Run(ctx context.Context) error {
	// Consume(queue, consumerTag, autoAck, exclusive, noLocal, noWait, args).
	// autoAck=false est LE paramètre important : RabbitMQ attend un Ack/Nack
	// explicite avant de considérer le message traité. Si le worker crashe
	// avant d'accuser réception, RabbitMQ redistribue le message à un autre
	// consumer dès qu'il détecte la déconnexion — rien n'est perdu en
	// silence. msgs est un vrai chan Go (pas un amqp.Channel) : RabbitMQ y
	// pousse chaque delivery.
	msgs, err := w.channel.Consume(TTSJobQueue, "", false, false, false, false, nil)
	if err != nil {
		return err
	}

	for {
		select {
		case <-ctx.Done():
			// Arrêt demandé (ex. signal SIGTERM côté cmd/worker) — sortie propre.
			return ctx.Err()
		case msg, ok := <-msgs:
			if !ok {
				// RabbitMQ a fermé le canal de livraison (déconnexion) —
				// on arrête plutôt que de boucler sur des messages vides.
				return nil
			}
			w.handle(ctx, msg)
		}
	}
}

// requeueDelay temporise chaque remise en queue. Sans elle, un 429 persistant
// devient une boucle serrée qui martèle ElevenLabs aussi vite que le réseau le
// permet — le meilleur moyen de se faire throttler au niveau du compte. Le
// worker traite les messages en série (un seul handle() à la fois dans Run),
// donc dormir ici ne bloque rien d'autre.
const requeueDelay = 2 * time.Second

// maxTTSAttempts borne les retries sur une erreur TTS transitoire (timeout,
// 5xx, hoquet réseau) -- sans ce plafond, Nack(requeue=true) reconduit le
// même message indéfiniment : chaque redelivery rappelle ElevenLabs (payant)
// pour, potentiellement, échouer encore. Au-delà, on abandonne proprement
// (AudioFile "failed", visible et rejouable via POST /audio-files/:id/retry)
// plutôt que de boucler en silence. Même valeur que uploadWithRetryAttempts,
// pas de raison technique de diverger, juste une même intuition "3 essais".
const maxTTSAttempts = 3

func (w *Worker) handle(ctx context.Context, msg amqp.Delivery) {
	var job ttsJobMessage
	if err := json.Unmarshal(msg.Body, &job); err != nil {
		log.Printf("tts worker: bad message, dropping: %v", err)
		// Nack(multiple, requeue). multiple=false : n'accuser/rejeter QUE ce
		// message, pas tous ceux reçus avant lui. requeue=false ici : un
		// message malformé le restera pour toujours — le remettre en queue
		// créerait une boucle infinie de re-livraison du même message cassé.
		_ = msg.Nack(false, false)
		return
	}

	// Marque l'AudioFile "generating" AVANT le travail lent (l'appel
	// ElevenLabs) — pendant que ça tourne, Postgres reflète l'état réel
	// plutôt qu'un mensonge ("queued" alors que ça travaille déjà).
	if err := application.StartAudioGeneration(ctx, w.audioFileRepo, job.AudioFileID); err != nil {
		log.Printf("tts worker: start generation failed for %s: %v", job.AudioFileID, err)
		// requeue=true ici : contrairement au message malformé, une erreur
		// de repository est probablement transitoire (base momentanément
		// indisponible) — retenter a du sens.
		time.Sleep(requeueDelay)
		_ = msg.Nack(false, true)
		return
	}

	audioBytes, duration, err := w.ttsGenerator.Generate(ctx, job.Text, job.Language, job.VoiceID)
	if err != nil {
		var permErr *ports.PermanentError
		if errors.As(err, &permErr) {
			log.Printf("tts worker: permanent TTS error for %s, marking failed: %v", job.AudioFileID, err)
			if failErr := application.FailAudioGeneration(ctx, w.audioFileRepo, job.AudioFileID, err.Error()); failErr != nil {
				log.Printf("tts worker: mark failed also failed for %s: %v", job.AudioFileID, failErr)
			}
			// Ack, pas Nack : réessayer le même message ne changera rien à une
			// clé invalide ou un texte/voice_id rejeté.
			_ = msg.Ack(false)
			return
		}
		log.Printf("tts worker: transient TTS error for %s (attempt %d/%d): %v",
			job.AudioFileID, job.Attempt+1, maxTTSAttempts, err)

		if job.Attempt+1 >= maxTTSAttempts {
			log.Printf("tts worker: giving up on %s after %d attempts, marking failed", job.AudioFileID, maxTTSAttempts)
			if failErr := application.FailAudioGeneration(ctx, w.audioFileRepo, job.AudioFileID, err.Error()); failErr != nil {
				log.Printf("tts worker: mark failed also failed for %s: %v", job.AudioFileID, failErr)
			}
			_ = msg.Ack(false)
			return
		}

		time.Sleep(requeueDelay)
		job.Attempt++
		if pubErr := w.requeueWithAttempt(ctx, job); pubErr != nil {
			// Le nouveau message (compteur incrémenté) n'est pas parti -- on
			// retombe sur Nack(requeue=true) plutôt que de perdre le job,
			// au prix de reperdre le compteur pour ce tour-ci (dégradé, pas
			// silencieux : c'est loggé).
			log.Printf("tts worker: failed to republish retry for %s, falling back to plain requeue: %v", job.AudioFileID, pubErr)
			_ = msg.Nack(false, true)
			return
		}
		_ = msg.Ack(false)
		return
	}

	// Le fichier audio part sur S3 via le port AudioStorage, jamais dans
	// RabbitMQ — la queue ne transporte que de petits messages de contrôle
	// (voir ttsJobMessage côté publisher).
	//
	// uploadWithRetry réessaie l'upload lui-même, en gardant audioBytes déjà
	// en mémoire -- sans ça, un Nack(requeue=true) ici referait tout le
	// message depuis le début, y compris le rappel ElevenLabs payant, pour
	// ne retenter au fond qu'un upload S3 gratuit. On ne tombe sur le Nack
	// (donc une régénération complète) qu'après avoir épuisé ces tentatives
	// locales, pas dès le premier hoquet réseau transitoire.
	storageURL, err := uploadWithRetry(ctx, w.storage, job.AudioFileID+".mp3", audioBytes)
	if err != nil {
		var permErr *ports.PermanentError
		if errors.As(err, &permErr) {
			log.Printf("tts worker: permanent S3 error for %s, marking failed: %v", job.AudioFileID, err)
			if failErr := application.FailAudioGeneration(ctx, w.audioFileRepo, job.AudioFileID, err.Error()); failErr != nil {
				log.Printf("tts worker: mark failed also failed for %s: %v", job.AudioFileID, failErr)
			}
			_ = msg.Ack(false)
			return
		}
		log.Printf("tts worker: upload failed after local retries for %s: %v", job.AudioFileID, err)
		time.Sleep(requeueDelay)
		_ = msg.Nack(false, true)
		return
	}

	// Marque l'AudioFile "ready" ET publie le Script associé (l'événement
	// de domaine "audio prêt → script publié" qu'on avait nommé plus tôt).
	if err := application.CompleteAudioGeneration(ctx, w.scriptRepo, w.audioFileRepo, job.AudioFileID, storageURL, "", duration); err != nil {
		log.Printf("tts worker: complete generation failed for %s: %v", job.AudioFileID, err)
		time.Sleep(requeueDelay)
		_ = msg.Nack(false, true)
		return
	}

	// Tout a réussi : on accuse réception, RabbitMQ peut supprimer le
	// message de la queue pour de bon.
	_ = msg.Ack(false)
}

// requeueWithAttempt republie une copie du job avec Attempt incrémenté --
// même forme de publication que AudioJobPublisher.PublishTTSJob (exchange
// par défaut, routing key = nom de la queue, message persistant), mais
// depuis le worker lui-même : Nack(requeue=true) redonne le MÊME message
// tel quel, sans permettre de modifier son contenu, donc pas de moyen d'y
// faire grimper un compteur autrement qu'en publiant un nouveau message.
func (w *Worker) requeueWithAttempt(ctx context.Context, job ttsJobMessage) error {
	body, err := json.Marshal(job)
	if err != nil {
		return err
	}
	return w.channel.PublishWithContext(ctx, "", TTSJobQueue, false, false, amqp.Publishing{
		ContentType:  "application/json",
		DeliveryMode: amqp.Persistent,
		Body:         body,
	})
}

// uploadWithRetryAttempts borne les tentatives locales -- un nombre fixe,
// pas de recours à un contexte avec deadline propre : on veut juste éviter
// qu'un hoquet réseau transitoire déclenche un Nack(requeue=true), qui lui
// referait tout le message depuis le début (donc un nouvel appel ElevenLabs
// payant) pour ne retenter, au fond, qu'un upload S3 gratuit qui n'a rien à
// voir avec la génération audio elle-même.
const uploadWithRetryAttempts = 3

// uploadWithRetry réessaie l'upload S3 seul, en gardant audioBytes déjà en
// mémoire -- jamais de rappel à ElevenLabs pour ces tentatives locales. On
// ne laisse remonter l'erreur vers handle() (donc vers le Nack qui referait
// tout) qu'après avoir épuisé ces tentatives, ou immédiatement si l'erreur
// est permanente (identifiants invalides, bucket absent) : réessayer une
// erreur permanente ne changerait rien, ni ici ni côté RabbitMQ.
func uploadWithRetry(ctx context.Context, storage ports.AudioStorage, key string, audioBytes []byte) (string, error) {
	var lastErr error
	for attempt := 1; attempt <= uploadWithRetryAttempts; attempt++ {
		storageURL, err := storage.Upload(ctx, key, audioBytes, "audio/mpeg")
		if err == nil {
			return storageURL, nil
		}

		var permErr *ports.PermanentError
		if errors.As(err, &permErr) {
			return "", err
		}

		lastErr = err
		if attempt < uploadWithRetryAttempts {
			log.Printf("tts worker: upload attempt %d/%d failed for %s, retrying: %v",
				attempt, uploadWithRetryAttempts, key, err)
			time.Sleep(time.Duration(attempt) * 500 * time.Millisecond) // 500ms, puis 1s
		}
	}
	return "", lastErr
}
