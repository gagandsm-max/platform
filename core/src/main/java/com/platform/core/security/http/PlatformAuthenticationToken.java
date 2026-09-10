package com.platform.core.security.http;

import com.platform.core.security.authorization.PrincipalContext;
import com.platform.core.security.identity.ResolvedAuthenticatedIdentity;
import org.springframework.security.authentication.AbstractAuthenticationToken;
import org.springframework.security.core.authority.AuthorityUtils;

import java.util.Objects;

/**
 * Framework Authentication carrying Platform principal context.
 * Spring authorities are intentionally empty — Platform authorization is grant-based, not ROLE_*.
 */
public final class PlatformAuthenticationToken extends AbstractAuthenticationToken {

    private final ResolvedAuthenticatedIdentity resolvedIdentity;
    private final PrincipalContext principalContext;

    public PlatformAuthenticationToken(
            ResolvedAuthenticatedIdentity resolvedIdentity,
            PrincipalContext principalContext
    ) {
        super(AuthorityUtils.NO_AUTHORITIES);
        this.resolvedIdentity = Objects.requireNonNull(resolvedIdentity, "resolvedIdentity");
        this.principalContext = Objects.requireNonNull(principalContext, "principalContext");
        setAuthenticated(true);
    }

    @Override
    public Object getCredentials() {
        return "";
    }

    @Override
    public PrincipalContext getPrincipal() {
        return principalContext;
    }

    public ResolvedAuthenticatedIdentity resolvedIdentity() {
        return resolvedIdentity;
    }

    public PrincipalContext principalContext() {
        return principalContext;
    }
}
