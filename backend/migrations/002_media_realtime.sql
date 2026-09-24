ALTER TABLE media_assets
    ADD COLUMN IF NOT EXISTS original_filename text NOT NULL DEFAULT '',
    ADD COLUMN IF NOT EXISTS source_account_id uuid REFERENCES napcat_accounts(id) ON DELETE SET NULL,
    ADD COLUMN IF NOT EXISTS source_raw_record_id uuid REFERENCES raw_records(id) ON DELETE SET NULL;

CREATE TABLE IF NOT EXISTS media_references (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    source_account_id uuid NOT NULL REFERENCES napcat_accounts(id) ON DELETE CASCADE,
    message_id uuid REFERENCES messages(id) ON DELETE CASCADE,
    person_id uuid REFERENCES persons(id) ON DELETE CASCADE,
    raw_record_id uuid REFERENCES raw_records(id) ON DELETE SET NULL,
    segment_index integer,
    segment_type text NOT NULL,
    media_kind text NOT NULL,
    source_ref text NOT NULL DEFAULT '',
    source_url text NOT NULL DEFAULT '',
    resolver_endpoint text NOT NULL DEFAULT '',
    original_filename text NOT NULL DEFAULT '',
    metadata jsonb NOT NULL DEFAULT '{}',
    status text NOT NULL DEFAULT 'pending' CHECK (status IN ('pending','downloading','completed','failed')),
    asset_id uuid REFERENCES media_assets(id) ON DELETE SET NULL,
    attempt_count integer NOT NULL DEFAULT 0,
    last_error text,
    next_attempt_at timestamptz NOT NULL DEFAULT now(),
    last_attempt_at timestamptz,
    completed_at timestamptz,
    created_at timestamptz NOT NULL DEFAULT now(),
    CHECK (message_id IS NOT NULL OR person_id IS NOT NULL)
);
CREATE UNIQUE INDEX IF NOT EXISTS media_references_message_segment_idx
    ON media_references(message_id, segment_index, segment_type) WHERE message_id IS NOT NULL;
CREATE UNIQUE INDEX IF NOT EXISTS media_references_person_avatar_idx
    ON media_references(source_account_id, person_id, source_url) WHERE person_id IS NOT NULL AND media_kind='avatar';
CREATE INDEX IF NOT EXISTS media_references_queue_idx
    ON media_references(status, next_attempt_at, created_at);
CREATE INDEX IF NOT EXISTS media_references_message_idx ON media_references(message_id);
CREATE INDEX IF NOT EXISTS media_references_person_idx ON media_references(person_id);

CREATE TABLE IF NOT EXISTS message_media (
    message_id uuid NOT NULL REFERENCES messages(id) ON DELETE CASCADE,
    media_reference_id uuid NOT NULL REFERENCES media_references(id) ON DELETE CASCADE,
    media_asset_id uuid REFERENCES media_assets(id) ON DELETE SET NULL,
    segment_index integer NOT NULL,
    segment_type text NOT NULL,
    metadata jsonb NOT NULL DEFAULT '{}',
    PRIMARY KEY(message_id, media_reference_id)
);
CREATE INDEX IF NOT EXISTS message_media_asset_idx ON message_media(media_asset_id);
CREATE INDEX IF NOT EXISTS raw_events_realtime_idx ON raw_events(created_at DESC, post_type);
CREATE INDEX IF NOT EXISTS source_connections_status_idx ON source_connections(kind, status);
