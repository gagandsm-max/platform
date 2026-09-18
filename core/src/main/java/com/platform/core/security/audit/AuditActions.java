package com.platform.core.security.audit;

public final class AuditActions {
    public static final AuditAction AUTHENTICATION_LOGIN = new AuditAction("authentication.login");
    public static final AuditAction AUTHENTICATION_LOGOUT = new AuditAction("authentication.logout");
    public static final AuditAction AUTHORIZATION_ALLOW = new AuditAction("authorization.allow");
    public static final AuditAction AUTHORIZATION_DENY = new AuditAction("authorization.deny");
    public static final AuditAction IDENTITY_MAPPING_RESOLVE = new AuditAction("identity.mapping.resolve");
    public static final AuditAction MFA_CHALLENGE = new AuditAction("mfa.challenge");
    public static final AuditAction CREDENTIAL_USE = new AuditAction("credential.use");
    public static final AuditAction CREDENTIAL_EDIT = new AuditAction("credential.edit");
    public static final AuditAction CREDENTIAL_REVEAL = new AuditAction("credential.reveal");

    /** Sensor Control Authority — credential lifecycle (no secrets). */
    public static final AuditAction SENSOR_CREDENTIAL_BIND = new AuditAction("sensor.credential.bind");
    public static final AuditAction SENSOR_CREDENTIAL_ROTATE = new AuditAction("sensor.credential.rotate");
    public static final AuditAction SENSOR_CREDENTIAL_REVOKE = new AuditAction("sensor.credential.revoke");

    /** Sensor Control Authority — administration / placement. */
    public static final AuditAction SENSOR_ENROLL = new AuditAction("sensor.enroll");
    public static final AuditAction SENSOR_DISABLE = new AuditAction("sensor.disable");
    public static final AuditAction SENSOR_SCOPE_REPLACE = new AuditAction("sensor.scope.replace");
    public static final AuditAction DISCOVERY_PLACEMENT_ASSIGN = new AuditAction("discovery.placement.assign");
    public static final AuditAction DISCOVERY_PLACEMENT_REJECT = new AuditAction("discovery.placement.reject");

    /** Agent Control Authority — credential lifecycle (no secrets). */
    public static final AuditAction AGENT_ENROLL = new AuditAction("agent.enroll");
    public static final AuditAction AGENT_CREDENTIAL_BIND = new AuditAction("agent.credential.bind");
    public static final AuditAction AGENT_CREDENTIAL_ROTATE = new AuditAction("agent.credential.rotate");
    public static final AuditAction AGENT_CREDENTIAL_REVOKE = new AuditAction("agent.credential.revoke");

    /** Agent Control Authority — administration / assignment (disable ≠ revoke). */
    public static final AuditAction AGENT_DISABLE = new AuditAction("agent.disable");
    public static final AuditAction AGENT_REVOKE = new AuditAction("agent.revoke");
    public static final AuditAction AGENT_ASSIGN = new AuditAction("agent.assign");
    public static final AuditAction AGENT_REASSIGN = new AuditAction("agent.reassign");
    public static final AuditAction AGENT_ASSIGNMENT_REVOKE = new AuditAction("agent.assignment.revoke");

    /** Capability Pack Sensor assignment (desired-state distribution; no manifest contents). */
    public static final AuditAction CAPABILITY_PACK_SENSOR_ASSIGN = new AuditAction("capabilitypack.sensor.assign");
    public static final AuditAction CAPABILITY_PACK_SENSOR_UNASSIGN = new AuditAction("capabilitypack.sensor.unassign");

    private AuditActions() {}
}
