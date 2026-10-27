// backend/internal/adapters/awsses/sender.go
package awsses

import (
	"context"
	"fmt"

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

// NewSender sets the From header's display name to "Memória Carioca"
// alongside the raw address -- SES's FromEmailAddress accepts the standard
// RFC 5322 "Display Name <address>" form directly, and without it clients
// show the raw address (or whatever the mailbox's own contact name happens
// to be) instead of the brand name.
func NewSender(client sesAPI, fromEmail string) *Sender {
	return &Sender{client: client, from: fmt.Sprintf("Memória Carioca <%s>", fromEmail)}
}

func (s *Sender) SendVerificationCode(ctx context.Context, toEmail, code, language string) error {
	c := verificationCopyFor(language)
	return s.send(ctx, toEmail, c.subject, c.heading, c.body, code, footerFor(language))
}

func (s *Sender) SendPasswordResetCode(ctx context.Context, toEmail, code, language string) error {
	c := resetCopyFor(language)
	return s.send(ctx, toEmail, c.subject, c.heading, c.body, code, footerFor(language))
}

func (s *Sender) send(ctx context.Context, toEmail, subject, heading, body, code, footer string) error {
	html := renderHTML(heading, body, code, footer)
	text := renderText(body, code, footer)

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
