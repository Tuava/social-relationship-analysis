-- 025_messages_is_recalled.sql
ALTER TABLE messages ADD COLUMN IF NOT EXISTS is_recalled boolean NOT NULL DEFAULT false;
CREATE INDEX IF NOT EXISTS messages_is_recalled_idx ON messages(conversation_id, is_recalled) WHERE is_recalled = true;
