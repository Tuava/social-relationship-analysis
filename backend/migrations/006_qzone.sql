CREATE TABLE IF NOT EXISTS qzone_connections (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    account_id uuid NOT NULL UNIQUE REFERENCES napcat_accounts(id) ON DELETE CASCADE,
    http_url text NOT NULL DEFAULT 'http://127.0.0.1:5700',
    ws_url text NOT NULL DEFAULT 'ws://127.0.0.1:5700/event',
    access_token text NOT NULL DEFAULT '',
    enabled boolean NOT NULL DEFAULT true,
    status text NOT NULL DEFAULT 'disconnected',
    last_connected_at timestamptz,
    last_event_at timestamptz,
    last_error text,
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now()
);

ALTER TABLE contents
    ADD COLUMN IF NOT EXISTS source_account_id uuid REFERENCES napcat_accounts(id) ON DELETE SET NULL,
    ADD COLUMN IF NOT EXISTS parent_content_id uuid REFERENCES contents(id) ON DELETE CASCADE,
    ADD COLUMN IF NOT EXISTS metadata jsonb NOT NULL DEFAULT '{}',
    ADD COLUMN IF NOT EXISTS updated_at timestamptz NOT NULL DEFAULT now();

CREATE INDEX IF NOT EXISTS contents_author_published_idx ON contents(author_id, published_at DESC);
CREATE INDEX IF NOT EXISTS contents_parent_idx ON contents(parent_content_id);
CREATE INDEX IF NOT EXISTS contents_metadata_idx ON contents USING gin(metadata);

ALTER TABLE media_references ADD COLUMN IF NOT EXISTS content_id uuid REFERENCES contents(id) ON DELETE CASCADE;

DO $$
DECLARE constraint_name text;
BEGIN
    SELECT conname INTO constraint_name
    FROM pg_constraint
    WHERE conrelid='media_references'::regclass
      AND contype='c'
      AND pg_get_constraintdef(oid) ILIKE '%message_id%person_id%';
    IF constraint_name IS NOT NULL THEN
        EXECUTE format('ALTER TABLE media_references DROP CONSTRAINT %I', constraint_name);
    END IF;
END $$;

ALTER TABLE media_references
    ADD CONSTRAINT media_references_owner_check
    CHECK (message_id IS NOT NULL OR person_id IS NOT NULL OR content_id IS NOT NULL);

CREATE UNIQUE INDEX IF NOT EXISTS media_references_content_segment_idx
    ON media_references(content_id, segment_index, segment_type) WHERE content_id IS NOT NULL;
CREATE INDEX IF NOT EXISTS media_references_content_idx ON media_references(content_id);

CREATE TABLE IF NOT EXISTS content_media (
    content_id uuid NOT NULL REFERENCES contents(id) ON DELETE CASCADE,
    media_reference_id uuid NOT NULL REFERENCES media_references(id) ON DELETE CASCADE,
    media_asset_id uuid REFERENCES media_assets(id) ON DELETE SET NULL,
    position integer NOT NULL DEFAULT 0,
    media_kind text NOT NULL,
    metadata jsonb NOT NULL DEFAULT '{}',
    PRIMARY KEY(content_id, media_reference_id)
);
CREATE INDEX IF NOT EXISTS content_media_asset_idx ON content_media(media_asset_id);
