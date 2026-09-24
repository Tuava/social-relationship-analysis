WITH ranked AS (
    SELECT id,
           first_value(id) OVER (PARTITION BY target_object_id ORDER BY occurred_at, id) AS keep_id,
           evidence_ids,
           raw_record_id
    FROM relation_events
    WHERE action_type = 'published'
      AND context_type = 'qzone'
      AND target_object_id IS NOT NULL
), expanded AS (
    SELECT keep_id, unnest(evidence_ids || CASE WHEN raw_record_id IS NULL THEN '{}'::uuid[] ELSE ARRAY[raw_record_id] END) AS evidence_id
    FROM ranked
), merged AS (
    SELECT keep_id, array_agg(DISTINCT evidence_id) AS evidence_ids
    FROM expanded
    GROUP BY keep_id
)
UPDATE relation_events event
SET evidence_ids = merged.evidence_ids
FROM merged
WHERE event.id = merged.keep_id;

WITH ranked AS (
    SELECT id,
           row_number() OVER (PARTITION BY target_object_id ORDER BY occurred_at, id) AS position
    FROM relation_events
    WHERE action_type = 'published'
      AND context_type = 'qzone'
      AND target_object_id IS NOT NULL
)
DELETE FROM relation_events event
USING ranked
WHERE event.id = ranked.id
  AND ranked.position > 1;

CREATE UNIQUE INDEX IF NOT EXISTS relation_events_qzone_published_content_unique
    ON relation_events(target_object_id)
    WHERE action_type = 'published'
      AND context_type = 'qzone'
      AND target_object_id IS NOT NULL;
