package user

import "errors"

var (
	ErrNotFound      = errors.New("user not found")
	ErrEmailTaken    = errors.New("email already registered")
	ErrGoogleIDTaken = errors.New("google account already linked to another user")
	ErrPhoneTaken    = errors.New("phone number already registered to another account")
)
