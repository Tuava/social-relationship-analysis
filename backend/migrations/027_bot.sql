-- QQ bot: query commands + cat persona entertainment.
-- One instance per NapCat account; outgoing messages are whitelist-guarded
-- and every reply is written to bot_audit_log.
CREATE TABLE IF NOT EXISTS bot_instances (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    account_id uuid NOT NULL REFERENCES napcat_accounts(id) ON DELETE CASCADE,
    enabled boolean NOT NULL DEFAULT false,
    name text NOT NULL DEFAULT '老吴猫',
    persona text NOT NULL DEFAULT '你是一只住在 QQ 群里的小猫咪，名字叫"老吴"。你说话简短、俏皮、粘人，喜欢在句尾加"老吴~"或"喵"。回答要可爱但别啰嗦。',
    meow_sound text NOT NULL DEFAULT '老吴~',
    command_prefix text NOT NULL DEFAULT '/',
    greeting text NOT NULL DEFAULT '喵~ 老吴来啦，有事喊我喵！',
    reply_probability integer NOT NULL DEFAULT 15,
    entertainment jsonb NOT NULL DEFAULT '["老吴~", "喵喵喵？", "抓你裤脚！", "老吴老吴，今天也要开心喵~", "谁在叫我？老吴在！"]',
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now()
);
CREATE UNIQUE INDEX IF NOT EXISTS bot_instances_account_idx ON bot_instances(account_id);

CREATE TABLE IF NOT EXISTS bot_group_whitelist (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    bot_id uuid NOT NULL REFERENCES bot_instances(id) ON DELETE CASCADE,
    platform_group_id text NOT NULL,
    enabled boolean NOT NULL DEFAULT true,
    created_at timestamptz NOT NULL DEFAULT now(),
    UNIQUE(bot_id, platform_group_id)
);

CREATE TABLE IF NOT EXISTS bot_audit_log (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    bot_id uuid REFERENCES bot_instances(id) ON DELETE SET NULL,
    account_id uuid REFERENCES napcat_accounts(id) ON DELETE SET NULL,
    platform_group_id text NOT NULL DEFAULT '',
    user_qq text NOT NULL DEFAULT '',
    trigger_type text NOT NULL DEFAULT '',
    command text NOT NULL DEFAULT '',
    reply text NOT NULL DEFAULT '',
    message_id text NOT NULL DEFAULT '',
    created_at timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX IF NOT EXISTS bot_audit_log_bot_created_idx ON bot_audit_log(bot_id, created_at DESC);
CREATE INDEX IF NOT EXISTS bot_audit_log_account_created_idx ON bot_audit_log(account_id, created_at DESC);
