package auth

import (
	"context"
	"errors"
	"testing"

	"github.com/google/uuid"
)

type mockSessionRevoker struct {
	revokeFunc func(
		ctx context.Context,
		refreshToken string,
	) (uuid.UUID, uuid.UUID, error)

	called       bool
	refreshToken string
}

func (m *mockSessionRevoker) RevokeByRefreshToken(
	ctx context.Context,
	refreshToken string,
) (uuid.UUID, uuid.UUID, error) {
	m.called = true
	m.refreshToken = refreshToken

	return m.revokeFunc(ctx, refreshToken)
}

func TestLogoutService_EmptyRefreshToken(t *testing.T) {
	repository := &mockSessionRevoker{
		revokeFunc: func(
			ctx context.Context,
			refreshToken string,
		) (uuid.UUID, uuid.UUID, error) {
			t.Fatal("repository should not be called")
			return uuid.Nil, uuid.Nil, nil
		},
	}

	service := NewLogoutService(repository)

	result, err := service.Logout(
		context.Background(),
		"",
	)

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if result == nil {
		t.Fatal("expected result")
	}

	if result.Revoked {
		t.Fatal("expected Revoked=false")
	}

	if result.SessionID != uuid.Nil {
		t.Fatalf(
			"expected empty session ID, got %s",
			result.SessionID,
		)
	}

	if result.UserID != uuid.Nil {
		t.Fatalf(
			"expected empty user ID, got %s",
			result.UserID,
		)
	}

	if repository.called {
		t.Fatal("repository should not have been called")
	}
}

func TestLogoutService_WhitespaceRefreshToken(t *testing.T) {
	repository := &mockSessionRevoker{
		revokeFunc: func(
			ctx context.Context,
			refreshToken string,
		) (uuid.UUID, uuid.UUID, error) {
			t.Fatal("repository should not be called")
			return uuid.Nil, uuid.Nil, nil
		},
	}

	service := NewLogoutService(repository)

	result, err := service.Logout(
		context.Background(),
		"   ",
	)

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if result == nil {
		t.Fatal("expected result")
	}

	if result.Revoked {
		t.Fatal("expected Revoked=false")
	}

	if repository.called {
		t.Fatal("repository should not have been called")
	}
}

func TestLogoutService_RevokesSession(t *testing.T) {
	sessionID := uuid.New()
	userID := uuid.New()

	repository := &mockSessionRevoker{
		revokeFunc: func(
			ctx context.Context,
			refreshToken string,
		) (uuid.UUID, uuid.UUID, error) {
			if refreshToken != "refresh-token-123" {
				t.Fatalf(
					"unexpected refresh token: %q",
					refreshToken,
				)
			}

			return sessionID, userID, nil
		},
	}

	service := NewLogoutService(repository)

	result, err := service.Logout(
		context.Background(),
		"refresh-token-123",
	)

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if result == nil {
		t.Fatal("expected result")
	}

	if !result.Revoked {
		t.Fatal("expected Revoked=true")
	}

	if result.SessionID != sessionID {
		t.Fatalf(
			"expected session ID %s, got %s",
			sessionID,
			result.SessionID,
		)
	}

	if result.UserID != userID {
		t.Fatalf(
			"expected user ID %s, got %s",
			userID,
			result.UserID,
		)
	}

	if !repository.called {
		t.Fatal("expected repository to be called")
	}

	if repository.refreshToken != "refresh-token-123" {
		t.Fatalf(
			"expected refresh token %q, got %q",
			"refresh-token-123",
			repository.refreshToken,
		)
	}
}

func TestLogoutService_AlreadyRevokedSession(t *testing.T) {
	repository := &mockSessionRevoker{
		revokeFunc: func(
			ctx context.Context,
			refreshToken string,
		) (uuid.UUID, uuid.UUID, error) {
			return uuid.Nil, uuid.Nil, nil
		},
	}

	service := NewLogoutService(repository)

	result, err := service.Logout(
		context.Background(),
		"already-revoked-token",
	)

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if result == nil {
		t.Fatal("expected result")
	}

	if result.Revoked {
		t.Fatal("expected Revoked=false")
	}

	if result.SessionID != uuid.Nil {
		t.Fatalf(
			"expected empty session ID, got %s",
			result.SessionID,
		)
	}

	if result.UserID != uuid.Nil {
		t.Fatalf(
			"expected empty user ID, got %s",
			result.UserID,
		)

	}
}

func TestLogoutService_RepositoryError(t *testing.T) {
	expectedErr := errors.New("database unavailable")

	repository := &mockSessionRevoker{
		revokeFunc: func(
			ctx context.Context,
			refreshToken string,
		) (uuid.UUID, uuid.UUID, error) {
			return uuid.Nil, uuid.Nil, expectedErr
		},
	}

	service := NewLogoutService(repository)

	result, err := service.Logout(
		context.Background(),
		"refresh-token-123",
	)

	if result != nil {
		t.Fatal("expected nil result on repository error")
	}

	if !errors.Is(err, expectedErr) {
		t.Fatalf(
			"expected error %v, got %v",
			expectedErr,
			err,
		)
	}
}

func TestLogoutService_TrimsRefreshToken(t *testing.T) {
	sessionID := uuid.New()
	userID := uuid.New()

	repository := &mockSessionRevoker{
		revokeFunc: func(
			ctx context.Context,
			refreshToken string,
		) (uuid.UUID, uuid.UUID, error) {
			if refreshToken != "refresh-token-123" {
				t.Fatalf(
					"expected trimmed token, got %q",
					refreshToken,
				)
			}

			return sessionID, userID, nil
		},
	}

	service := NewLogoutService(repository)

	result, err := service.Logout(
		context.Background(),
		"  refresh-token-123  ",
	)

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if result == nil {
		t.Fatal("expected result")
	}

	if !result.Revoked {
		t.Fatal("expected Revoked=true")
	}

	if result.SessionID != sessionID {
		t.Fatalf(
			"expected session ID %s, got %s",
			sessionID,
			result.SessionID,
		)
	}

	if result.UserID != userID {
		t.Fatalf(
			"expected user ID %s, got %s",
			userID,
			result.UserID,
		)
	}
}
