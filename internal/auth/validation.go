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
	locale string,
) error {
	fullName = strings.TrimSpace(fullName)
	email = strings.TrimSpace(email)

	if err := validateFullName(fullName); err != nil {
		return err
	}

	if err := validateEmail(email); err != nil {
		return err
	}

	if err := validatePassword(password); err != nil {
		return err
	}

	if err := validateLocale(locale); err != nil {
		return err
	}

	return nil
}

func validateFullName(value string) error {
	if value == "" {
		return ErrInvalidFullName
	}

	length := utf8.RuneCountInString(value)

	if length < 2 || length > 200 {
		return ErrInvalidFullName
	}

	for _, r := range value {
		if unicode.IsControl(r) {
			return ErrInvalidFullName
		}
	}

	return nil
}

func validateEmail(value string) error {
	if value == "" || len(value) > 254 {
		return ErrInvalidEmail
	}

	// Reject display-name format:
	// "John Doe <john@example.com>"
	//
	// We only accept a plain email address.
	parsed, err := mail.ParseAddress(value)
	if err != nil {
		return ErrInvalidEmail
	}

	if !strings.EqualFold(parsed.Address, value) {
		return ErrInvalidEmail
	}

	return nil
}

func validatePassword(password string) error {
	if password == "" {
		return ErrWeakPassword
	}

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

	if !hasUpper ||
		!hasLower ||
		!hasNumber ||
		!hasSpecial {
		return ErrWeakPassword
	}

	return nil
}

var ErrInvalidLocale = errors.New("invalid locale")

func validateLocale(locale string) error {
	switch locale {
	case "en", "fa":
		return nil
	default:
		return ErrInvalidLocale
	}
}
