package com.platform.core.security.authorization;

/**
 * Bounded resource types required by the current Day-02 domain plus the credential permission boundary.
 */
public enum ResourceType {
    TENANT,
    SITE,
    GROUP,
    TAG,
    ENTITY,
    ENTITY_IDENTIFIER,
    CREDENTIAL
}
