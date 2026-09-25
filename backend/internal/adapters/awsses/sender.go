// backend/internal/adapters/awsses/sender.go
package awsses

import (
	"context"

	"github.com/aws/aws-sdk-go-v2/service/sesv2"
	"github.com/aws/aws-sdk-go-v2/service/sesv2/types"
)

// sesAPI exposes only the one method used here -- *sesv2.Client satisfies
// it structurally, same pattern already used by internal/adapters/awspolly
// (pollyAPI) for testing without simulating the SDK's own request signing.
type sesAPI interface {
	SendEmail(ctx context.Context, params *sesv2.SendEmailInput, optFns ...func(*sesv2.Options)) (*sesv2.SendEmailOutput, error)
}

type Sender struct {
	client sesAPI
	from   string
}

func NewSender(client sesAPI, fromEmail string) *Sender {
	return &Sender{client: client, from: fromEmail}
}

func (s *Sender) SendVerificationCode(ctx context.Context, toEmail, code, language string) error {
	c := verificationCopyFor(language)
	return s.send(ctx, toEmail, c.subject, c.heading, c.body, code)
}

func (s *Sender) SendPasswordResetCode(ctx context.Context, toEmail, code, language string) error {
	c := resetCopyFor(language)
	return s.send(ctx, toEmail, c.subject, c.heading, c.body, code)
}

func (s *Sender) send(ctx context.Context, toEmail, subject, heading, body, code string) error {
	html := renderHTML(heading, body, code)
	text := renderText(body, code)

	_, err := s.client.SendEmail(ctx, &sesv2.SendEmailInput{
		FromEmailAddress: &s.from,
		Destination: &types.Destination{
			ToAddresses: []string{toEmail},
		},
		Content: &types.EmailContent{
			Simple: &types.Message{
				Subject: &types.Content{Data: &subject},
				Body: &types.Body{
					Html: &types.Content{Data: &html},
					Text: &types.Content{Data: &text},
				},
			},
		},
	})
	return err
}
