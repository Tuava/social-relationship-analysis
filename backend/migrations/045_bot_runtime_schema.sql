-- Bring fresh installations up to the schema used by bot runtime and API.
CREATE TABLE IF NOT EXISTS bot_user_whitelist (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    bot_id uuid NOT NULL REFERENCES bot_instances(id) ON DELETE CASCADE,
    user_qq text NOT NULL,
    enabled boolean NOT NULL DEFAULT true,
    created_at timestamptz NOT NULL DEFAULT now(),
    UNIQUE(bot_id, user_qq)
);
ALTER TABLE bot_audit_log
    ADD COLUMN IF NOT EXISTS status text NOT NULL DEFAULT 'completed',
    ADD COLUMN IF NOT EXISTS tools_called text[] NOT NULL DEFAULT '{}',
    ADD COLUMN IF NOT EXISTS latency_ms integer NOT NULL DEFAULT 0,
    ADD COLUMN IF NOT EXISTS error_message text NOT NULL DEFAULT '';
