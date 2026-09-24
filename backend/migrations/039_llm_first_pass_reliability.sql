-- Runtime controls for predictable first-pass structured analysis.
INSERT INTO system_configs (key, category, value, description)
VALUES
    ('ai.llm_max_tokens', 'ai', '4096'::jsonb, '单次模型响应最大输出 token 数'),
    ('ai.llm_request_timeout_seconds', 'ai', '120'::jsonb, '单次模型 HTTP 请求超时时间（秒）'),
    ('ai.llm_internal_attempts', 'ai', '1'::jsonb, '单次分析内部 HTTP 尝试次数；批量重试由流水线统一调度')
ON CONFLICT (key) DO NOTHING;
