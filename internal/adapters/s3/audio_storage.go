package s3

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/aws/smithy-go"

	"rioaudioguide/backend/internal/ports"
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
		var apiErr smithy.APIError
		if errors.As(err, &apiErr) && isPermanentS3Error(apiErr.ErrorCode()) {
			return "", &ports.PermanentError{StatusCode: 0, Body: apiErr.ErrorMessage()}
		}
		return "", err
	}
	return fmt.Sprintf("s3://%s/%s", a.bucket, key), nil
}

// isPermanentS3Error : codes qui ne se résoudront jamais en réessayant la même
// requête — credentials invalides, permissions refusées, bucket absent.
// Tout le reste (SlowDown, InternalError, ServiceUnavailable, timeouts réseau)
// reste transitoire, géré par le Nack(requeue=true) déjà en place.
func isPermanentS3Error(code string) bool {
	switch code {
	case "InvalidAccessKeyId", "AccessDenied", "SignatureDoesNotMatch", "NoSuchBucket":
		return true
	default:
		return false
	}
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
