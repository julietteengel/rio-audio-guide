package rabbitmq

import (
	"context"
	"encoding/json"
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
}

func NewWorker(channel *amqp.Channel, scriptRepo ports.ScriptRepository, audioFileRepo ports.AudioFileRepository, storage ports.AudioStorage) (*Worker, error) {
	// Même déclaration que côté publisher — idempotente, et nécessaire ici
	// aussi : rien ne garantit que le publisher aura tourné avant le worker
	// au premier démarrage (deux binaires séparés, cmd/api et cmd/worker).
	if _, err := channel.QueueDeclare(TTSJobQueue, true, false, false, false, nil); err != nil {
		return nil, err
	}
	return &Worker{channel: channel, scriptRepo: scriptRepo, audioFileRepo: audioFileRepo, storage: storage}, nil
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

	// Marque l'AudioFile "generating" AVANT le travail lent (le stub, puis
	// un jour ElevenLabs) — pendant que ça tourne, Postgres reflète l'état
	// réel plutôt qu'un mensonge ("queued" alors que ça travaille déjà).
	if err := application.StartAudioGeneration(ctx, w.audioFileRepo, job.AudioFileID); err != nil {
		log.Printf("tts worker: start generation failed for %s: %v", job.AudioFileID, err)
		// requeue=true ici : contrairement au message malformé, une erreur
		// de repository est probablement transitoire (base momentanément
		// indisponible) — retenter a du sens.
		_ = msg.Nack(false, true)
		return
	}

	// Stub explicite : pas de vrai appel ElevenLabs ici, juste des octets et
	// une durée plausibles, pour exercer réellement le reste du pipeline
	// (upload, Postgres, transition Script→published) sans dépendre d'une
	// clé API. À remplacer plus tard par un vrai appel TTS.
	audioBytes, duration := generateAudioStub(job.Text)

	// Le fichier "audio" (factice pour l'instant) part sur S3 via le port
	// AudioStorage, jamais dans RabbitMQ — la queue ne transporte que de
	// petits messages de contrôle (voir ttsJobMessage côté publisher).
	storageURL, err := w.storage.Upload(ctx, job.AudioFileID+".mp3", audioBytes, "audio/mpeg")
	if err != nil {
		log.Printf("tts worker: upload failed for %s: %v", job.AudioFileID, err)
		_ = msg.Nack(false, true)
		return
	}

	// Marque l'AudioFile "ready" ET publie le Script associé (l'événement
	// de domaine "audio prêt → script publié" qu'on avait nommé plus tôt).
	if err := application.CompleteAudioGeneration(ctx, w.scriptRepo, w.audioFileRepo, job.AudioFileID, storageURL, "", duration); err != nil {
		log.Printf("tts worker: complete generation failed for %s: %v", job.AudioFileID, err)
		_ = msg.Nack(false, true)
		return
	}

	// Tout a réussi : on accuse réception, RabbitMQ peut supprimer le
	// message de la queue pour de bon.
	_ = msg.Ack(false)
}

// generateAudioStub remplace le vrai appel TTS (ElevenLabs) — pas construit ici,
// nécessiterait une clé API et sa propre conception. Renvoie un résultat
// plausible mais factice pour exercer réellement le reste du pipeline (upload,
// Postgres, transition Script→published).
func generateAudioStub(text string) (audioBytes []byte, duration time.Duration) {
	wordCount := len(text) / 5
	duration = time.Duration(wordCount) * 400 * time.Millisecond
	return []byte("STUB-AUDIO:" + text), duration
}
