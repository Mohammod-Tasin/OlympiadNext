package postgres

import (
	"context"
	"database/sql"
	"fmt"
)

type DeviceRepository struct {
	db *sql.DB
}

func NewDeviceRepository(db *sql.DB) *DeviceRepository {
	return &DeviceRepository{db: db}
}

func (r *DeviceRepository) UpsertDevice(ctx context.Context, userID, deviceFingerprint string) error {
	const q = `
		INSERT INTO user_devices (id, user_id, device_fingerprint, last_login_at, created_at)
		VALUES (gen_random_uuid(), $1, $2, now(), now())
		ON CONFLICT (user_id, device_fingerprint)
		DO UPDATE SET last_login_at = now()`

	if _, err := r.db.ExecContext(ctx, q, userID, deviceFingerprint); err != nil {
		return fmt.Errorf("device_repository: upsert device failed: %w", err)
	}
	return nil
}
