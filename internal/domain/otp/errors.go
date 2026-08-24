package otp

import "errors"

var ErrNotFound = errors.New("otp: no valid code found")
