package com.platform.core.security.http;

import com.platform.core.security.authorization.PrincipalContext;
import org.springframework.security.core.Authentication;
import org.springframework.security.core.context.SecurityContextHolder;

import java.util.Optional;

/** Access to the authenticated Platform principal from the security context. */
public final class PlatformSecurityContext {

    private PlatformSecurityContext() {
    }

    public static Optional<PrincipalContext> currentPrincipal() {
        Authentication authentication = SecurityContextHolder.getContext().getAuthentication();
        if (authentication instanceof PlatformAuthenticationToken token) {
            return Optional.of(token.principalContext());
        }
        return Optional.empty();
    }

    public static PrincipalContext requirePrincipal() {
        return currentPrincipal().orElseThrow(() -> new UnauthorizedAccessException("AUTH_REQUIRED", "Authentication required"));
    }
}
