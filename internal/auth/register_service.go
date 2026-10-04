package auth

import (
	"backend/internal/legal"
	"context"
	"errors"
	"fmt"
	"net"
	"strings"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"

	"backend/internal/password"
	"backend/internal/token"
	"backend/internal/user"
)

var (
	ErrInvalidRegistration = errors.New(
		"invalid registration data",
	)

	ErrEmailAlreadyExists = errors.New(
		"If registration can be completed, you will receive a verification email.",
	)
)

type RegisterRequest struct {
	FullName         string
	Email            string
	Password         string
	TermsVersion     string
	PrivacyVersion   string
	MarketingConsent bool
	IPAddress        net.IP
	UserAgent        string
}

type RegisterResult struct {
	UserID            uuid.UUID
	EmailVerification string
}

type RegistrationService struct {
	db *pgxpool.Pool
}

func NewRegistrationService(
	db *pgxpool.Pool,
) *RegistrationService {
	return &RegistrationService{
		db: db,
	}
}

func (s *RegistrationService) Register(
	ctx context.Context,
	req RegisterRequest,
) (*RegisterResult, error) {

	fullName := strings.TrimSpace(req.FullName)
	email := user.NormalizeEmail(req.Email)

	if err := validateRegistrationInput(
		fullName,
		email,
		req.Password,
	); err != nil {
		return nil, err
	}

	if req.TermsVersion != legal.CurrentTermsVersion {
		return nil, ErrInvalidRegistration
	}

	if req.PrivacyVersion != legal.CurrentPrivacyVersion {
		return nil, ErrInvalidRegistration
	}

	passwordHash, err := password.Hash(
		req.Password,
	)
	if err != nil {
		return nil, fmt.Errorf(
			"hash password: %w",
			err,
		)
	}

	verificationToken, err := token.Generate(32)
	if err != nil {
		return nil, fmt.Errorf(
			"generate verification token: %w",
			err,
		)
	}

	verificationHash := token.Hash(
		verificationToken,
	)

	tx, err := s.db.Begin(ctx)
	if err != nil {
		return nil, fmt.Errorf(
			"begin registration transaction: %w",
			err,
		)
	}

	defer func() {
		_ = tx.Rollback(ctx)
	}()

	var userID uuid.UUID

	const createUser = `
		INSERT INTO users (
			email,
			full_name,
			last_login_ip,
			account_status
		)
		VALUES ($1, $2, $3, 'pending')
		RETURNING id
	`

	err = tx.QueryRow(
		ctx,
		createUser,
		email,
		fullName,
		req.IPAddress,
	).Scan(&userID)

	if err != nil {
		if isUniqueViolation(err) {
			return nil, ErrEmailAlreadyExists
		}

		return nil, fmt.Errorf(
			"create user: %w",
			err,
		)
	}

	const createCredentials = `
		INSERT INTO user_credentials (
			user_id,
			password_hash
		)
		VALUES ($1, $2)
	`

	_, err = tx.Exec(
		ctx,
		createCredentials,
		userID,
		passwordHash,
	)

	if err != nil {
		return nil, fmt.Errorf(
			"create credentials: %w",
			err,
		)
	}

	const createLegalAcceptance = `
		INSERT INTO legal_acceptances (
			user_id,
			terms_version,
			privacy_version,
			marketing_consent,
			ip_address
		)
		VALUES ($1, $2, $3, $4, $5)
	`

	_, err = tx.Exec(
		ctx,
		createLegalAcceptance,
		userID,
		req.TermsVersion,
		req.PrivacyVersion,
		req.MarketingConsent,
		req.IPAddress,
	)

	if err != nil {
		return nil, fmt.Errorf(
			"record legal acceptance: %w",
			err,
		)
	}

	const createVerification = `
		INSERT INTO email_verifications (
			user_id,
			token_hash,
			expires_at
		)
		VALUES (
			$1,
			$2,
			now() + interval '24 hours'
		)
	`

	_, err = tx.Exec(
		ctx,
		createVerification,
		userID,
		verificationHash,
	)

	if err != nil {
		return nil, fmt.Errorf(
			"create email verification: %w",
			err,
		)
	}

	const createSecurityEvent = `
		INSERT INTO security_events (
			user_id,
			event_type,
			ip_address,
			user_agent
		)
		VALUES ($1, 'ACCOUNT_REGISTERED', $2, $3)
	`

	_, err = tx.Exec(
		ctx,
		createSecurityEvent,
		userID,
		req.IPAddress,
		req.UserAgent,
	)

	if err != nil {
		return nil, fmt.Errorf(
			"create security event: %w",
			err,
		)
	}

	if err := tx.Commit(ctx); err != nil {
		return nil, fmt.Errorf(
			"commit registration: %w",
			err,
		)
	}

	return &RegisterResult{
		UserID: userID,

		// This value is returned to the application
		// so the email service can send it.
		//
		// It is NEVER stored in plaintext in PostgreSQL.
		EmailVerification: verificationToken,
	}, nil
}

func isUniqueViolation(err error) bool {
	var pgErr *pgconn.PgError

	if !errors.As(err, &pgErr) {
		return false
	}

	return pgErr.Code == "23505"
}
