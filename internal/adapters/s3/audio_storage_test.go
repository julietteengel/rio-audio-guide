package s3

import (
	"context"
	"os"
	"testing"

	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/s3"
)

func testClient(t *testing.T) *s3.Client {
	t.Helper()
	cfg, err := config.LoadDefaultConfig(context.Background())
	if err != nil {
		t.Fatalf("load aws config: %v", err)
	}
	return s3.NewFromConfig(cfg)
}

func testBucket(t *testing.T) string {
	t.Helper()
	bucket := os.Getenv("S3_TEST_BUCKET")
	if bucket == "" {
		t.Skip("S3_TEST_BUCKET not set — skipping real-S3 integration test")
	}
	return bucket
}

func TestAudioStorage_Upload(t *testing.T) {
	client := testClient(t)
	bucket := testBucket(t)

	storage := NewAudioStorage(client, bucket)
	url, err := storage.Upload(context.Background(), "test/audio.mp3", []byte("fake audio bytes"), "audio/mpeg")
	if err != nil {
		t.Fatalf("upload: %v", err)
	}
	want := "s3://" + bucket + "/test/audio.mp3"
	if url != want {
		t.Fatalf("got %q, want %q", url, want)
	}
}
