-- Media recognition stub: reserved columns for OCR / ASR results.
-- Phase 1 only adds the fields and MCP output plumbing; no recognition
-- model is wired up yet. Null values mean "not processed".
ALTER TABLE media_assets
    ADD COLUMN IF NOT EXISTS ocr_text text,
    ADD COLUMN IF NOT EXISTS transcript_text text,
    ADD COLUMN IF NOT EXISTS processed_at timestamptz;

CREATE INDEX IF NOT EXISTS media_assets_processed_idx
    ON media_assets(processed_at)
    WHERE processed_at IS NOT NULL;
