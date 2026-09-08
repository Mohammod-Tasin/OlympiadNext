-- Per-user delivery channel for transactional notifications (currently the
-- admit-card-ready alert). Email is always available and is the default;
-- a user may opt into SMS instead, but only after proving the number with
-- an OTP.
--
--   notification_method            — the channel actually used: 'email' until
--                                    a phone number is verified, then 'phone'
--   notification_phone             — the number an OTP was last sent to
--   notification_phone_verified    — true once that number passed OTP check
--   notification_phone_otp / _expiry — the outstanding 6-digit code, mirrors
--                                    the email_otp / email_otp_expiry columns
--
-- These are deliberately separate columns from the removed phone/SMS *auth*
-- fields (migration 0011 dropped phone_number / is_phone_verified): this is
-- a notification preference, not a login identity.

ALTER TABLE users
    ADD COLUMN IF NOT EXISTS notification_method           VARCHAR(10) NOT NULL DEFAULT 'email',
    ADD COLUMN IF NOT EXISTS notification_phone            VARCHAR(20),
    ADD COLUMN IF NOT EXISTS notification_phone_verified   BOOLEAN NOT NULL DEFAULT false,
    ADD COLUMN IF NOT EXISTS notification_phone_otp        VARCHAR(10),
    ADD COLUMN IF NOT EXISTS notification_phone_otp_expiry TIMESTAMPTZ;

-- Postgres has no ADD CONSTRAINT IF NOT EXISTS; guard it so a rerun is safe.
DO $$
BEGIN
    IF NOT EXISTS (
        SELECT 1 FROM pg_constraint WHERE conname = 'chk_users_notification_method'
    ) THEN
        ALTER TABLE users
            ADD CONSTRAINT chk_users_notification_method
            CHECK (notification_method IN ('email', 'phone'));
    END IF;
END $$;
