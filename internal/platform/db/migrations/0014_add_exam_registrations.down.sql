DROP TABLE IF EXISTS exam_registrations;

ALTER TABLE events
    DROP COLUMN IF EXISTS bkash_number,
    DROP COLUMN IF EXISTS nagad_number,
    DROP COLUMN IF EXISTS registration_fee;
