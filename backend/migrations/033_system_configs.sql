-- Migration 033: system_configs for dynamic application & AI configuration

CREATE TABLE IF NOT EXISTS system_configs (
    key VARCHAR(64) PRIMARY KEY,
    category VARCHAR(32) NOT NULL,
    value JSONB NOT NULL,
    description TEXT,
    updated_at TIMESTAMP WITH TIME ZONE DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_system_configs_category ON system_configs(category);

-- Preset default system configs
INSERT INTO system_configs (key, category, value, description) VALUES
('ai.llm_provider', 'ai', '"openai_compatible"', 'LLM 服务商类型: openai_compatible / deepseek / gemini / ollama'),
('ai.llm_api_base', 'ai', '"https://api.openai.com/v1"', 'LLM API 基础请求路径'),
('ai.llm_api_key', 'ai', '""', 'LLM API 访问密钥'),
('ai.llm_model', 'ai', '"deepseek-chat"', '全息画像与文本研判默认模型名称'),
('ai.context_depth', 'ai', '150', '单次画像研判加载的历史消息上下文条数 (50-500)'),
('ai.temperature', 'ai', '0.1', '大模型生成发散度 (0.0-1.0，刑侦级研判推荐 0.1)'),
('ai.strict_mode', 'ai', 'true', '反张冠李戴严格模式: 仅采信第一人称自述'),
('ai.custom_prompt', 'ai', '""', '附加系统提示词指令'),

('vision.api_base', 'vision', '""', '多模态视觉 API 基础路径 (空则继承主 LLM)'),
('vision.api_key', 'vision', '""', '多模态视觉 API 访问密钥 (空则继承主 LLM)'),
('vision.model', 'vision', '"gemini-2.5-flash"', '多模态视觉与 OCR 模型名称'),
('vision.auto_parse', 'vision', 'true', '是否自动解析说说配图与关键聊天图片'),
('vision.max_images_per_feed', 'vision', '3', '单条动态最大配图分析数量'),

('qzone.time_window', 'qzone', '"1year"', '空间动态研判时间窗口: 3months / 6months / 1year / all'),
('qzone.tier1_minutes', 'qzone', '5', 'Tier 1 极速互动圈层判定时限 (分钟)'),
('qzone.tier2_min_count', 'qzone', '3', 'Tier 2 深度互动圈层最少互动次数'),

('bot.response_mode', 'bot', '"whitelist_only"', 'QQ 机器人响应范围: whitelist_only / all / admin_only'),
('bot.command_prefix', 'bot', '"/"', '机器人指令前缀'),
('bot.auto_persona_on_join', 'bot', 'false', '新成员入群时是否自动触发全息研判'),

('media.avatar_policy', 'media', '"auto_cache"', '头像存储策略: auto_cache / persist_all / remote_only'),
('media.feed_image_policy', 'media', '"on_demand"', '空间动态配图下载策略: on_demand / all / none'),
('media.max_cache_gb', 'media', '10', '本地媒体缓存最大容量限制 (GB)'),

('crawler.qzone_interval_seconds', 'crawler', '1.5', 'QZone 爬取请求间隔时间 (秒)'),
('crawler.auto_circuit_breaker', 'crawler', 'true', '连续报错 3 次自动熔断暂停采集任务')
ON CONFLICT (key) DO NOTHING;
