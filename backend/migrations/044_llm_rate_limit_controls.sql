-- Model request pacing is separate from concurrency: providers may reject
-- back-to-back requests even when only one request is active.
INSERT INTO system_configs (key, category, value, description)
VALUES
    ('ai.min_request_interval_ms', 'ai', '3000'::jsonb, '同一模型连续请求的最小间隔（毫秒）；0 表示不主动限速'),
    ('ai.rate_limit_backoff_seconds', 'ai', '45'::jsonb, '收到 429/503 且响应未提供 Retry-After 时的模型冷却时间（秒）')
ON CONFLICT (key) DO NOTHING;
