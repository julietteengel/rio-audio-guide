package s3

import (
	"context"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials"
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

func TestAudioStorage_PresignURL(t *testing.T) {
	cfg := aws.Config{
		Region:      "us-east-1",
		Credentials: credentials.NewStaticCredentialsProvider("test-key", "test-secret", ""),
	}
	client := s3.NewFromConfig(cfg)
	storage := NewAudioStorage(client, "rio-audio-guide")

	url, err := storage.PresignURL(context.Background(), "abc123.mp3", 15*time.Minute)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(url, "rio-audio-guide") || !strings.Contains(url, "abc123.mp3") {
		t.Fatalf("got url %q, want it to reference the bucket and key", url)
	}
	if !strings.Contains(url, "X-Amz-Signature") {
		t.Fatalf("got url %q, want a signed URL (X-Amz-Signature query param)", url)
	}
}
