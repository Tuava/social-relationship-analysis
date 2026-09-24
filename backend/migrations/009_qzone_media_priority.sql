ALTER TABLE media_references
    ADD COLUMN IF NOT EXISTS relation_depth integer NOT NULL DEFAULT 99,
    ADD COLUMN IF NOT EXISTS priority_reason text NOT NULL DEFAULT 'global',
    ADD COLUMN IF NOT EXISTS priority_boost integer NOT NULL DEFAULT 0;

DO $$
BEGIN
    IF NOT EXISTS (
        SELECT 1 FROM pg_constraint
        WHERE conrelid = 'media_references'::regclass
          AND conname = 'media_references_relation_depth_check'
    ) THEN
        ALTER TABLE media_references
            ADD CONSTRAINT media_references_relation_depth_check
            CHECK (relation_depth BETWEEN 0 AND 99);
    END IF;
END $$;

CREATE INDEX IF NOT EXISTS media_references_priority_queue_idx
    ON media_references(
        download_selected DESC,
        relation_depth ASC,
        priority_boost DESC,
        media_kind,
        created_at ASC
    )
    WHERE status IN ('pending', 'failed');

CREATE INDEX IF NOT EXISTS media_references_depth_reason_idx
    ON media_references(relation_depth, priority_reason);
