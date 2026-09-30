package auth

import (
	"errors"
	"net/mail"
	"strings"
	"unicode"
	"unicode/utf8"
)

var (
	ErrInvalidEmail    = errors.New("invalid email")
	ErrWeakPassword    = errors.New("weak password")
	ErrInvalidFullName = errors.New("invalid full name")
)

func validateRegistrationInput(
	fullName string,
	email string,
	password string,
) error {
	fullName = strings.TrimSpace(fullName)
	email = strings.TrimSpace(email)

	if fullName == "" || utf8.RuneCountInString(fullName) > 200 {
		return ErrInvalidFullName
	}

	if _, err := mail.ParseAddress(email); err != nil {
		return ErrInvalidEmail
	}

	if len(email) > 254 {
		return ErrInvalidEmail
	}

	if err := validatePassword(password); err != nil {
		return err
	}

	return nil
}

func validatePassword(password string) error {
	if len(password) < 12 || len(password) > 128 {
		return ErrWeakPassword
	}

	var (
		hasUpper   bool
		hasLower   bool
		hasNumber  bool
		hasSpecial bool
	)

	for _, r := range password {
		switch {
		case unicode.IsUpper(r):
			hasUpper = true
		case unicode.IsLower(r):
			hasLower = true
		case unicode.IsDigit(r):
			hasNumber = true
		case unicode.IsPunct(r) || unicode.IsSymbol(r):
			hasSpecial = true
		}
	}

	if !hasUpper || !hasLower || !hasNumber || !hasSpecial {
		return ErrWeakPassword
	}

	return nil
}
