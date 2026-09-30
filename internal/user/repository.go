package user

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

var ErrEmailAlreadyExists = errors.New(
	"If registration can be completed, you will receive a verification email.",
)

var ErrUserNotFound = errors.New(
	"user not found",
)

type User struct {
	ID                  uuid.UUID
	Email               string
	FullName            string
	EmailVerified       bool
	AccountStatus       string
	FailedLoginAttempts int
	LockedUntil         *time.Time
	LastLoginAt         *time.Time
	LastLoginIP         *string
	CreatedAt           time.Time
	UpdatedAt           time.Time
}

type Repository struct {
	db *pgxpool.Pool
}

func NewRepository(
	db *pgxpool.Pool,
) *Repository {
	return &Repository{
		db: db,
	}
}

func NormalizeEmail(email string) string {
	return strings.ToLower(
		strings.TrimSpace(email),
	)
}

func (r *Repository) FindByID(
	ctx context.Context,
	id uuid.UUID,
) (*User, error) {
	const query = `
		SELECT
			id,
			email,
			full_name,
			email_verified,
			account_status,
			failed_login_attempts,
			locked_until,
			last_login_at,
			host(last_login_ip),
			created_at,
			updated_at
		FROM users
		WHERE id = $1
	`

	var u User

	err := r.db.QueryRow(ctx, query, id).Scan(
		&u.ID,
		&u.Email,
		&u.FullName,
		&u.EmailVerified,
		&u.AccountStatus,
		&u.FailedLoginAttempts,
		&u.LockedUntil,
		&u.LastLoginAt,
		&u.LastLoginIP,
		&u.CreatedAt,
		&u.UpdatedAt,
	)

	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrUserNotFound
	}

	if err != nil {
		return nil, fmt.Errorf("find user by id: %w", err)
	}

	return &u, nil
}

func (r *Repository) FindByEmail(
	ctx context.Context,
	email string,
) (*User, error) {
	email = NormalizeEmail(email)

	const query = `
		SELECT
			id,
			email,
			full_name,
			email_verified,
			account_status,
			failed_login_attempts,
			locked_until,
			last_login_at,
			host(last_login_ip),
			created_at,
			updated_at
		FROM users
		WHERE email = $1
	`

	var user User

	err := r.db.QueryRow(
		ctx,
		query,
		email,
	).Scan(
		&user.ID,
		&user.Email,
		&user.FullName,
		&user.EmailVerified,
		&user.AccountStatus,
		&user.FailedLoginAttempts,
		&user.LockedUntil,
		&user.LastLoginAt,
		&user.LastLoginIP,
		&user.CreatedAt,
		&user.UpdatedAt,
	)

	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrUserNotFound
	}

	if err != nil {
		return nil, fmt.Errorf(
			"find user by email: %w",
			err,
		)
	}

	return &user, nil
}
