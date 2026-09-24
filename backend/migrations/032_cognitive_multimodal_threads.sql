-- Migration 032: Multimodal vision, perceptual hashing, and dialogue threads

-- 1. Enhance media_assets for multimodal analysis
ALTER TABLE media_assets
    ADD COLUMN IF NOT EXISTS phash text,
    ADD COLUMN IF NOT EXISTS visual_tags jsonb NOT NULL DEFAULT '[]'::jsonb,
    ADD COLUMN IF NOT EXISTS ocr_status text NOT NULL DEFAULT 'pending';

CREATE INDEX IF NOT EXISTS media_assets_phash_idx
    ON media_assets(phash)
    WHERE phash IS NOT NULL AND phash != '';

CREATE INDEX IF NOT EXISTS media_assets_ocr_status_idx
    ON media_assets(ocr_status);

-- GIN index on ocr_text for fast full-text searching
CREATE INDEX IF NOT EXISTS media_assets_ocr_text_gin_idx
    ON media_assets USING gin(to_tsvector('simple', COALESCE(ocr_text, '')));

-- 2. Dialogue threads for multiparty chat disentanglement
CREATE TABLE IF NOT EXISTS dialogue_threads (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    conversation_id uuid NOT NULL REFERENCES conversations(id) ON DELETE CASCADE,
    title text NOT NULL DEFAULT '',
    topic_category text NOT NULL DEFAULT 'general',
    stance text NOT NULL DEFAULT 'casual',
    summary text NOT NULL DEFAULT '',
    participant_count integer NOT NULL DEFAULT 0,
    message_count integer NOT NULL DEFAULT 0,
    started_at timestamptz NOT NULL,
    ended_at timestamptz NOT NULL,
    key_entities jsonb NOT NULL DEFAULT '[]'::jsonb,
    created_at timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS dialogue_threads_conversation_idx
    ON dialogue_threads(conversation_id, started_at DESC);

CREATE TABLE IF NOT EXISTS thread_messages (
    thread_id uuid NOT NULL REFERENCES dialogue_threads(id) ON DELETE CASCADE,
    message_id uuid NOT NULL REFERENCES messages(id) ON DELETE CASCADE,
    sequence_index integer NOT NULL DEFAULT 0,
    reply_to_message_id uuid REFERENCES messages(id) ON DELETE SET NULL,
    link_confidence numeric NOT NULL DEFAULT 1.0,
    PRIMARY KEY (thread_id, message_id)
);

CREATE INDEX IF NOT EXISTS thread_messages_message_idx
    ON thread_messages(message_id);
