package com.platform.core.security.http;

import com.platform.core.security.authorization.AuthenticationState;
import com.platform.core.security.authorization.PermissionGrantDirectory;
import com.platform.core.security.authorization.PrincipalContext;
import com.platform.core.security.identity.AuthenticatedExternalIdentity;
import com.platform.core.security.identity.AuthenticationAssurance;
import com.platform.core.security.identity.ExternalSubject;
import com.platform.core.security.identity.IdentityProviderType;
import com.platform.core.security.identity.IdentityResolutionException;
import com.platform.core.security.identity.IdentityResolver;
import com.platform.core.security.identity.ProviderInstanceId;
import com.platform.core.security.identity.ResolvedAuthenticatedIdentity;
import org.springframework.core.convert.converter.Converter;
import org.springframework.security.authentication.AbstractAuthenticationToken;
import org.springframework.security.authentication.AuthenticationServiceException;
import org.springframework.security.oauth2.jwt.Jwt;
import org.springframework.security.oauth2.server.resource.InvalidBearerTokenException;
import org.springframework.stereotype.Component;

import java.time.Instant;
import java.util.Objects;

/**
 * Converts a signature-validated JWT into a PlatformAuthenticationToken.
 * Role/permission claims in the token are ignored for authorization.
 */
@Component
public final class PlatformJwtAuthenticationConverter implements Converter<Jwt, AbstractAuthenticationToken> {

    private final OidcResourceServerProperties properties;
    private final IdentityResolver identityResolver;
    private final PermissionGrantDirectory grantDirectory;
    private final SecurityAuditEmitter auditEmitter;

    public PlatformJwtAuthenticationConverter(
            OidcResourceServerProperties properties,
            IdentityResolver identityResolver,
            PermissionGrantDirectory grantDirectory,
            SecurityAuditEmitter auditEmitter
    ) {
        this.properties = Objects.requireNonNull(properties);
        this.identityResolver = Objects.requireNonNull(identityResolver);
        this.grantDirectory = Objects.requireNonNull(grantDirectory);
        this.auditEmitter = Objects.requireNonNull(auditEmitter);
    }

    @Override
    public AbstractAuthenticationToken convert(Jwt jwt) {
        Instant now = Instant.now();
        String subject = jwt.getSubject();
        if (subject == null || subject.isBlank()) {
            auditEmitter.authenticationFailure("AUTH_SUBJECT_MISSING");
            throw new InvalidBearerTokenException("Token subject is missing");
        }

        Instant issuedAt = jwt.getIssuedAt() == null ? now : jwt.getIssuedAt();
        Instant expiresAt = jwt.getExpiresAt();

        AuthenticatedExternalIdentity evidence;
        try {
            evidence = new AuthenticatedExternalIdentity(
                    IdentityProviderType.OIDC,
                    new ProviderInstanceId(properties.getProviderInstanceId()),
                    new ExternalSubject(subject),
                    issuedAt,
                    AuthenticationAssurance.withoutVerifiedMfa(),
                    expiresAt
            );
        } catch (RuntimeException ex) {
            auditEmitter.authenticationFailure("AUTH_IDENTITY_INVALID");
            throw new InvalidBearerTokenException("Token identity is invalid");
        }

        ResolvedAuthenticatedIdentity resolved;
        try {
            resolved = identityResolver.resolve(evidence, now);
        } catch (IdentityResolutionException ex) {
            auditEmitter.authenticationFailure(ex.failure().name());
            throw new InvalidBearerTokenException("Identity could not be resolved");
        } catch (RuntimeException ex) {
            auditEmitter.authenticationFailure("AUTH_IDENTITY_ERROR");
            throw new AuthenticationServiceException("Identity resolution failed");
        }

        PermissionGrantDirectory.PrincipalAuthorization authz = grantDirectory.load(resolved.platformSubject());
        PrincipalContext principal = new PrincipalContext(
                resolved.platformSubject().subjectId().toString(),
                AuthenticationState.AUTHENTICATED,
                authz.tenantScopes(),
                authz.permissionGrants()
        );
        return new PlatformAuthenticationToken(resolved, principal);
    }
}
