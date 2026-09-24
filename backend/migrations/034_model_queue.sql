-- Migration 034: Add model queue concurrency and timeout configurations
INSERT INTO system_configs (key, category, value, description)
VALUES 
  ('ai.max_concurrency', 'ai', '2'::jsonb, '同模型单节点最大并发请求队列容量（防止触发供应商 QPS / 429 限制）'),
  ('ai.queue_timeout_seconds', 'ai', '180'::jsonb, '模型排队最大等待超时时间（秒）')
ON CONFLICT (key) DO NOTHING;
