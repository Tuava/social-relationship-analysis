CREATE EXTENSION IF NOT EXISTS pgcrypto;
CREATE EXTENSION IF NOT EXISTS pg_trgm;

CREATE TABLE IF NOT EXISTS app_users (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    username text NOT NULL UNIQUE,
    password_hash text NOT NULL,
    created_at timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE IF NOT EXISTS napcat_accounts (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(), name text NOT NULL,
    qq_uin text NOT NULL DEFAULT '', ws_url text NOT NULL, ws_token text NOT NULL DEFAULT '',
    http_url text NOT NULL DEFAULT '', http_token text NOT NULL DEFAULT '', enabled boolean NOT NULL DEFAULT true,
    status text NOT NULL DEFAULT 'disconnected', last_connected_at timestamptz, created_at timestamptz NOT NULL DEFAULT now()
);
CREATE TABLE IF NOT EXISTS source_connections (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(), account_id uuid NOT NULL REFERENCES napcat_accounts(id) ON DELETE CASCADE,
    kind text NOT NULL, status text NOT NULL DEFAULT 'disconnected', last_event_at timestamptz, error text,
    UNIQUE(account_id, kind)
);
CREATE TABLE IF NOT EXISTS collection_runs (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(), account_id uuid REFERENCES napcat_accounts(id) ON DELETE SET NULL,
    type text NOT NULL, status text NOT NULL DEFAULT 'queued', progress integer NOT NULL DEFAULT 0 CHECK(progress BETWEEN 0 AND 100),
    error text, started_at timestamptz, ended_at timestamptz, created_at timestamptz NOT NULL DEFAULT now()
);
CREATE TABLE IF NOT EXISTS collection_cursors (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(), account_id uuid NOT NULL REFERENCES napcat_accounts(id) ON DELETE CASCADE,
    scope text NOT NULL, cursor jsonb NOT NULL DEFAULT '{}', updated_at timestamptz NOT NULL DEFAULT now(),
    UNIQUE(account_id, scope)
);

CREATE TABLE IF NOT EXISTS raw_records (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(), account_id uuid REFERENCES napcat_accounts(id) ON DELETE SET NULL,
    source text NOT NULL, endpoint_or_event_type text NOT NULL, payload jsonb NOT NULL, payload_hash text NOT NULL,
    collected_at timestamptz NOT NULL DEFAULT now(), UNIQUE(account_id, source, endpoint_or_event_type, payload_hash)
);
CREATE INDEX IF NOT EXISTS raw_records_collected_idx ON raw_records(collected_at DESC);
CREATE INDEX IF NOT EXISTS raw_records_payload_idx ON raw_records USING gin(payload);
CREATE TABLE IF NOT EXISTS raw_events (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(), raw_record_id uuid NOT NULL REFERENCES raw_records(id) ON DELETE CASCADE,
    event_id text, post_type text, message_type text, occurred_at timestamptz, sequence bigint, created_at timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE IF NOT EXISTS media_assets (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(), sha256 text NOT NULL UNIQUE, mime_type text NOT NULL DEFAULT 'application/octet-stream',
    size bigint NOT NULL DEFAULT 0, object_path text NOT NULL, source_url text, source_message_id text,
    downloaded_at timestamptz, created_at timestamptz NOT NULL DEFAULT now()
);
CREATE TABLE IF NOT EXISTS persons (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(), display_name text NOT NULL DEFAULT '', first_seen_at timestamptz NOT NULL DEFAULT now(), last_seen_at timestamptz NOT NULL DEFAULT now()
);
CREATE TABLE IF NOT EXISTS person_identifiers (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(), person_id uuid NOT NULL REFERENCES persons(id) ON DELETE CASCADE,
    platform text NOT NULL, platform_user_id text NOT NULL, source_account_id uuid REFERENCES napcat_accounts(id) ON DELETE SET NULL,
    confidence numeric NOT NULL DEFAULT 1, UNIQUE(platform, platform_user_id)
);
CREATE TABLE IF NOT EXISTS person_profiles (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(), person_id uuid NOT NULL REFERENCES persons(id) ON DELETE CASCADE,
    nickname text NOT NULL DEFAULT '', avatar_uri text NOT NULL DEFAULT '', card_or_remark text NOT NULL DEFAULT '',
    valid_from timestamptz NOT NULL DEFAULT now(), valid_to timestamptz, source text NOT NULL,
    raw_record_id uuid REFERENCES raw_records(id) ON DELETE SET NULL
);
CREATE INDEX IF NOT EXISTS person_profiles_nickname_idx ON person_profiles USING gin(nickname gin_trgm_ops);

CREATE TABLE IF NOT EXISTS groups (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(), platform text NOT NULL DEFAULT 'qq', platform_group_id text NOT NULL,
    group_name text NOT NULL DEFAULT '', first_seen_at timestamptz NOT NULL DEFAULT now(), UNIQUE(platform, platform_group_id)
);
CREATE TABLE IF NOT EXISTS group_memberships (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(), group_id uuid NOT NULL REFERENCES groups(id) ON DELETE CASCADE,
    person_id uuid NOT NULL REFERENCES persons(id) ON DELETE CASCADE, source_account_id uuid REFERENCES napcat_accounts(id) ON DELETE SET NULL,
    role text NOT NULL DEFAULT '', valid_from timestamptz NOT NULL DEFAULT now(), valid_to timestamptz,
    raw_record_id uuid REFERENCES raw_records(id) ON DELETE SET NULL, UNIQUE(group_id, person_id, source_account_id)
);
CREATE TABLE IF NOT EXISTS conversations (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(), conversation_type text NOT NULL, platform_conversation_id text NOT NULL,
    name text NOT NULL DEFAULT '', UNIQUE(conversation_type, platform_conversation_id)
);
CREATE TABLE IF NOT EXISTS contents (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(), platform text NOT NULL, platform_content_id text NOT NULL,
    author_id uuid REFERENCES persons(id) ON DELETE SET NULL, context_type text NOT NULL, body text NOT NULL DEFAULT '',
    published_at timestamptz, raw_record_id uuid REFERENCES raw_records(id) ON DELETE SET NULL, UNIQUE(platform, platform_content_id)
);
CREATE TABLE IF NOT EXISTS messages (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(), source_account_id uuid REFERENCES napcat_accounts(id) ON DELETE SET NULL,
    source_message_id text, conversation_id uuid REFERENCES conversations(id) ON DELETE SET NULL, sender_id uuid REFERENCES persons(id) ON DELETE SET NULL,
    sent_at timestamptz, raw_text text NOT NULL DEFAULT '', message_segments jsonb NOT NULL DEFAULT '[]', reply_to_message_id text,
    raw_record_id uuid REFERENCES raw_records(id) ON DELETE SET NULL, created_at timestamptz NOT NULL DEFAULT now(), UNIQUE(source_account_id, source_message_id)
);
CREATE INDEX IF NOT EXISTS messages_text_idx ON messages USING gin(to_tsvector('simple', raw_text));
CREATE INDEX IF NOT EXISTS messages_sent_idx ON messages(sent_at DESC);

CREATE TABLE IF NOT EXISTS relation_events (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(), source_account_id uuid REFERENCES napcat_accounts(id) ON DELETE SET NULL,
    actor_person_id uuid REFERENCES persons(id) ON DELETE SET NULL, target_person_id uuid REFERENCES persons(id) ON DELETE SET NULL,
    target_object_id uuid, action_type text NOT NULL, context_type text NOT NULL, occurred_at timestamptz NOT NULL,
    evidence_ids uuid[] NOT NULL DEFAULT '{}', raw_record_id uuid REFERENCES raw_records(id) ON DELETE SET NULL,
    UNIQUE(raw_record_id, action_type, actor_person_id, target_person_id)
);
CREATE INDEX IF NOT EXISTS relation_events_actor_idx ON relation_events(actor_person_id, occurred_at DESC);
CREATE INDEX IF NOT EXISTS relation_events_target_idx ON relation_events(target_person_id, occurred_at DESC);

CREATE TABLE IF NOT EXISTS analysis_runs (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(), type text NOT NULL, status text NOT NULL DEFAULT 'queued', input jsonb NOT NULL DEFAULT '{}',
    error text, created_at timestamptz NOT NULL DEFAULT now(), completed_at timestamptz
);
CREATE TABLE IF NOT EXISTS analysis_snapshots (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(), run_id uuid REFERENCES analysis_runs(id) ON DELETE CASCADE, scope_id text NOT NULL,
    window_start timestamptz, window_end timestamptz, algorithm_version text NOT NULL, metric_name text NOT NULL, subject_id text NOT NULL, value jsonb NOT NULL
);
CREATE TABLE IF NOT EXISTS ego_networks (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(), target_qq text NOT NULL, depth integer NOT NULL DEFAULT 1, status text NOT NULL DEFAULT 'queued',
    input jsonb NOT NULL DEFAULT '{}', node_count integer NOT NULL DEFAULT 0, edge_count integer NOT NULL DEFAULT 0,
    created_at timestamptz NOT NULL DEFAULT now(), completed_at timestamptz
);
CREATE TABLE IF NOT EXISTS ego_network_nodes (
    network_id uuid NOT NULL REFERENCES ego_networks(id) ON DELETE CASCADE, node_key text NOT NULL, node_type text NOT NULL,
    label text NOT NULL DEFAULT '', metadata jsonb NOT NULL DEFAULT '{}', PRIMARY KEY(network_id, node_key)
);
CREATE TABLE IF NOT EXISTS ego_network_edges (
    network_id uuid NOT NULL REFERENCES ego_networks(id) ON DELETE CASCADE, source_key text NOT NULL, target_key text NOT NULL,
    relation_type text NOT NULL, weight numeric NOT NULL DEFAULT 1, evidence_ids uuid[] NOT NULL DEFAULT '{}', PRIMARY KEY(network_id, source_key, target_key, relation_type)
);

CREATE TABLE IF NOT EXISTS operation_requests (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(), account_id uuid NOT NULL REFERENCES napcat_accounts(id) ON DELETE CASCADE,
    endpoint text NOT NULL, parameters jsonb NOT NULL DEFAULT '{}', preview_hash text NOT NULL, status text NOT NULL DEFAULT 'pending',
    created_by uuid REFERENCES app_users(id) ON DELETE SET NULL, confirmed_at timestamptz, created_at timestamptz NOT NULL DEFAULT now()
);
CREATE TABLE IF NOT EXISTS operation_audits (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(), operation_id uuid NOT NULL REFERENCES operation_requests(id) ON DELETE CASCADE,
    request jsonb NOT NULL DEFAULT '{}', response jsonb NOT NULL DEFAULT '{}', status text NOT NULL, created_at timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE IF NOT EXISTS ai_runs (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(), task_type text NOT NULL, model_provider text NOT NULL DEFAULT '', model_name text NOT NULL DEFAULT '',
    prompt_version text NOT NULL DEFAULT '', input_hash text NOT NULL DEFAULT '', evidence_pack_id uuid, output jsonb NOT NULL DEFAULT '{}',
    status text NOT NULL DEFAULT 'queued', created_at timestamptz NOT NULL DEFAULT now(), completed_at timestamptz
);
CREATE TABLE IF NOT EXISTS evidence_packs (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(), subject_id text NOT NULL, scope jsonb NOT NULL DEFAULT '{}', event_ids uuid[] NOT NULL DEFAULT '{}',
    redaction_policy text NOT NULL DEFAULT 'local-only', token_budget integer NOT NULL DEFAULT 0, created_at timestamptz NOT NULL DEFAULT now()
);
CREATE TABLE IF NOT EXISTS inferences (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(), subject_id text NOT NULL, attribute_type text NOT NULL, value jsonb NOT NULL,
    confidence numeric NOT NULL DEFAULT 0, method_version text NOT NULL, evidence_event_ids uuid[] NOT NULL DEFAULT '{}',
    review_status text NOT NULL DEFAULT 'pending', created_at timestamptz NOT NULL DEFAULT now()
);
