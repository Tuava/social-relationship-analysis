-- 035_vision_pipeline_settings.sql
-- Add comprehensive configuration options for multimodal vision triggers, targets, and cloud caching

INSERT INTO system_configs (key, category, value, description, updated_at)
VALUES 
  ('vision.scope', 'vision', '"all"', '图片识别数据源范围 (all:全部 / feed_only:仅空间动态 / chat_only:仅群聊配图 / manual_only:仅手动按需)', NOW()),
  ('vision.trigger_mode', 'vision', '"on_demand"', '图片识别触发时机 (on_demand:研判时按需即时识别 / immediate:爬虫抓取后立即识别 / batch_cron:定时批量识别)', NOW()),
  ('vision.auto_parse_feed', 'vision', 'true', '空间动态说说配图是否自动触发多模态识别', NOW()),
  ('vision.auto_parse_chat', 'vision', 'false', '群聊消息图片是否自动触发多模态识别 (建议关闭以避免海量表情包浪费 Token)', NOW()),
  ('vision.chat_filter_mode', 'vision', '"screenshot_and_doc"', '群聊图片过滤策略 (all:全部 / screenshot_and_doc:仅长图与单据截图 / target_users_only:仅重点监控目标)', NOW()),
  ('vision.auto_download_cloud', 'vision', 'true', '遇到远程云端图片 URL 时是否自动预下载到本地高速缓存', NOW()),
  ('vision.max_file_size_mb', 'vision', '10', '单张图片允许下载与识别的最大文件体积 (MB)', NOW()),
  ('vision.max_chat_images_per_group_day', 'vision', '50', '单个群聊每天最大允许自动识别的图片上限', NOW())
ON CONFLICT (key) DO UPDATE SET
  category = EXCLUDED.category,
  description = EXCLUDED.description,
  updated_at = NOW();
