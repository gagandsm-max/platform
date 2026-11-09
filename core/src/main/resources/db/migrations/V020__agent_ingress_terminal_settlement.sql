-- Core-authoritative Agent-ingress continuity. Accepted ACK semantics remain unchanged.
ALTER TABLE platform.sensor_agent_ingress_watermark
    ADD COLUMN settled_through_sequence BIGINT;

ALTER TABLE platform.sensor_agent_ingress_watermark
    ADD COLUMN pending_request_sha256 VARCHAR(64),
    ADD COLUMN last_request_sha256 VARCHAR(64),
    ADD CONSTRAINT chk_agent_ingress_pending_request_sha256 CHECK (
        pending_request_sha256 IS NULL OR pending_request_sha256 ~ '^[0-9a-f]{64}$'),
    ADD CONSTRAINT chk_agent_ingress_last_request_sha256 CHECK (
        last_request_sha256 IS NULL OR last_request_sha256 ~ '^[0-9a-f]{64}$');

UPDATE platform.sensor_agent_ingress_watermark
   SET settled_through_sequence = acked_through_sequence;

ALTER TABLE platform.sensor_agent_ingress_watermark
    ALTER COLUMN settled_through_sequence SET NOT NULL,
    ADD CONSTRAINT chk_agent_ingress_settled_max_safe CHECK (
        settled_through_sequence >= acked_through_sequence
        AND settled_through_sequence <= 9007199254740991
    );

CREATE TABLE platform.sensor_agent_ingress_terminal_disposition (
    disposition_id UUID PRIMARY KEY,
    tenant_id UUID NOT NULL REFERENCES platform.tenant(tenant_id) ON DELETE CASCADE,
    sensor_id VARCHAR(255) NOT NULL,
    stream_id VARCHAR(128) NOT NULL,
    first_sequence BIGINT NOT NULL,
    last_sequence BIGINT NOT NULL,
    checksum VARCHAR(64) NOT NULL,
    request_sha256 VARCHAR(64),
    batch_id VARCHAR(255) NOT NULL,
    sensor_ingress_record_id VARCHAR(255) NOT NULL,
    agent_batch_id VARCHAR(255) NOT NULL,
    correlation_id VARCHAR(255) NOT NULL,
    agent_id VARCHAR(128) NOT NULL,
    assignment_generation BIGINT NOT NULL,
    entity_id UUID,
    binding_version BIGINT,
    claimed_host_id VARCHAR(255),
    reason_code VARCHAR(64) NOT NULL,
    disposition_state VARCHAR(32) NOT NULL DEFAULT 'TERMINALLY_REJECTED',
    created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    CONSTRAINT uq_agent_ingress_terminal_sequence
        UNIQUE (tenant_id, sensor_id, stream_id, first_sequence),
    CONSTRAINT chk_agent_ingress_terminal_singleton CHECK (
        first_sequence = last_sequence
        AND first_sequence BETWEEN 1 AND 9007199254740991
    ),
    CONSTRAINT chk_agent_ingress_terminal_stream CHECK (stream_id ~ '^stream-[0-9a-f]{32}$'),
    CONSTRAINT chk_agent_ingress_terminal_checksum CHECK (checksum ~ '^[0-9a-f]{64}$'),
    CONSTRAINT chk_agent_ingress_terminal_request_sha256 CHECK (
        request_sha256 IS NULL OR request_sha256 ~ '^[0-9a-f]{64}$'),
    CONSTRAINT chk_agent_ingress_terminal_assignment CHECK (
        assignment_generation BETWEEN 1 AND 9007199254740991
    ),
    CONSTRAINT chk_agent_ingress_terminal_reason CHECK (reason_code = 'ERR_AGENT_IDENTITY_CONFLICT'),
    CONSTRAINT chk_agent_ingress_terminal_state CHECK (disposition_state = 'TERMINALLY_REJECTED')
);

CREATE FUNCTION platform.reject_agent_ingress_terminal_mutation() RETURNS trigger
LANGUAGE plpgsql AS $$
BEGIN
    RAISE EXCEPTION 'Agent-ingress terminal disposition is immutable';
END;
$$;

CREATE TRIGGER trg_agent_ingress_terminal_immutable
    BEFORE UPDATE OR DELETE ON platform.sensor_agent_ingress_terminal_disposition
    FOR EACH ROW EXECUTE FUNCTION platform.reject_agent_ingress_terminal_mutation();
