-- BUILD-004 Slice 1: MAX_SAFE upper bounds for physical control-generation counters.
-- Additive only. Does not rewrite V003/V005/V007/V008 or clamp existing rows.
-- Invalid historical values above MAX_SAFE fail VALIDATE rather than silent rewrite.

-- discovery_request.assignment_version (>= 0 already; add upper bound)
ALTER TABLE platform.discovery_request
    DROP CONSTRAINT IF EXISTS chk_discovery_assignment_version_max_safe;
ALTER TABLE platform.discovery_request
    ADD CONSTRAINT chk_discovery_assignment_version_max_safe
    CHECK (assignment_version <= 9007199254740991) NOT VALID;
ALTER TABLE platform.discovery_request
    VALIDATE CONSTRAINT chk_discovery_assignment_version_max_safe;

-- agent_sensor_assignment.generation (>= 1 already; add upper bound)
ALTER TABLE platform.agent_sensor_assignment
    DROP CONSTRAINT IF EXISTS chk_agent_sensor_assignment_generation_max_safe;
ALTER TABLE platform.agent_sensor_assignment
    ADD CONSTRAINT chk_agent_sensor_assignment_generation_max_safe
    CHECK (generation <= 9007199254740991) NOT VALID;
ALTER TABLE platform.agent_sensor_assignment
    VALIDATE CONSTRAINT chk_agent_sensor_assignment_generation_max_safe;

-- tenant_capability_pack_activation.generation (>= 1 already; add upper bound)
ALTER TABLE platform.tenant_capability_pack_activation
    DROP CONSTRAINT IF EXISTS chk_tcpa_generation_max_safe;
ALTER TABLE platform.tenant_capability_pack_activation
    ADD CONSTRAINT chk_tcpa_generation_max_safe
    CHECK (generation <= 9007199254740991) NOT VALID;
ALTER TABLE platform.tenant_capability_pack_activation
    VALIDATE CONSTRAINT chk_tcpa_generation_max_safe;

-- sensor_capability_pack_state desired/applied (>= 0 and applied <= desired already)
ALTER TABLE platform.sensor_capability_pack_state
    DROP CONSTRAINT IF EXISTS chk_scps_desired_generation_max_safe;
ALTER TABLE platform.sensor_capability_pack_state
    ADD CONSTRAINT chk_scps_desired_generation_max_safe
    CHECK (desired_generation <= 9007199254740991) NOT VALID;
ALTER TABLE platform.sensor_capability_pack_state
    VALIDATE CONSTRAINT chk_scps_desired_generation_max_safe;

ALTER TABLE platform.sensor_capability_pack_state
    DROP CONSTRAINT IF EXISTS chk_scps_applied_generation_max_safe;
ALTER TABLE platform.sensor_capability_pack_state
    ADD CONSTRAINT chk_scps_applied_generation_max_safe
    CHECK (applied_generation <= 9007199254740991) NOT VALID;
ALTER TABLE platform.sensor_capability_pack_state
    VALIDATE CONSTRAINT chk_scps_applied_generation_max_safe;
