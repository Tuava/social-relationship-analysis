ALTER TABLE collection_run_modules DROP CONSTRAINT IF EXISTS collection_run_modules_status_check;
ALTER TABLE collection_run_modules ADD CONSTRAINT collection_run_modules_status_check
  CHECK (status = ANY (ARRAY['waiting', 'running', 'complete', 'partial', 'failed', 'no_permission', 'no_results', 'retrying', 'cancelled', 'skipped']));

-- Empty visitor responses are scan results with no observations, not complete
-- visitor coverage. Correct historical module rows written by older collectors.
ALTER TABLE collection_run_modules
    DROP CONSTRAINT IF EXISTS collection_run_modules_status_check;

ALTER TABLE collection_run_modules
    ADD CONSTRAINT collection_run_modules_status_check
    CHECK (status IN ('waiting', 'running', 'complete', 'partial', 'failed', 'no_permission', 'retrying', 'cancelled', 'skipped', 'no_results'));
UPDATE collection_run_modules
SET status = 'no_results',
    ended_at = COALESCE(ended_at, updated_at, now()),
    updated_at = now()
WHERE module = 'qzone_visitors'
  AND status = 'complete'
  AND pages_completed = 0
  AND records_collected = 0;
