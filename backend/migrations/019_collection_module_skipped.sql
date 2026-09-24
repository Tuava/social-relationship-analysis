ALTER TABLE collection_run_modules
    DROP CONSTRAINT IF EXISTS collection_run_modules_status_check;

ALTER TABLE collection_run_modules
    ADD CONSTRAINT collection_run_modules_status_check
    CHECK (status IN ('waiting', 'running', 'complete', 'partial', 'failed', 'no_permission', 'retrying', 'cancelled', 'skipped'));
