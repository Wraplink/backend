package auth

import (
	"context"
	"errors"
	"fmt"
	"net"

	"backend/internal/password"

	"github.com/google/uuid"
)

var (
	ErrInvalidCurrentPassword = errors.New("current password is incorrect")
	ErrPasswordUnchanged      = errors.New("new password must differ from current password")
)

type PasswordChangeRequest struct {
	UserID          uuid.UUID
	CurrentPassword string
	NewPassword     string
	IPAddress       net.IP
	UserAgent       string
}

type PasswordChangeService struct {
	repository *PasswordChangeRepository
}

func NewPasswordChangeService(
	repository *PasswordChangeRepository,
) *PasswordChangeService {
	if repository == nil {
		panic("auth: nil password change repository")
	}

	return &PasswordChangeService{repository: repository}
}

func (s *PasswordChangeService) Change(
	ctx context.Context,
	req PasswordChangeRequest,
) error {
	if req.UserID == uuid.Nil {
		return errors.New("invalid user ID")
	}

	if req.CurrentPassword == "" {
		return ErrInvalidCurrentPassword
	}

	if err := validatePassword(req.NewPassword); err != nil {
		return err
	}

	currentHash, err := s.repository.FindPasswordHash(ctx, req.UserID)
	if err != nil {
		return fmt.Errorf("load current password: %w", err)
	}

	if !password.Verify(req.CurrentPassword, currentHash) {
		return ErrInvalidCurrentPassword
	}

	if password.Verify(req.NewPassword, currentHash) {
		return ErrPasswordUnchanged
	}

	newHash, err := password.Hash(req.NewPassword)
	if err != nil {
		return fmt.Errorf("hash new password: %w", err)
	}

	if err := s.repository.Change(
		ctx,
		req.UserID,
		newHash,
		req.IPAddress,
		req.UserAgent,
	); err != nil {
		return fmt.Errorf("change password: %w", err)
	}

	return nil
}
