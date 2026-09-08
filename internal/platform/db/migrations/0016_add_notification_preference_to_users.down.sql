ALTER TABLE users DROP CONSTRAINT IF EXISTS chk_users_notification_method;

ALTER TABLE users
    DROP COLUMN IF EXISTS notification_method,
    DROP COLUMN IF EXISTS notification_phone,
    DROP COLUMN IF EXISTS notification_phone_verified,
    DROP COLUMN IF EXISTS notification_phone_otp,
    DROP COLUMN IF EXISTS notification_phone_otp_expiry;
