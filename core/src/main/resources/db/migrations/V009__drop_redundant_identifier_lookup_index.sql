-- V009: remove redundant entity_identifier lookup index.
--
-- V001 defines uq_identifier_resolution as a UNIQUE constraint on exactly
-- (tenant_id, namespace, identifier_type, identifier_value). PostgreSQL backs
-- that constraint with a unique btree index, so idx_identifier_lookup duplicates
-- the same lookup prefix/order and adds write/storage overhead without adding a
-- distinct access path.
--
-- Forward-only migration: never rewrite V001 because deployed databases may
-- already have its checksum recorded.

DROP INDEX IF EXISTS platform.idx_identifier_lookup;
