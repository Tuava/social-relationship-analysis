-- Reclassify QQ animation/custom-expression image segments without touching
-- the original message JSON or downloaded object files.
UPDATE media_references
SET media_kind='sticker'
WHERE segment_type='image'
  AND (
    source_url ILIKE '%gxh.vip.qq.com%'
    OR COALESCE(metadata->>'sub_type','') NOT IN ('', '0')
    OR metadata->>'summary' ILIKE '%动画表情%'
    OR metadata->>'summary' ILIKE '%自定义表情%'
  );

UPDATE media_download_policies
SET enabled=false, updated_at=now()
WHERE media_kind='audio';
