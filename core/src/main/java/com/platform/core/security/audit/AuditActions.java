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

    private AuditActions() {}
}
