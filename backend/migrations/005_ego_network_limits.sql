ALTER TABLE ego_networks
    ADD COLUMN IF NOT EXISTS truncated boolean NOT NULL DEFAULT false,
    ADD COLUMN IF NOT EXISTS processed_event_count integer NOT NULL DEFAULT 0;

ALTER TABLE ego_network_edges
    ADD COLUMN IF NOT EXISTS event_count integer NOT NULL DEFAULT 1,
    ADD COLUMN IF NOT EXISTS first_seen timestamptz,
    ADD COLUMN IF NOT EXISTS last_seen timestamptz;

UPDATE ego_network_edges
SET event_count = GREATEST(event_count, weight::integer)
WHERE event_count IS NULL OR event_count < weight::integer;
