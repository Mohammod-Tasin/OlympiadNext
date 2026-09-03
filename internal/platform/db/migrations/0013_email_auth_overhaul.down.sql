-- Reverse 0013. Phone numbers and prior phone-verification state were
-- destroyed by the up migration and cannot be recovered.

CREATE TABLE IF NOT EXISTS otps (
    id          UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id     UUID NOT NULL REFERENCES users (id) ON DELETE CASCADE,
    target_type TEXT NOT NULL CHECK (target_type IN ('email', 'phone')),
    code        VARCHAR(255) NOT NULL,
    expires_at  TIMESTAMPTZ NOT NULL,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX IF NOT EXISTS idx_otps_user_id_target_type ON otps (user_id, target_type);

ALTER TABLE users DROP COLUMN IF EXISTS email_otp;
ALTER TABLE users DROP COLUMN IF EXISTS email_otp_expiry;
ALTER TABLE users ADD COLUMN IF NOT EXISTS is_phone_verified BOOLEAN NOT NULL DEFAULT false;
ALTER TABLE users ADD COLUMN IF NOT EXISTS phone_number VARCHAR(20) UNIQUE;
ALTER TABLE users RENAME COLUMN email_verified TO is_email_verified;
