//go:build integration

package rabbitmq

import (
	"context"
	"testing"
	"time"

	amqp "github.com/rabbitmq/amqp091-go"
)

func testChannel(t *testing.T) *amqp.Channel {
	t.Helper()
	conn, err := amqp.Dial("amqp://guest:guest@localhost:5672/")
	if err != nil {
		t.Fatalf("dial rabbitmq: %v", err)
	}
	t.Cleanup(func() { _ = conn.Close() })

	channel, err := conn.Channel()
	if err != nil {
		t.Fatalf("open channel: %v", err)
	}
	t.Cleanup(func() { _ = channel.Close() })
	return channel
}

func TestAudioJobPublisher_PublishTTSJob(t *testing.T) {
	channel := testChannel(t)

	publisher, err := NewAudioJobPublisher(channel)
	if err != nil {
		t.Fatalf("new publisher: %v", err)
	}

	if err := publisher.PublishTTSJob(context.Background(), "audio-1", "script-1", "Texte", "fr", "voice-1"); err != nil {
		t.Fatalf("publish: %v", err)
	}

	msgs, err := channel.Consume(TTSJobQueue, "", true, false, false, false, nil)
	if err != nil {
		t.Fatalf("consume: %v", err)
	}
	select {
	case msg := <-msgs:
		if len(msg.Body) == 0 {
			t.Fatal("got empty message body")
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for published message")
	}
}
