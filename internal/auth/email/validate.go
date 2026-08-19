package email

import (
	"errors"
	"net/mail"
)

const minPasswordLength = 8

var (
	ErrInvalidEmail = errors.New("invalid email address")
	ErrWeakPassword = errors.New("password must be at least 8 characters")
)

func ValidateEmail(raw string) error {
	if _, err := mail.ParseAddress(raw); err != nil {
		return ErrInvalidEmail
	}
	return nil
}

func ValidatePasswordStrength(plain string) error {
	if len(plain) < minPasswordLength {
		return ErrWeakPassword
	}
	return nil
}
