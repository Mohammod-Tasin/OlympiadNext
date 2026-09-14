-- Removes the unused general per-student admit card (0019). Confirmed
-- genuinely unused: no admin panel UI calls
-- POST /api/admin/users/{id}/admit-card, and no frontend reads
-- GET /api/auth/me's admit_card_url. The per-registration admit card on
-- exam_registrations (0015) is unaffected and remains the only system.

ALTER TABLE users DROP COLUMN IF EXISTS admit_card_url;
