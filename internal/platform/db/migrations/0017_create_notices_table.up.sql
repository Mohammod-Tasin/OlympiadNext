CREATE TABLE IF NOT EXISTS notices (
    id            UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    text_en       TEXT NOT NULL,
    text_bn       TEXT NOT NULL,
    display_order INTEGER NOT NULL DEFAULT 0,
    is_active     BOOLEAN NOT NULL DEFAULT true,
    created_at    TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at    TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- Partial index: the client-facing board only ever lists active notices,
-- ordered by display_order.
CREATE INDEX IF NOT EXISTS idx_notices_active_display_order
    ON notices (display_order)
    WHERE is_active;
