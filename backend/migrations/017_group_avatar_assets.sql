ALTER TABLE media_references
    ADD COLUMN IF NOT EXISTS group_id uuid REFERENCES groups(id) ON DELETE CASCADE;

ALTER TABLE media_references
    DROP CONSTRAINT IF EXISTS media_references_owner_check;

ALTER TABLE media_references
    ADD CONSTRAINT media_references_owner_check
    CHECK (message_id IS NOT NULL OR person_id IS NOT NULL OR content_id IS NOT NULL OR group_id IS NOT NULL);

CREATE UNIQUE INDEX IF NOT EXISTS media_references_group_avatar_idx
    ON media_references(source_account_id, group_id, source_url)
    WHERE group_id IS NOT NULL AND media_kind='avatar';

CREATE INDEX IF NOT EXISTS media_references_group_idx
    ON media_references(group_id);
