package auth

import (
	"backend/internal/legal"
	"backend/internal/user"
	"context"
	"errors"
	"strings"

	"github.com/jackc/pgx/v5"
)

type googleProfile struct {
	Subject       string `json:"sub"`
	Email         string `json:"email"`
	EmailVerified bool   `json:"email_verified"`
	Name          string `json:"name"`
}

func (h *GoogleAuthHandler) account(ctx context.Context, profile googleProfile, flow googleFlow, req LoginRequest) (*LoginUser, error) {
	if profile.Subject == "" || len(profile.Subject) > 255 || !profile.EmailVerified {
		return nil, ErrInvalidCredentials
	}

	const lookup = `
		SELECT u.id, u.email, u.full_name, u.email_verified, u.account_status, u.locked_until
		FROM google_identities g JOIN users u ON u.id = g.user_id
		WHERE g.subject = $1
	`
	var account LoginUser
	err := h.db.QueryRow(ctx, lookup, profile.Subject).Scan(
		&account.ID, &account.Email, &account.FullName, &account.EmailVerified,
		&account.AccountStatus, &account.LockedUntil,
	)
	if err == nil {
		if account.AccountStatus != "active" || !account.EmailVerified || isLocked(&account) {
			return nil, ErrInvalidCredentials
		}
		return &account, nil
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return nil, err
	}
	if !flow.Register || flow.TermsVersion != legal.CurrentTermsVersion || flow.PrivacyVersion != legal.CurrentPrivacyVersion {
		return nil, ErrInvalidCredentials
	}

	account.Email = user.NormalizeEmail(profile.Email)
	account.FullName = strings.TrimSpace(profile.Name)
	if validateEmail(account.Email) != nil || validateFullName(account.FullName) != nil {
		return nil, ErrInvalidRegistration
	}

	tx, err := h.db.Begin(ctx)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback(ctx)

	err = tx.QueryRow(ctx, `
		INSERT INTO users (email, full_name, account_status, email_verified, last_login_ip)
		VALUES ($1, $2, 'active', true, $3)
		RETURNING id
	`, account.Email, account.FullName, req.IPAddress).Scan(&account.ID)
	if err != nil {
		return nil, err
	}
	if _, err = tx.Exec(ctx, `INSERT INTO google_identities (subject, user_id) VALUES ($1, $2)`, profile.Subject, account.ID); err != nil {
		return nil, err
	}
	if _, err = tx.Exec(ctx, `
		INSERT INTO legal_acceptances (user_id, terms_version, privacy_version, marketing_consent, ip_address)
		VALUES ($1, $2, $3, $4, $5)
	`, account.ID, flow.TermsVersion, flow.PrivacyVersion, flow.MarketingConsent, req.IPAddress); err != nil {
		return nil, err
	}
	if _, err = tx.Exec(ctx, `
		INSERT INTO security_events (user_id, event_type, ip_address, user_agent)
		VALUES ($1, 'ACCOUNT_REGISTERED', $2, $3)
	`, account.ID, req.IPAddress, req.UserAgent); err != nil {
		return nil, err
	}
	if err := tx.Commit(ctx); err != nil {
		return nil, err
	}
	return &account, nil
}
