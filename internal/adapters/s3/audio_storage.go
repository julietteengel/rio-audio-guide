package s3

import (
	"bytes"
	"context"
	"fmt"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/s3"
)

type AudioStorage struct {
	client        *s3.Client
	presignClient *s3.PresignClient
	bucket        string
}

func NewAudioStorage(client *s3.Client, bucket string) *AudioStorage {
	return &AudioStorage{client: client, presignClient: s3.NewPresignClient(client), bucket: bucket}
}

func (a *AudioStorage) Upload(ctx context.Context, key string, data []byte, contentType string) (string, error) {
	_, err := a.client.PutObject(ctx, &s3.PutObjectInput{
		Bucket:      aws.String(a.bucket),
		Key:         aws.String(key),
		Body:        bytes.NewReader(data),
		ContentType: aws.String(contentType),
	})
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("s3://%s/%s", a.bucket, key), nil
}

// PresignURL rend le fichier stocké chargeable directement par un client HTTP
// (navigateur, app mobile) — storage_url (s3://bucket/clé) n'est pas une URL
// HTTP utilisable telle quelle. Ne fait aucun appel réseau : la signature est
// calculée localement à partir des credentials du client.
func (a *AudioStorage) PresignURL(ctx context.Context, key string, expiry time.Duration) (string, error) {
	req, err := a.presignClient.PresignGetObject(ctx, &s3.GetObjectInput{
		Bucket: aws.String(a.bucket),
		Key:    aws.String(key),
	}, s3.WithPresignExpires(expiry))
	if err != nil {
		return "", err
	}
	return req.URL, nil
}
