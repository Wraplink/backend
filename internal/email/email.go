package email

import "context"

type VerificationEmail struct {
	To       string
	FullName string
	Token    string
	Locale   string
}

type Sender interface {
	SendVerificationEmail(
		ctx context.Context,
		message VerificationEmail,
	) error
}
