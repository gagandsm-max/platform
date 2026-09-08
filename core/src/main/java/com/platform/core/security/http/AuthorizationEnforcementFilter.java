package com.platform.core.security.http;

import com.platform.core.security.authorization.AuthorizationDecision;
import com.platform.core.security.authorization.PrincipalContext;
import jakarta.servlet.FilterChain;
import jakarta.servlet.ServletException;
import jakarta.servlet.http.HttpServletRequest;
import jakarta.servlet.http.HttpServletResponse;
import org.springframework.core.Ordered;
import org.springframework.core.annotation.Order;
import org.springframework.security.core.Authentication;
import org.springframework.security.core.context.SecurityContextHolder;
import org.springframework.stereotype.Component;
import org.springframework.web.filter.OncePerRequestFilter;

import java.io.IOException;
import java.util.Optional;
import java.util.UUID;

/**
 * Server-side authorization for authenticated business APIs.
 * Runs after Spring Security authentication; never trusts client-supplied roles.
 */
@Component
@Order(Ordered.LOWEST_PRECEDENCE - 50)
public class AuthorizationEnforcementFilter extends OncePerRequestFilter {

    private final AuthorizationRouteMapper routeMapper;
    private final SecurityErrorResponseWriter errorWriter;
    private final SecurityAuditEmitter auditEmitter;

    public AuthorizationEnforcementFilter(
            AuthorizationRouteMapper routeMapper,
            SecurityErrorResponseWriter errorWriter,
            SecurityAuditEmitter auditEmitter
    ) {
        this.routeMapper = routeMapper;
        this.errorWriter = errorWriter;
        this.auditEmitter = auditEmitter;
    }

    @Override
    protected boolean shouldNotFilter(HttpServletRequest request) {
        String path = request.getRequestURI();
        if (path == null) {
            return true;
        }
        return path.startsWith("/actuator/")
                || path.equals("/api/v1/version")
                || path.startsWith("/api/v1/health/")
                || path.startsWith("/api/v1/sensor/")
                || path.equals("/api/v1/telemetry");
    }

    @Override
    protected void doFilterInternal(
            HttpServletRequest request,
            HttpServletResponse response,
            FilterChain filterChain
    ) throws ServletException, IOException {
        Authentication authentication = SecurityContextHolder.getContext().getAuthentication();
        if (!(authentication instanceof PlatformAuthenticationToken token)) {
            // Business APIs require PlatformAuthenticationToken; missing => 401.
            if (request.getRequestURI() != null && request.getRequestURI().startsWith("/api/")) {
                auditEmitter.authenticationFailure("AUTH_REQUIRED");
                errorWriter.writeUnauthorized(request, response, "AUTH_REQUIRED");
                return;
            }
            filterChain.doFilter(request, response);
            return;
        }

        PrincipalContext principal = token.principalContext();
        Optional<AuthorizationDecision> decision = routeMapper.evaluate(
                principal,
                request.getMethod(),
                request.getRequestURI()
        );
        if (decision.isEmpty()) {
            filterChain.doFilter(request, response);
            return;
        }

        AuthorizationDecision result = decision.get();
        UUID actorId = token.resolvedIdentity().platformSubject().subjectId();
        UUID tenantId = extractTenantId(request.getRequestURI());
        if (result.allowed()) {
            auditEmitter.authorizationAllow(actorId, tenantId, "AUTHORIZATION_ALLOW");
            filterChain.doFilter(request, response);
            return;
        }

        String reason = result.denyReason() == null ? "PERMISSION_DENIED" : result.denyReason().name();
        auditEmitter.authorizationDeny(actorId, tenantId, reason);
        if (result.denyReason() == AuthorizationDecision.DenyReason.NOT_AUTHENTICATED) {
            errorWriter.writeUnauthorized(request, response, "AUTH_REQUIRED");
            return;
        }
        // Conceal tenant/resource existence for all other denials.
        errorWriter.writeNotFound(response);
    }

    private static UUID extractTenantId(String path) {
        if (path == null) {
            return null;
        }
        String marker = "/api/v1/tenants/";
        int idx = path.indexOf(marker);
        if (idx < 0) {
            return null;
        }
        String rest = path.substring(idx + marker.length());
        int slash = rest.indexOf('/');
        String candidate = slash < 0 ? rest : rest.substring(0, slash);
        try {
            return UUID.fromString(candidate);
        } catch (IllegalArgumentException ex) {
            return null;
        }
    }
}
