DELETE FROM relation_events
WHERE id IN (
    SELECT id
    FROM (
        SELECT id, row_number() OVER (
            PARTITION BY raw_record_id,action_type,actor_person_id,target_person_id,target_object_id
            ORDER BY id
        ) AS duplicate_number
        FROM relation_events
    ) ranked
    WHERE duplicate_number > 1
);

CREATE UNIQUE INDEX IF NOT EXISTS relation_events_evidence_endpoint_idx
    ON relation_events(
        raw_record_id,
        action_type,
        COALESCE(actor_person_id, '00000000-0000-0000-0000-000000000000'::uuid),
        COALESCE(target_person_id, '00000000-0000-0000-0000-000000000000'::uuid),
        COALESCE(target_object_id, '00000000-0000-0000-0000-000000000000'::uuid)
    )
    WHERE raw_record_id IS NOT NULL;
