-- V009: remove redundant entity_identifier lookup index
--
-- V001 creates the tenant-scoped uniqueness constraint:
--   uq_identifier_resolution UNIQUE (tenant_id, namespace, identifier_type, identifier_value)
-- PostgreSQL backs that constraint with a unique btree index that already serves the
-- identical lookup prefix. V001 also created idx_identifier_lookup with the same key
-- columns, duplicating write amplification and storage without adding a distinct access path.
--
-- Do not rewrite V001 because it may already be applied and checksum-protected.
DROP INDEX IF EXISTS platform.idx_identifier_lookup;
