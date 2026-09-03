-- Auth is now email/password + Google only. Phone numbers, SMS OTPs and
-- the shared otps table (which carried both email and phone codes) all go
-- away; email verification state moves onto the user row so a user can
-- only ever have one outstanding code.

ALTER TABLE users
    DROP COLUMN IF EXISTS phone_number,
    DROP COLUMN IF EXISTS is_phone_verified;

DROP TABLE IF EXISTS otps;

-- Rename in place rather than add-and-backfill so already-verified users
-- keep their verified status across the deploy. Guarded because a database
-- created after this migration lands never had the old column.
DO $$
BEGIN
    IF EXISTS (
        SELECT 1 FROM information_schema.columns
        WHERE table_name = 'users' AND column_name = 'is_email_verified'
    ) THEN
        ALTER TABLE users RENAME COLUMN is_email_verified TO email_verified;
    END IF;
END $$;

ALTER TABLE users
    ADD COLUMN IF NOT EXISTS email_verified    BOOLEAN NOT NULL DEFAULT false,
    ADD COLUMN IF NOT EXISTS email_otp         VARCHAR(10),
    ADD COLUMN IF NOT EXISTS email_otp_expiry  TIMESTAMPTZ;
