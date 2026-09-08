ALTER TABLE exam_registrations
    DROP COLUMN IF EXISTS admit_card_url,
    DROP COLUMN IF EXISTS admit_card_uploaded_at;
