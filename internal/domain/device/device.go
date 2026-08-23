package device

import "time"

type Device struct {
	ID                string
	UserID            string
	DeviceFingerprint string
	LastLoginAt       time.Time
	CreatedAt         time.Time
}
