-- A NULL source account was used by the first coverage endpoint as a
-- placeholder. Once account-scoped coverage exists, that placeholder must
-- not shadow the real result.
DELETE FROM collection_coverage legacy
WHERE legacy.source_account_id IS NULL
  AND EXISTS (
    SELECT 1
    FROM collection_coverage scoped
    WHERE scoped.person_id = legacy.person_id
      AND scoped.data_source = legacy.data_source
      AND scoped.source_account_id IS NOT NULL
  );

CREATE UNIQUE INDEX IF NOT EXISTS collection_coverage_identity_idx
    ON collection_coverage(
      person_id,
      COALESCE(source_account_id, '00000000-0000-0000-0000-000000000000'::uuid),
      data_source
    );
