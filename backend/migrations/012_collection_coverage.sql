-- Collection coverage tracking per person per data source
-- Enables distinguishing "no data exists" from "not yet collected"
CREATE TABLE IF NOT EXISTS collection_coverage (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    person_id uuid NOT NULL REFERENCES persons(id) ON DELETE CASCADE,
    source_account_id uuid REFERENCES napcat_accounts(id) ON DELETE SET NULL,
    data_source text NOT NULL,
    status text NOT NULL DEFAULT 'not_collected',
    items_collected integer NOT NULL DEFAULT 0,
    items_total integer,
    last_collected_at timestamptz,
    last_error text,
    cursor_data jsonb NOT NULL DEFAULT '{}',
    UNIQUE(person_id, source_account_id, data_source)
);

CREATE INDEX IF NOT EXISTS collection_coverage_person_idx ON collection_coverage(person_id, data_source);

-- Valid status values:
-- not_collected: never attempted
-- collecting: task currently running
-- partial: pagination incomplete or interface limited
-- complete: current interface scope fully scanned
-- no_results: completed scan but found nothing
-- failed: cookie, permission, or interface error
DO $$ BEGIN
    IF NOT EXISTS (
        SELECT 1 FROM pg_constraint WHERE conname = 'collection_coverage_status_check'
    ) THEN
        ALTER TABLE collection_coverage ADD CONSTRAINT collection_coverage_status_check
            CHECK (status IN ('not_collected', 'collecting', 'partial', 'complete', 'no_results', 'failed', 'inferred'));
    END IF;
END $$;
