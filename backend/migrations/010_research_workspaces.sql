CREATE TABLE IF NOT EXISTS research_workspaces (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id uuid NOT NULL REFERENCES app_users(id) ON DELETE CASCADE,
    name text NOT NULL DEFAULT '',
    state jsonb NOT NULL DEFAULT '{}',
    is_draft boolean NOT NULL DEFAULT false,
    version integer NOT NULL DEFAULT 1,
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now()
);

CREATE UNIQUE INDEX IF NOT EXISTS research_workspaces_user_draft_idx
    ON research_workspaces(user_id) WHERE is_draft;

CREATE INDEX IF NOT EXISTS research_workspaces_user_updated_idx
    ON research_workspaces(user_id, updated_at DESC);
