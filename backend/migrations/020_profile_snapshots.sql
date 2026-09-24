-- Profile rows are temporal snapshots. Repeated observations are retained in
-- profile_observations without creating another identical visible version.
ALTER TABLE person_profiles
    ADD COLUMN IF NOT EXISTS snapshot_hash text NOT NULL DEFAULT '',
    ADD COLUMN IF NOT EXISTS version_number integer NOT NULL DEFAULT 1,
    ADD COLUMN IF NOT EXISTS observation_count bigint NOT NULL DEFAULT 1,
    ADD COLUMN IF NOT EXISTS first_observed_at timestamptz NOT NULL DEFAULT now(),
    ADD COLUMN IF NOT EXISTS last_observed_at timestamptz NOT NULL DEFAULT now();

UPDATE person_profiles
SET first_observed_at = valid_from,
    last_observed_at = COALESCE(valid_to, valid_from),
    snapshot_hash = CASE WHEN snapshot_hash <> '' THEN snapshot_hash ELSE encode(digest(
        jsonb_build_object(
            'nickname', nickname,
            'avatar_uri', avatar_uri,
            'card_or_remark', card_or_remark,
            'profile_data', profile_data,
            'sex', sex,
            'age', age,
            'area', area,
            'signature', signature,
            'reg_time', reg_time,
            'login_days', login_days
        )::text, 'sha256'), 'hex') END;

WITH numbered AS (
    SELECT id, row_number() OVER (
        PARTITION BY person_id, source, COALESCE(source_account_id, '00000000-0000-0000-0000-000000000000'::uuid)
        ORDER BY valid_from, id
    ) AS version_number
    FROM person_profiles
)
UPDATE person_profiles p
SET version_number = numbered.version_number
FROM numbered
WHERE p.id = numbered.id;

-- Keep old duplicate current rows as historical rows instead of deleting them.
WITH ranked AS (
    SELECT id, row_number() OVER (
        PARTITION BY person_id, source, COALESCE(source_account_id, '00000000-0000-0000-0000-000000000000'::uuid)
        ORDER BY valid_from DESC, id DESC
    ) AS position
    FROM person_profiles
    WHERE valid_to IS NULL
)
UPDATE person_profiles p
SET valid_to = COALESCE(p.valid_to, now())
FROM ranked r
WHERE p.id = r.id AND r.position > 1;

CREATE UNIQUE INDEX IF NOT EXISTS person_profiles_current_stream_idx
    ON person_profiles(
        person_id,
        source,
        COALESCE(source_account_id, '00000000-0000-0000-0000-000000000000'::uuid)
    ) WHERE valid_to IS NULL;

CREATE TABLE IF NOT EXISTS profile_observations (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    profile_id uuid NOT NULL REFERENCES person_profiles(id) ON DELETE CASCADE,
    person_id uuid NOT NULL REFERENCES persons(id) ON DELETE CASCADE,
    source text NOT NULL,
    source_account_id uuid REFERENCES napcat_accounts(id) ON DELETE SET NULL,
    raw_record_id uuid REFERENCES raw_records(id) ON DELETE SET NULL,
    snapshot_hash text NOT NULL,
    payload jsonb NOT NULL DEFAULT '{}',
    observed_at timestamptz NOT NULL DEFAULT now(),
    UNIQUE(profile_id, raw_record_id)
);

CREATE INDEX IF NOT EXISTS profile_observations_person_time_idx
    ON profile_observations(person_id, observed_at DESC);
CREATE INDEX IF NOT EXISTS profile_observations_hash_idx
    ON profile_observations(snapshot_hash);
