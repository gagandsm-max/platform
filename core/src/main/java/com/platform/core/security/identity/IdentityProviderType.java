package com.platform.core.security.identity;

/** Provider families that may authenticate an external identity. */
public enum IdentityProviderType {
    LOCAL,
    OIDC,
    SAML,
    LDAP
}
