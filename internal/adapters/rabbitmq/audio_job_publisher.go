package rabbitmq

import (
	"context"
	"encoding/json"
	"sync"

	amqp "github.com/rabbitmq/amqp091-go"
)

type AudioJobPublisher struct {
	// channel est un canal AMQP — pas un chan Go. C'est une conversation
	// virtuelle multiplexée à l'intérieur d'une seule connexion TCP vers
	// RabbitMQ (amqp.Dial → Connection, puis conn.Channel() → Channel).
	channel *amqp.Channel
	// mu protège channel : *amqp.Channel n'est pas thread-safe, et une fois
	// branché au serveur HTTP, plusieurs goroutines (une par requête, gérées
	// par Echo) peuvent appeler PublishTTSJob en même temps.
	mu sync.Mutex
}

// TTSJobQueue est à la fois le nom de la queue ET la routing key utilisée
// plus bas (voir PublishTTSJob) — les deux coïncident quand on publie via
// l'exchange par défaut de RabbitMQ.
const TTSJobQueue = "tts_jobs"

// ttsJobMessage est le contenu du message — juste les métadonnées nécessaires
// pour générer l'audio, jamais le fichier audio lui-même (trop volumineux
// pour une queue de messages ; le fichier réel ira sur S3, voir le worker).
type ttsJobMessage struct {
	AudioFileID string `json:"audio_file_id"`
	ScriptID    string `json:"script_id"`
	Text        string `json:"text"`
	Language    string `json:"language"`
	VoiceID     string `json:"voice_id"`
	// Attempt compte les tentatives déjà faites -- absent (donc zéro) sur
	// tout message publié ici, via l'API HTTP normale (POST /scripts/:id/
	// review, POST /audio-files/:id/retry). Seul le worker l'incrémente,
	// en republiant lui-même sur échec transitoire (voir worker.go,
	// requeueWithAttempt) plutôt que via Nack(requeue=true) -- un Nack
	// remet le MÊME message tel quel, sans moyen de faire grimper un
	// compteur dessus.
	Attempt int `json:"attempt"`
}

func NewAudioJobPublisher(channel *amqp.Channel) (*AudioJobPublisher, error) {
	// QueueDeclare(nom, durable, autoDelete, exclusive, noWait, args).
	// durable=true : la queue elle-même survit à un redémarrage du broker
	// (pas les messages dedans — ça, c'est DeliveryMode: amqp.Persistent,
	// plus bas). Idempotent : déclarer une queue qui existe déjà avec les
	// mêmes paramètres ne fait rien, pas d'erreur.
	if _, err := channel.QueueDeclare(TTSJobQueue, true, false, false, false, nil); err != nil {
		return nil, err
	}
	return &AudioJobPublisher{channel: channel}, nil
}

func (p *AudioJobPublisher) PublishTTSJob(ctx context.Context, audioFileID, scriptID, text, language, voiceID string) error {
	// Construire le message ne touche pas la ressource partagée (channel) —
	// donc fait avant de verrouiller, pour garder la section critique courte.
	body, err := json.Marshal(ttsJobMessage{
		AudioFileID: audioFileID,
		ScriptID:    scriptID,
		Text:        text,
		Language:    language,
		VoiceID:     voiceID,
	})
	if err != nil {
		return err
	}

	p.mu.Lock()
	defer p.mu.Unlock()
	// PublishWithContext(ctx, exchange, routingKey, mandatory, immediate, msg).
	// exchange="" : l'exchange PAR DÉFAUT de RabbitMQ, toujours présent,
	// jamais déclaré explicitement. Son comportement spécial : il relie
	// automatiquement chaque queue à elle-même, avec le nom de la queue comme
	// routing key — donc publier avec routingKey=TTSJobQueue l'envoie
	// directement dans la queue tts_jobs, sans exchange ni binding à créer
	// nous-mêmes. mandatory=false : pas d'erreur si personne n'écoute (on
	// veut juste que le message attende dans la queue).
	// DeliveryMode: amqp.Persistent : le message est écrit sur disque par
	// RabbitMQ, pas juste gardé en mémoire — survit à un redémarrage du
	// broker, contrairement à un message non-persistant.
	return p.channel.PublishWithContext(ctx, "", TTSJobQueue, false, false, amqp.Publishing{
		ContentType:  "application/json",
		DeliveryMode: amqp.Persistent,
		Body:         body,
	})
}
