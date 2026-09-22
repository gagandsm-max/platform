package com.platform.core.domain.inventory;

import com.platform.core.domain.DomainException;
import com.platform.core.domain.tenant.Tenant;
import com.platform.core.domain.tenant.TenantMode;
import com.platform.core.domain.tenant.TenantService;
import com.platform.core.domain.tenant.TenantStatus;
import com.platform.core.persistence.EntityAttributeRepository;
import com.platform.core.persistence.EntityGroupRepository;
import com.platform.core.persistence.EntityIdentifierRepository;
import com.platform.core.persistence.EntityManagementEndpointRepository;
import com.platform.core.persistence.EntityRepository;
import com.platform.core.persistence.MembershipRepository;
import com.platform.core.persistence.SiteRepository;
import com.platform.core.persistence.TagRepository;
import com.platform.core.persistence.TenantRepository;
import com.platform.core.security.http.SecurityAuditEmitter;
import com.platform.core.support.PostgresTestSupport;
import org.junit.jupiter.api.AfterEach;
import org.junit.jupiter.api.BeforeEach;
import org.junit.jupiter.api.Tag;
import org.junit.jupiter.api.Test;
import org.springframework.dao.DataIntegrityViolationException;
import org.springframework.jdbc.core.JdbcTemplate;
import org.springframework.jdbc.core.namedparam.NamedParameterJdbcTemplate;

import java.util.List;
import java.util.Optional;
import java.util.UUID;

import static org.junit.jupiter.api.Assertions.assertEquals;
import static org.junit.jupiter.api.Assertions.assertThrows;
import static org.junit.jupiter.api.Assertions.assertTrue;
import static org.mockito.Mockito.mock;

@Tag("postgres")
class EntityManagementEndpointIntegrationTest {

    private PostgresTestSupport pg;
    private TenantService tenants;
    private EntityService entities;
    private EntityManagementEndpointService endpoints;
    private EntityManagementEndpointRepository endpointRepo;

    @BeforeEach
    void setUp() throws Exception {
        pg = new PostgresTestSupport();
        pg.start();
        pg.migrate();
        NamedParameterJdbcTemplate named = pg.namedJdbc();
        TenantRepository tenantRepo = new TenantRepository(named);
        SiteRepository siteRepo = new SiteRepository(named);
        EntityGroupRepository groupRepo = new EntityGroupRepository(named);
        TagRepository tagRepo = new TagRepository(named);
        EntityRepository entityRepo = new EntityRepository(named);
        EntityIdentifierRepository identifierRepo = new EntityIdentifierRepository(named);
        EntityAttributeRepository attributeRepo = new EntityAttributeRepository(named);
        MembershipRepository membershipRepo = new MembershipRepository(named);
        endpointRepo = new EntityManagementEndpointRepository(named);

        tenants = new TenantService(tenantRepo);
        EntityGroupService groups = new EntityGroupService(groupRepo, tenants);
        TagService tags = new TagService(tagRepo, tenants);
        entities = new EntityService(
                entityRepo, identifierRepo, attributeRepo, membershipRepo, siteRepo, groups, tags, tenants
        );
        endpoints = new EntityManagementEndpointService(entityRepo, endpointRepo, mock(SecurityAuditEmitter.class));
    }

    @AfterEach
    void tearDown() {
        if (pg != null) {
            pg.stop();
        }
    }

    @Test
    void primaryUniquenessCrossTenantFkAndRestartPersistence() {
        Tenant tenantA = tenants.create("ME-A", TenantMode.SINGLE, TenantStatus.ACTIVE, "UTC");
        Tenant tenantB = tenants.create("ME-B", TenantMode.SINGLE, TenantStatus.ACTIVE, "UTC");
        Entity entityA = entities.create(
                tenantA.tenantId(), "NETWORK_DEVICE", "Edge-A", null, null,
                LifecycleState.ACTIVE, null, null, null, null, null, "tester", List.of());
        Entity entityB = entities.create(
                tenantB.tenantId(), "NETWORK_DEVICE", "Edge-B", null, null,
                LifecycleState.ACTIVE, null, null, null, null, null, "tester", List.of());

        EntityManagementEndpoint primary = endpoints.createManual(
                tenantA.tenantId(), entityA.entityId(),
                ManagementEndpointAddressType.IPV4, "10.20.30.40", true);
        endpoints.createManual(
                tenantA.tenantId(), entityA.entityId(),
                ManagementEndpointAddressType.FQDN, "backup.example.com", false);

        Optional<EntityManagementEndpoint> resolved =
                endpoints.resolvePrimaryManagementEndpoint(tenantA.tenantId(), entityA.entityId());
        assertTrue(resolved.isPresent());
        assertEquals(primary.endpointId(), resolved.get().endpointId());
        assertEquals("10.20.30.40", resolved.get().address());

        // DB unique partial index: second ACTIVE primary for same entity must fail.
        JdbcTemplate jdbc = pg.jdbc();
        assertThrows(DataIntegrityViolationException.class, () -> jdbc.update("""
                INSERT INTO platform.entity_management_endpoint
                    (endpoint_id, tenant_id, entity_id, address_type, address, state, source,
                     is_primary, created_by, created_at, updated_by, updated_at)
                VALUES
                    ('018f0e0a-0000-7000-8000-00000000dead'::uuid, ?::uuid, ?::uuid,
                     'IPV4', '10.20.30.41', 'ACTIVE', 'MANUAL', TRUE,
                     'tester', NOW(), 'tester', NOW())
                """, tenantA.tenantId(), entityA.entityId()));

        // Cross-tenant entity attachment blocked by composite FK.
        assertThrows(DataIntegrityViolationException.class, () -> jdbc.update("""
                INSERT INTO platform.entity_management_endpoint
                    (endpoint_id, tenant_id, entity_id, address_type, address, state, source,
                     is_primary, created_by, created_at, updated_by, updated_at)
                VALUES
                    ('018f0e0a-0000-7000-8000-00000000beef'::uuid, ?::uuid, ?::uuid,
                     'IPV4', '10.20.30.50', 'ACTIVE', 'MANUAL', TRUE,
                     'tester', NOW(), 'tester', NOW())
                """, tenantA.tenantId(), entityB.entityId()));

        // Cross-tenant resolve fails closed (entity not in tenant).
        DomainException cross = assertThrows(DomainException.class,
                () -> endpoints.resolvePrimaryManagementEndpoint(tenantB.tenantId(), entityA.entityId()));
        assertEquals("ENTITY_NOT_FOUND", cross.getCode());

        // Discovery observation alone never auto-creates authority.
        assertEquals(2, endpoints.list(tenantA.tenantId(), entityA.entityId()).size());
        assertEquals(0, endpoints.list(tenantB.tenantId(), entityB.entityId()).size());

        // Restart persistence: remigrate is idempotent and rows survive.
        pg.migrate();
        assertEquals(1, jdbc.queryForObject(
                "SELECT COUNT(*) FROM platform.schema_history WHERE version='V010'", Integer.class));
        assertEquals(2, jdbc.queryForObject(
                "SELECT COUNT(*) FROM platform.entity_management_endpoint WHERE tenant_id=?::uuid",
                Integer.class, tenantA.tenantId()));
        assertEquals(primary.endpointId(),
                endpoints.resolvePrimaryManagementEndpoint(tenantA.tenantId(), entityA.entityId())
                        .orElseThrow().endpointId());
    }
}
