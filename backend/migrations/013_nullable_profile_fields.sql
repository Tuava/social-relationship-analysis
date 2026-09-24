-- Make profile fields nullable - QZone/NapCat data often has missing fields
ALTER TABLE person_profiles ALTER COLUMN sex DROP NOT NULL;
ALTER TABLE person_profiles ALTER COLUMN area DROP NOT NULL;
ALTER TABLE person_profiles ALTER COLUMN signature DROP NOT NULL;
