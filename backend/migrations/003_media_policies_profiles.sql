CREATE TABLE IF NOT EXISTS media_download_policies (
    media_kind text PRIMARY KEY,
    enabled boolean NOT NULL DEFAULT false,
    priority integer NOT NULL DEFAULT 0,
    updated_at timestamptz NOT NULL DEFAULT now()
);

INSERT INTO media_download_policies(media_kind, enabled, priority) VALUES
    ('avatar', true, 100),
    ('image', false, 50),
    ('sticker', false, 45),
    ('audio', false, 30),
    ('video', false, 20),
    ('file', false, 10)
ON CONFLICT(media_kind) DO NOTHING;

CREATE TABLE IF NOT EXISTS media_download_settings (
    singleton boolean PRIMARY KEY DEFAULT true CHECK(singleton),
    paused boolean NOT NULL DEFAULT false,
    concurrency integer NOT NULL DEFAULT 3 CHECK(concurrency BETWEEN 1 AND 6),
    updated_at timestamptz NOT NULL DEFAULT now()
);
INSERT INTO media_download_settings(singleton) VALUES(true) ON CONFLICT(singleton) DO NOTHING;

ALTER TABLE person_profiles
    ADD COLUMN IF NOT EXISTS profile_data jsonb NOT NULL DEFAULT '{}',
    ADD COLUMN IF NOT EXISTS source_account_id uuid REFERENCES napcat_accounts(id) ON DELETE SET NULL;

ALTER TABLE media_references
    ADD COLUMN IF NOT EXISTS download_selected boolean NOT NULL DEFAULT false;

CREATE INDEX IF NOT EXISTS media_download_policy_queue_idx
    ON media_references(media_kind, status, next_attempt_at, created_at);
