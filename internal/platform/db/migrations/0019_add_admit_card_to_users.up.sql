-- General per-student admit card, independent of the per-registration admit
-- card on exam_registrations (0015). Set by an admin via
-- POST /api/admin/users/{id}/admit-card, stored under uploads/users/<id>/
-- alongside the KYC document.

ALTER TABLE users
    ADD COLUMN IF NOT EXISTS admit_card_url VARCHAR(255);
