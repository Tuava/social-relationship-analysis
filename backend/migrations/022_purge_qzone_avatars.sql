-- 022_purge_qzone_avatars.sql: Clean all QZone anti-hotlink avatar URLs and ensure clean QQ avatars

-- 1. Reset QZone avatar URLs in person_profiles
UPDATE person_profiles 
SET avatar_uri = ''
WHERE avatar_uri LIKE '%store.qq.com%' 
   OR avatar_uri LIKE '%qzone%' 
   OR avatar_uri LIKE '%qpic.cn%'
   OR avatar_uri LIKE '%figureurl%';

-- 2. Delete media references pointing to QZone anti-hotlink URLs
DELETE FROM media_references 
WHERE media_kind = 'avatar' 
  AND (source_url LIKE '%store.qq.com%' 
    OR source_url LIKE '%qzone%' 
    OR source_url LIKE '%qpic.cn%'
    OR source_url LIKE '%figureurl%');

-- 3. Queue standard official high-res avatars for all existing QQ persons who do not have a completed avatar reference
INSERT INTO media_references(source_account_id, person_id, segment_type, media_kind, source_url, metadata, relation_depth, priority_reason)
SELECT 
    (SELECT id FROM napcat_accounts ORDER BY enabled DESC, created_at DESC LIMIT 1),
    p.id,
    'avatar',
    'avatar',
    'https://q1.qlogo.cn/g?b=qq&nk=' || pi.platform_user_id || '&s=640',
    jsonb_build_object('qq', pi.platform_user_id),
    0,
    'avatar'
FROM persons p
JOIN person_identifiers pi ON pi.person_id = p.id AND pi.platform = 'qq'
WHERE pi.platform_user_id <> ''
  AND NOT EXISTS (
      SELECT 1 FROM media_references mr 
      WHERE mr.person_id = p.id AND mr.media_kind = 'avatar' AND mr.status = 'completed'
  )
ON CONFLICT (source_account_id, person_id, source_url) WHERE person_id IS NOT NULL AND media_kind = 'avatar'
DO NOTHING;
