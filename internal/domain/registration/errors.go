package registration

import "errors"

var (
	ErrNotFound                  = errors.New("registration not found")
	ErrAlreadyRegistered         = errors.New("already registered for this event")
	ErrDuplicateTransactionID    = errors.New("this transaction id has already been submitted")
	ErrAlreadyReviewed           = errors.New("registration has already been reviewed")
	ErrInvalidUnrejectTransition = errors.New("only a rejected registration can be returned to pending")
)
