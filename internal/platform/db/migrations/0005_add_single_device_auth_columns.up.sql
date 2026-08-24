ALTER TABLE users
    ADD COLUMN IF NOT EXISTS active_device_fingerprint VARCHAR(255),
    ADD COLUMN IF NOT EXISTS is_email_verified BOOLEAN NOT NULL DEFAULT false,
    ADD COLUMN IF NOT EXISTS is_phone_verified BOOLEAN NOT NULL DEFAULT false;
