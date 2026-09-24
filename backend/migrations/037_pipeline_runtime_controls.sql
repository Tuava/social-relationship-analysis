ALTER TABLE batch_persona_pipelines
    ADD COLUMN IF NOT EXISTS retry_backoff_seconds INT NOT NULL DEFAULT 15,
    ADD COLUMN IF NOT EXISTS queue_wait_seconds INT NOT NULL DEFAULT 180;
