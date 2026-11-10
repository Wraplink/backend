package auth

import (
	"context"
	"errors"
	"fmt"
	"net"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	"backend/internal/token"
)

var (
	ErrInvalidVerificationToken = errors.New("invalid verification token")
)

type EmailVerificationService struct {
	db *pgxpool.Pool
}

func NewEmailVerificationService(
	db *pgxpool.Pool,
) *EmailVerificationService {
	return &EmailVerificationService{
		db: db,
	}
}

type VerifyEmailRequest struct {
	Token     string
	IPAddress net.IP
	UserAgent string
}

func (s *EmailVerificationService) Verify(
	ctx context.Context,
	req VerifyEmailRequest,
) error {
	rawToken := strings.TrimSpace(req.Token)

	if rawToken == "" {
		return ErrInvalidVerificationToken
	}

	tokenHash := token.Hash(rawToken)

	tx, err := s.db.Begin(ctx)
	if err != nil {
		return fmt.Errorf(
			"begin email verification transaction: %w",
			err,
		)
	}

	defer func() {
		_ = tx.Rollback(ctx)
	}()

	var (
		verificationID uuid.UUID
		userID         uuid.UUID
		expiresAt      time.Time
		usedAt         *time.Time
	)

	const findVerification = `
		SELECT
			id,
			user_id,
			expires_at,
			used_at
		FROM email_verifications
		WHERE token_hash = $1
		FOR UPDATE
	`

	err = tx.QueryRow(
		ctx,
		findVerification,
		tokenHash,
	).Scan(
		&verificationID,
		&userID,
		&expiresAt,
		&usedAt,
	)

	if err != nil {
		return ErrInvalidVerificationToken
	}

	if usedAt != nil {
		return ErrInvalidVerificationToken
	}

	if !time.Now().Before(expiresAt) {
		return ErrInvalidVerificationToken
	}

	const markUsed = `
		UPDATE email_verifications
		SET used_at = now()
		WHERE id = $1
	`

	if _, err := tx.Exec(
		ctx,
		markUsed,
		verificationID,
	); err != nil {
		return fmt.Errorf(
			"mark email verification used: %w",
			err,
		)
	}

	const activateUser = `
		UPDATE users
		SET
			email_verified = TRUE,
			account_status = 'active',
			updated_at = now()
		WHERE id = $1
		  AND email_verified = FALSE
	`

	result, err := tx.Exec(
		ctx,
		activateUser,
		userID,
	)

	if err != nil {
		return fmt.Errorf(
			"activate verified user: %w",
			err,
		)
	}

	if result.RowsAffected() != 1 {
		return ErrInvalidVerificationToken
	}

	const securityEvent = `
		INSERT INTO security_events (
			user_id,
			event_type,
			ip_address,
			user_agent
		)
		VALUES (
			$1,
			'EMAIL_VERIFIED',
			$2,
			$3
		)
	`

	if _, err := tx.Exec(
		ctx,
		securityEvent,
		userID,
		req.IPAddress,
		req.UserAgent,
	); err != nil {
		return fmt.Errorf(
			"create email verification security event: %w",
			err,
		)
	}

	// Reserve the highest-priority eligible starter offer in the same
	// transaction as verification. This is an onboarding-only event:
	// the verification token is single-use, and replays cannot award again.
	// No subscription or network entitlement is activated here.
	const reserveStarter = `
		INSERT INTO free_plan_claims
			(id, user_id, plan_id, plan_code, starter_policy_id, source, state, reservation_expires_at)
		SELECT gen_random_uuid(), $1, p.plan_id, catalog.code, p.id, 'starter', 'reserved',
			now() + (p.reservation_ttl_seconds * interval '1 second')
		FROM starter_plan_policies p
		JOIN plans catalog ON catalog.id = p.plan_id
		WHERE p.enabled = TRUE
			AND p.assignment_trigger = 'email_verified'
			AND p.region = ''
			AND catalog.price_minor = 0
			AND catalog.active = TRUE
			AND catalog.archived_at IS NULL
			AND (catalog.publish_at IS NULL OR catalog.publish_at <= now())
			AND NOT EXISTS (
				SELECT 1 FROM free_plan_claims c
				WHERE c.user_id = $1 AND c.source = 'starter'
			)
			AND NOT EXISTS (
				SELECT 1 FROM free_plan_claims c
				WHERE c.user_id = $1 AND (c.plan_id = p.plan_id OR lower(c.plan_code) = lower(catalog.code))
			)
		ORDER BY p.priority DESC, p.id ASC
		LIMIT 1
		ON CONFLICT DO NOTHING
	`
	if _, err := tx.Exec(ctx, reserveStarter, userID); err != nil {
		return fmt.Errorf("reserve starter offer after verification: %w", err)
	}

	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf(
			"commit email verification: %w",
			err,
		)
	}

	return nil
}
