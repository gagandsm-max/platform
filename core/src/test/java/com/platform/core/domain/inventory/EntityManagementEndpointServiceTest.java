package com.platform.core.domain.inventory;

import com.platform.core.domain.DomainException;
import com.platform.core.persistence.EntityManagementEndpointRepository;
import com.platform.core.persistence.EntityRepository;
import com.platform.core.security.authorization.AuthenticationState;
import com.platform.core.security.authorization.PrincipalContext;
import com.platform.core.security.http.PlatformAuthenticationToken;
import com.platform.core.security.http.SecurityAuditEmitter;
import com.platform.core.security.identity.AuthenticatedExternalIdentity;
import com.platform.core.security.identity.AuthenticationAssurance;
import com.platform.core.security.identity.ExternalSubject;
import com.platform.core.security.identity.IdentityProviderType;
import com.platform.core.security.identity.PlatformSubject;
import com.platform.core.security.identity.ProviderInstanceId;
import com.platform.core.security.identity.ResolvedAuthenticatedIdentity;
import org.junit.jupiter.api.AfterEach;
import org.junit.jupiter.api.BeforeEach;
import org.junit.jupiter.api.Test;
import org.springframework.security.core.context.SecurityContextHolder;

import java.time.Instant;
import java.util.List;
import java.util.Optional;
import java.util.Set;
import java.util.UUID;

import static org.junit.jupiter.api.Assertions.assertEquals;
import static org.junit.jupiter.api.Assertions.assertFalse;
import static org.junit.jupiter.api.Assertions.assertThrows;
import static org.junit.jupiter.api.Assertions.assertTrue;
import static org.mockito.ArgumentMatchers.any;
import static org.mockito.ArgumentMatchers.eq;
import static org.mockito.ArgumentMatchers.isNull;
import static org.mockito.Mockito.mock;
import static org.mockito.Mockito.never;
import static org.mockito.Mockito.verify;
import static org.mockito.Mockito.when;

class EntityManagementEndpointServiceTest {

    private static final UUID TENANT = UUID.fromString("018f0e0a-0000-7000-8000-0000000000a1");
    private static final UUID OTHER_TENANT = UUID.fromString("018f0e0a-0000-7000-8000-0000000000b1");
    private static final UUID ENTITY = UUID.fromString("018f0e0a-0000-7000-8000-0000000000e1");
    private static final UUID OTHER_ENTITY = UUID.fromString("018f0e0a-0000-7000-8000-0000000000e2");

    private EntityRepository entities;
    private EntityManagementEndpointRepository endpoints;
    private SecurityAuditEmitter audit;
    private EntityManagementEndpointService service;

    @BeforeEach
    void setUp() {
        entities = mock(EntityRepository.class);
        endpoints = mock(EntityManagementEndpointRepository.class);
        audit = mock(SecurityAuditEmitter.class);
        service = new EntityManagementEndpointService(entities, endpoints, audit);
        authenticateAs("admin-user");
    }

    @AfterEach
    void tearDown() {
        SecurityContextHolder.clearContext();
    }

    @Test
    void createIpv4Primary() {
        stubEntity(TENANT, ENTITY);
        EntityManagementEndpoint created = service.createManual(
                TENANT, ENTITY, ManagementEndpointAddressType.IPV4, "10.0.0.1", true);
        assertEquals("10.0.0.1", created.address());
        assertTrue(created.primary());
        verify(endpoints).clearPrimaryFlags(eq(TENANT), eq(ENTITY), isNull(), eq("admin-user"), any(Instant.class));
        verify(endpoints).insert(any());
    }

    @Test
    void createIpv6AndFqdn() {
        stubEntity(TENANT, ENTITY);
        assertEquals(ManagementEndpointAddressType.IPV6,
                service.createManual(TENANT, ENTITY, ManagementEndpointAddressType.IPV6, "2001:db8::1", false)
                        .addressType());
        assertEquals("device.example.com",
                service.createManual(TENANT, ENTITY, ManagementEndpointAddressType.FQDN, "Device.Example.COM", false)
                        .address());
    }

    @Test
    void rejectsEntityInOtherTenant() {
        when(entities.findByTenantAndId(TENANT, ENTITY)).thenReturn(Optional.empty());
        DomainException ex = assertThrows(DomainException.class,
                () -> service.createManual(TENANT, ENTITY, ManagementEndpointAddressType.IPV4, "10.0.0.1", true));
        assertEquals("ENTITY_NOT_FOUND", ex.getCode());
        verify(endpoints, never()).insert(any());
    }

    @Test
    void rejectsNonexistentEntity() {
        when(entities.findByTenantAndId(TENANT, OTHER_ENTITY)).thenReturn(Optional.empty());
        assertThrows(DomainException.class,
                () -> service.list(TENANT, OTHER_ENTITY));
    }

    @Test
    void rejectsUrlAsEndpoint() {
        stubEntity(TENANT, ENTITY);
        assertThrows(DomainException.class,
                () -> service.createManual(TENANT, ENTITY, ManagementEndpointAddressType.FQDN,
                        "http://host.example", false));
    }

    @Test
    void primaryTransitionClearsPreviousPrimary() {
        stubEntity(TENANT, ENTITY);
        Instant now = Instant.now();
        EntityManagementEndpoint existing = new EntityManagementEndpoint(
                UUID.fromString("018f0e0a-0000-7000-8000-0000000000f1"),
                TENANT, ENTITY, ManagementEndpointAddressType.IPV4, "10.0.0.2",
                ManagementEndpointState.ACTIVE, ManagementEndpointSource.MANUAL, false,
                "actor", now, "actor", now);
        when(endpoints.findByTenantEntityAndId(TENANT, ENTITY, existing.endpointId()))
                .thenReturn(Optional.of(existing));

        EntityManagementEndpoint updated = service.updateState(
                TENANT, ENTITY, existing.endpointId(), ManagementEndpointState.ACTIVE, true);
        assertTrue(updated.primary());
        verify(endpoints).clearPrimaryFlags(eq(TENANT), eq(ENTITY), eq(existing.endpointId()), eq("admin-user"), any(Instant.class));
        verify(endpoints).update(any());
    }

    @Test
    void resolvePrimaryReturnsActivePrimaryOnly() {
        stubEntity(TENANT, ENTITY);
        Instant now = Instant.now();
        EntityManagementEndpoint primary = new EntityManagementEndpoint(
                UUID.fromString("018f0e0a-0000-7000-8000-0000000000f2"),
                TENANT, ENTITY, ManagementEndpointAddressType.FQDN, "poll.example.com",
                ManagementEndpointState.ACTIVE, ManagementEndpointSource.MANUAL, true,
                "actor", now, "actor", now);
        when(endpoints.findActivePrimary(TENANT, ENTITY)).thenReturn(Optional.of(primary));
        Optional<EntityManagementEndpoint> resolved = service.resolvePrimaryManagementEndpoint(TENANT, ENTITY);
        assertTrue(resolved.isPresent());
        assertEquals("poll.example.com", resolved.get().address());
        assertFalse(resolved.get().address().contains("community"));
    }

    @Test
    void discoveryObservationAloneDoesNotCreateAuthority() {
        // There is no DiscoveryService coupling: only explicit createFromApprovedSource.
        stubEntity(TENANT, ENTITY);
        EntityManagementEndpoint created = service.createFromApprovedSource(
                TENANT, ENTITY, ManagementEndpointAddressType.IPV4, "10.1.2.3",
                ManagementEndpointSource.DISCOVERY, true);
        assertEquals(ManagementEndpointSource.DISCOVERY, created.source());
        EntityManagementEndpoint provisioned = service.createFromApprovedSource(
                TENANT, ENTITY, ManagementEndpointAddressType.FQDN, "edge.example.com",
                ManagementEndpointSource.PROVISIONING, false);
        assertEquals(ManagementEndpointSource.PROVISIONING, provisioned.source());
        // MANUAL is rejected for the approved-source boundary.
        assertThrows(DomainException.class,
                () -> service.createFromApprovedSource(
                        TENANT, ENTITY, ManagementEndpointAddressType.IPV4, "10.1.2.4",
                        ManagementEndpointSource.MANUAL, false));
    }

    @Test
    void createManualAlwaysSetsManualSource() {
        stubEntity(TENANT, ENTITY);
        EntityManagementEndpoint created = service.createManual(
                TENANT, ENTITY, ManagementEndpointAddressType.FQDN, "poll.example.com", true);
        assertEquals(ManagementEndpointSource.MANUAL, created.source());
    }

    @Test
    void crossTenantLookupOfOtherTenantEntityFails() {
        when(entities.findByTenantAndId(OTHER_TENANT, ENTITY)).thenReturn(Optional.empty());
        DomainException ex = assertThrows(DomainException.class,
                () -> service.resolvePrimaryManagementEndpoint(OTHER_TENANT, ENTITY));
        assertEquals("ENTITY_NOT_FOUND", ex.getCode());
    }

    private void stubEntity(UUID tenantId, UUID entityId) {
        Instant now = Instant.now();
        when(entities.findByTenantAndId(tenantId, entityId)).thenReturn(Optional.of(
                new Entity(entityId, tenantId, "DEVICE", "name", null, null,
                        LifecycleState.ACTIVE, null, null, null, null, null,
                        now, now, "actor", now, now)));
    }

    private static void authenticateAs(String subjectId) {
        Instant now = Instant.parse("2026-06-01T00:00:00Z");
        PrincipalContext principal = new PrincipalContext(
                subjectId, AuthenticationState.AUTHENTICATED, Set.of(), Set.of());
        UUID subjectUuid = UUID.fromString("018f0e0a-0000-7000-8000-0000000000aa");
        SecurityContextHolder.getContext().setAuthentication(
                new PlatformAuthenticationToken(
                        ResolvedAuthenticatedIdentity.current(
                                new PlatformSubject(subjectUuid),
                                new AuthenticatedExternalIdentity(
                                        IdentityProviderType.OIDC,
                                        new ProviderInstanceId("platform-oidc"),
                                        new ExternalSubject(subjectId),
                                        now,
                                        AuthenticationAssurance.withoutVerifiedMfa(),
                                        Instant.parse("2099-01-01T00:00:00Z")
                                ),
                                now
                        ),
                        principal
                )
        );
    }
}
