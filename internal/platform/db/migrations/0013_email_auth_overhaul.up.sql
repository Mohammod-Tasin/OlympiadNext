-- Consolidate authentication to email/password (with an emailed OTP) and
-- Google sign-in only. All phone-number / SMS-OTP schema is removed.

-- The verification flag takes its final name.
ALTER TABLE users RENAME COLUMN is_email_verified TO email_verified;

-- Drop phone auth columns (phone_number also drops its UNIQUE index).
ALTER TABLE users DROP COLUMN IF EXISTS phone_number;
ALTER TABLE users DROP COLUMN IF EXISTS is_phone_verified;

-- The email OTP now lives on the user row: a single active code per user,
-- stored in plaintext (short-lived, 6 digits) with its expiry.
ALTER TABLE users ADD COLUMN IF NOT EXISTS email_otp        VARCHAR(10);
ALTER TABLE users ADD COLUMN IF NOT EXISTS email_otp_expiry TIMESTAMPTZ;

-- The shared email/phone OTP table is superseded by the columns above.
DROP TABLE IF EXISTS otps;
