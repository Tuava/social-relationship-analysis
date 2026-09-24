-- Backfill bidirectional friend_visible relations between collector accounts and their QZone feed authors
INSERT INTO relation_events (
    source_account_id,
    actor_person_id,
    target_person_id,
    action_type,
    context_type,
    occurred_at,
    evidence_ids,
    raw_record_id
)
SELECT DISTINCT
    na.id as source_account_id,
    self.id as actor_person_id,
    c.author_id as target_person_id,
    'friend_visible' as action_type,
    'qzone_friend' as context_type,
    COALESCE(c.published_at, now()) as occurred_at,
    ARRAY[COALESCE(c.raw_record_id, '00000000-0000-0000-0000-000000000000'::uuid)] as evidence_ids,
    c.raw_record_id
FROM contents c
JOIN napcat_accounts na ON na.id = c.source_account_id
JOIN persons self ON true
JOIN person_identifiers si ON si.person_id = self.id AND si.platform = 'qq' AND si.platform_user_id = na.qq_uin
WHERE c.platform = 'qzone' 
  AND c.author_id IS NOT NULL 
  AND c.author_id <> self.id
ON CONFLICT DO NOTHING;

INSERT INTO relation_events (
    source_account_id,
    actor_person_id,
    target_person_id,
    action_type,
    context_type,
    occurred_at,
    evidence_ids,
    raw_record_id
)
SELECT DISTINCT
    na.id as source_account_id,
    c.author_id as actor_person_id,
    self.id as target_person_id,
    'friend_visible' as action_type,
    'qzone_friend' as context_type,
    COALESCE(c.published_at, now()) as occurred_at,
    ARRAY[COALESCE(c.raw_record_id, '00000000-0000-0000-0000-000000000000'::uuid)] as evidence_ids,
    c.raw_record_id
FROM contents c
JOIN napcat_accounts na ON na.id = c.source_account_id
JOIN persons self ON true
JOIN person_identifiers si ON si.person_id = self.id AND si.platform = 'qq' AND si.platform_user_id = na.qq_uin
WHERE c.platform = 'qzone' 
  AND c.author_id IS NOT NULL 
  AND c.author_id <> self.id
ON CONFLICT DO NOTHING;
