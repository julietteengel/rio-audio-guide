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

func NewSender(client sesAPI, fromEmail string) *Sender {
	return &Sender{client: client, from: fromEmail}
}

func (s *Sender) SendVerificationCode(ctx context.Context, toEmail, code string) error {
	subject := "Your Memória Carioca verification code"
	body := fmt.Sprintf("Your verification code is %s. It expires in 15 minutes.", code)

	_, err := s.client.SendEmail(ctx, &sesv2.SendEmailInput{
		FromEmailAddress: &s.from,
		Destination: &types.Destination{
			ToAddresses: []string{toEmail},
		},
		Content: &types.EmailContent{
			Simple: &types.Message{
				Subject: &types.Content{Data: &subject},
				Body: &types.Body{
					Text: &types.Content{Data: &body},
				},
			},
		},
	})
	return err
}
