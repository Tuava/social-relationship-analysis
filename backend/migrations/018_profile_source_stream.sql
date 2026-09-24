CREATE INDEX IF NOT EXISTS person_profiles_source_stream_idx
    ON person_profiles(person_id, source, source_account_id, valid_from DESC);
