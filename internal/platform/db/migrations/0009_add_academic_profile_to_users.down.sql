ALTER TABLE users
    DROP COLUMN IF EXISTS institution_name,
    DROP COLUMN IF EXISTS level,
    DROP COLUMN IF EXISTS medium;
