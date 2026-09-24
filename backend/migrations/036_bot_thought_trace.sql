-- Add thought_trace to bot_audit_log for real-time streaming and thinking process visibility.
ALTER TABLE bot_audit_log ADD COLUMN IF NOT EXISTS thought_trace jsonb DEFAULT '[]'::jsonb;
