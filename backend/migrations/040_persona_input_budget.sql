INSERT INTO system_configs (key, category, value, description)
VALUES ('ai.persona_input_char_budget', 'ai', '12000'::jsonb, '单次人物画像输入的历史消息字符预算；在完整时间跨度内均匀取样')
ON CONFLICT (key) DO NOTHING;
