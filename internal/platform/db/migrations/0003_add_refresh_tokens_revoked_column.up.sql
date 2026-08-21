ALTER TABLE refresh_tokens
    ADD COLUMN revoked BOOLEAN GENERATED ALWAYS AS (revoked_at IS NOT NULL) STORED;

CREATE INDEX IF NOT EXISTS idx_refresh_tokens_revoked ON refresh_tokens (revoked);
