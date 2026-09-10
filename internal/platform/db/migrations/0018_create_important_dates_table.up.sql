CREATE TABLE IF NOT EXISTS important_dates (
    id            UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    event_date    DATE NOT NULL,
    title         TEXT NOT NULL,
    details_en    TEXT NOT NULL,
    details_bn    TEXT NOT NULL,
    display_order INTEGER NOT NULL DEFAULT 0,
    is_active     BOOLEAN NOT NULL DEFAULT true,
    created_at    TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at    TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- Partial index: the client-facing list only ever reads active rows,
-- mirroring the notices board.
CREATE INDEX IF NOT EXISTS idx_important_dates_active_display_order
    ON important_dates (display_order)
    WHERE is_active;
