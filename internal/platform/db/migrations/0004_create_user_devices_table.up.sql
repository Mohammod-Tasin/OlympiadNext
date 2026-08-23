CREATE TABLE IF NOT EXISTS user_devices (
    id                 UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id            UUID NOT NULL REFERENCES users (id) ON DELETE CASCADE,
    device_fingerprint VARCHAR(255) NOT NULL,
    last_login_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
    created_at         TIMESTAMPTZ NOT NULL DEFAULT now(),
    CONSTRAINT uq_user_devices_user_id_fingerprint UNIQUE (user_id, device_fingerprint)
);

CREATE INDEX IF NOT EXISTS idx_user_devices_user_id ON user_devices (user_id);
