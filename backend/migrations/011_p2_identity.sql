-- P2-1: Add card column to group_memberships for per-group nicknames
ALTER TABLE group_memberships ADD COLUMN IF NOT EXISTS card text NOT NULL DEFAULT '';

-- P2-2: Add structured profile columns extracted from profile_data jsonb
ALTER TABLE person_profiles ADD COLUMN IF NOT EXISTS sex text NOT NULL DEFAULT '';
ALTER TABLE person_profiles ADD COLUMN IF NOT EXISTS age integer;
ALTER TABLE person_profiles ADD COLUMN IF NOT EXISTS area text NOT NULL DEFAULT '';
ALTER TABLE person_profiles ADD COLUMN IF NOT EXISTS signature text NOT NULL DEFAULT '';
ALTER TABLE person_profiles ADD COLUMN IF NOT EXISTS reg_time bigint;
ALTER TABLE person_profiles ADD COLUMN IF NOT EXISTS login_days integer;

-- P2-3: Index for nickname change queries
CREATE INDEX IF NOT EXISTS person_profiles_person_nickname_idx ON person_profiles(person_id, valid_from DESC);

-- P2-4: Add reply_target_content_id for comment-to-comment nesting
ALTER TABLE contents ADD COLUMN IF NOT EXISTS reply_to_content_id uuid REFERENCES contents(id) ON DELETE SET NULL;
CREATE INDEX IF NOT EXISTS contents_reply_target_idx ON contents(reply_to_content_id) WHERE reply_to_content_id IS NOT NULL;
