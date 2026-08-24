ALTER TABLE users
    DROP COLUMN IF EXISTS active_device_fingerprint,
    DROP COLUMN IF EXISTS is_email_verified,
    DROP COLUMN IF EXISTS is_phone_verified;
