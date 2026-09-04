DROP INDEX IF EXISTS idx_users_verification_status;

ALTER TABLE users DROP CONSTRAINT IF EXISTS chk_users_verification_status;

ALTER TABLE users
    DROP COLUMN IF EXISTS profile_picture,
    DROP COLUMN IF EXISTS verification_doc,
    DROP COLUMN IF EXISTS verification_status;
