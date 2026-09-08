-- Admin-issued admit cards for an approved exam registration. The admin
-- uploads a PDF once the payment is confirmed; the student downloads it
-- through the owner-or-admin gated /uploads route.
--
-- admit_card_url stores the storage path only (e.g.
-- "/uploads/admit-cards/<userID>/<uuid>.pdf"), never the file itself.
-- Both columns stay NULL until an admin uploads the card.

ALTER TABLE exam_registrations
    ADD COLUMN IF NOT EXISTS admit_card_url         VARCHAR(255),
    ADD COLUMN IF NOT EXISTS admit_card_uploaded_at TIMESTAMPTZ;
