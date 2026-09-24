INSERT INTO system_configs (key, category, value, description)
VALUES ('ai.persona_thinking_mode', 'ai', '"disabled"'::jsonb, '人物画像请求的模型思考模式；disabled 用于优先保证结构化 JSON 输出')
ON CONFLICT (key) DO NOTHING;
