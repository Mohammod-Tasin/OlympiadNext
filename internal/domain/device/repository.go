package device

import "context"

type Repository interface {
	UpsertDevice(ctx context.Context, userID, deviceFingerprint string) error
}
