package com.platform.core.security.authorization;

import java.util.Objects;

/**
 * Immutable authorization outcome. Does not embed principal/grant dumps or secrets.
 * DenyReason is an internal classification; HTTP layers should avoid leaking existence oracles.
 */
public record AuthorizationDecision(Outcome outcome, DenyReason denyReason) {
    public enum Outcome {
        ALLOW,
        DENY
    }

    public enum DenyReason {
        NOT_AUTHENTICATED,
        TENANT_SCOPE_DENIED,
        PERMISSION_DENIED,
        RESOURCE_SCOPE_DENIED,
        INVALID_REQUEST
    }

    public AuthorizationDecision {
        Objects.requireNonNull(outcome, "outcome must not be null");
        if (outcome == Outcome.ALLOW && denyReason != null) {
            throw new IllegalArgumentException("ALLOW must not carry a deny reason");
        }
        if (outcome == Outcome.DENY && denyReason == null) {
            throw new IllegalArgumentException("DENY requires a deny reason");
        }
    }

    public static AuthorizationDecision allow() {
        return new AuthorizationDecision(Outcome.ALLOW, null);
    }

    public static AuthorizationDecision deny(DenyReason reason) {
        return new AuthorizationDecision(Outcome.DENY, Objects.requireNonNull(reason, "reason must not be null"));
    }

    public boolean allowed() {
        return outcome == Outcome.ALLOW;
    }

    @Override
    public String toString() {
        if (outcome == Outcome.ALLOW) {
            return "AuthorizationDecision[ALLOW]";
        }
        return "AuthorizationDecision[DENY," + denyReason + "]";
    }
}
