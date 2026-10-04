package email

import (
	"context"
	"fmt"
	"log/slog"
	"net/url"
	"strings"
)

type LogSender struct {
	logger      *slog.Logger
	frontendURL string
}

func NewLogSender(
	logger *slog.Logger,
	frontendURL string,
) *LogSender {
	return &LogSender{
		logger:      logger,
		frontendURL: strings.TrimRight(frontendURL, "/"),
	}
}

func (s *LogSender) SendVerificationEmail(
	ctx context.Context,
	message VerificationEmail,
) error {
	verificationURL := fmt.Sprintf(
		"%s/auth/verify-email?token=%s",
		s.frontendURL,
		url.QueryEscape(message.Token),
	)

	// DEVELOPMENT ONLY.
	//
	// Never log verification tokens or verification URLs
	// in production.
	s.logger.InfoContext(
		ctx,
		"verification email generated",
		"email", message.To,
		"locale", message.Locale,
		"verification_url", verificationURL,
	)

	return nil
}
