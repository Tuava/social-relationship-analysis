ALTER TABLE collection_runs
    ADD COLUMN IF NOT EXISTS config jsonb NOT NULL DEFAULT '{}';

CREATE TABLE IF NOT EXISTS collection_scope_rules (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    run_id uuid NOT NULL REFERENCES collection_runs(id) ON DELETE CASCADE,
    rule_type text NOT NULL CHECK (rule_type IN ('entry', 'exclude')),
    target_type text NOT NULL CHECK (target_type IN ('qq', 'group', 'post', 'conversation')),
    target_id text NOT NULL,
    mode text NOT NULL DEFAULT 'record_only',
    metadata jsonb NOT NULL DEFAULT '{}',
    created_at timestamptz NOT NULL DEFAULT now(),
    UNIQUE(run_id, rule_type, target_type, target_id)
);

CREATE INDEX IF NOT EXISTS collection_scope_rules_run_idx
    ON collection_scope_rules(run_id, rule_type, target_type);

CREATE TABLE IF NOT EXISTS collection_run_modules (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    run_id uuid NOT NULL REFERENCES collection_runs(id) ON DELETE CASCADE,
    module text NOT NULL,
    status text NOT NULL DEFAULT 'waiting'
        CHECK (status IN ('waiting', 'running', 'complete', 'partial', 'failed', 'no_permission', 'retrying', 'cancelled')),
    pages_completed integer NOT NULL DEFAULT 0,
    pages_total integer,
    records_collected bigint NOT NULL DEFAULT 0,
    cursor jsonb NOT NULL DEFAULT '{}',
    error text,
    started_at timestamptz,
    updated_at timestamptz NOT NULL DEFAULT now(),
    ended_at timestamptz,
    UNIQUE(run_id, module)
);

CREATE INDEX IF NOT EXISTS collection_run_modules_run_idx
    ON collection_run_modules(run_id, updated_at DESC);

CREATE TABLE IF NOT EXISTS collection_candidates (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    run_id uuid NOT NULL REFERENCES collection_runs(id) ON DELETE CASCADE,
    account_id uuid REFERENCES napcat_accounts(id) ON DELETE SET NULL,
    entity_type text NOT NULL CHECK (entity_type IN ('qq', 'group', 'post', 'conversation')),
    entity_id text NOT NULL,
    state text NOT NULL DEFAULT 'discovered'
        CHECK (state IN ('discovered', 'collected', 'expandable', 'expanded', 'excluded')),
    depth integer NOT NULL DEFAULT 0 CHECK (depth >= 0),
    priority numeric NOT NULL DEFAULT 0,
    selected boolean NOT NULL DEFAULT false,
    discovery_count integer NOT NULL DEFAULT 1,
    contexts text[] NOT NULL DEFAULT '{}',
    discovery_paths jsonb NOT NULL DEFAULT '[]',
    first_discovered_at timestamptz NOT NULL DEFAULT now(),
    last_discovered_at timestamptz NOT NULL DEFAULT now(),
    expanded_at timestamptz,
    UNIQUE(run_id, entity_type, entity_id)
);

CREATE INDEX IF NOT EXISTS collection_candidates_queue_idx
    ON collection_candidates(run_id, state, selected DESC, priority DESC, depth, last_discovered_at);
