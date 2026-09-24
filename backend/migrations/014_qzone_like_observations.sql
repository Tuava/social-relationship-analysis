-- A like list is a snapshot, not a stream of repeated events. Keep one
-- relationship edge per account/person/post while retaining every raw
-- observation as evidence_ids.
WITH ranked AS (
    SELECT id,
           first_value(id) OVER (
               PARTITION BY source_account_id, actor_person_id, target_object_id, action_type, context_type
               ORDER BY occurred_at, id
           ) AS keep_id,
           unnest(evidence_ids || CASE WHEN raw_record_id IS NULL THEN '{}'::uuid[] ELSE ARRAY[raw_record_id] END) AS evidence_id
    FROM relation_events
    WHERE action_type='liked' AND target_object_id IS NOT NULL
), merged AS (
    SELECT keep_id, array_agg(DISTINCT evidence_id) AS evidence_ids
    FROM ranked
    GROUP BY keep_id
)
UPDATE relation_events event
SET evidence_ids=merged.evidence_ids,
    raw_record_id=event.raw_record_id
FROM merged
WHERE event.id=merged.keep_id;

WITH ranked AS (
    SELECT id,
           row_number() OVER (
               PARTITION BY source_account_id, actor_person_id, target_object_id, action_type, context_type
               ORDER BY occurred_at, id
           ) AS position
    FROM relation_events
    WHERE action_type='liked' AND target_object_id IS NOT NULL
)
DELETE FROM relation_events event
USING ranked
WHERE event.id=ranked.id AND ranked.position>1;

CREATE UNIQUE INDEX IF NOT EXISTS relation_events_qzone_like_unique
    ON relation_events(source_account_id, actor_person_id, target_object_id, action_type, context_type)
    WHERE action_type='liked' AND target_object_id IS NOT NULL;
