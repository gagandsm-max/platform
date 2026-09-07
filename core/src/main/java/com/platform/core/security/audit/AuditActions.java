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

    private AuditActions() {}
}
