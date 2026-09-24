-- Migration 020 adds non-null timestamp columns with defaults, so PostgreSQL
-- initially fills legacy rows with the migration time. Restore their temporal
-- bounds from the pre-existing profile validity range.
UPDATE person_profiles
SET first_observed_at = valid_from,
    last_observed_at = COALESCE(valid_to, valid_from)
WHERE observation_count = 1;
