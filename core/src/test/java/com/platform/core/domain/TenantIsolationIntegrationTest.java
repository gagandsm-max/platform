package com.platform.core.domain;

import com.platform.core.api.PageResult;
import com.platform.core.domain.inventory.Entity;
import com.platform.core.domain.inventory.EntityGroup;
import com.platform.core.domain.inventory.EntityGroupService;
import com.platform.core.domain.inventory.EntityService;
import com.platform.core.domain.inventory.LifecycleState;
import com.platform.core.domain.inventory.Site;
import com.platform.core.domain.inventory.SiteService;
import com.platform.core.domain.inventory.Tag;
import com.platform.core.domain.inventory.TagService;
import com.platform.core.domain.tenant.Tenant;
import com.platform.core.domain.tenant.TenantMode;
import com.platform.core.domain.tenant.TenantService;
import com.platform.core.domain.tenant.TenantStatus;
import com.platform.core.persistence.EntityAttributeRepository;
import com.platform.core.persistence.EntityGroupRepository;
import com.platform.core.persistence.EntityIdentifierRepository;
import com.platform.core.persistence.EntityRepository;
import com.platform.core.persistence.MembershipRepository;
import com.platform.core.persistence.SiteRepository;
import com.platform.core.persistence.TagRepository;
import com.platform.core.persistence.TenantRepository;
import com.platform.core.support.PostgresTestSupport;
import org.junit.jupiter.api.AfterEach;
import org.junit.jupiter.api.BeforeEach;
import org.junit.jupiter.api.Test;
import org.springframework.jdbc.core.namedparam.NamedParameterJdbcTemplate;

import java.math.BigDecimal;
import java.util.List;
import java.util.UUID;

import static org.junit.jupiter.api.Assertions.assertEquals;
import static org.junit.jupiter.api.Assertions.assertNotEquals;
import static org.junit.jupiter.api.Assertions.assertThrows;
import static org.junit.jupiter.api.Assertions.assertTrue;

class TenantIsolationIntegrationTest {

    private PostgresTestSupport pg;
    private TenantService tenants;
    private SiteService sites;
    private EntityGroupService groups;
    private TagService tags;
    private EntityService entities;

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

        tenants = new TenantService(tenantRepo);
        sites = new SiteService(siteRepo, tenants);
        groups = new EntityGroupService(groupRepo, tenants);
        tags = new TagService(tagRepo, tenants);
        entities = new EntityService(
                entityRepo, identifierRepo, attributeRepo, membershipRepo, siteRepo, groups, tags, tenants
        );
    }

    @AfterEach
    void tearDown() {
        if (pg != null) {
            pg.stop();
        }
    }

    @Test
    void tenantIsolationMatrixAndOverlappingAddresses() {
        Tenant a = tenants.create("Tenant A", TenantMode.SINGLE, TenantStatus.ACTIVE, "UTC");
        Tenant b = tenants.create("Tenant B", TenantMode.MSP_CHILD, TenantStatus.ACTIVE, "America/New_York");
        assertNotEquals(a.tenantId(), b.tenantId());

        Site siteA = sites.create(a.tenantId(), null, "HQ", "A HQ", "UTC");
        Site siteB = sites.create(b.tenantId(), null, "HQ", "B HQ", "UTC");
        assertEquals("HQ", siteA.name());
        assertEquals("HQ", siteB.name());

        EntityGroup groupA = groups.create(a.tenantId(), null, "Routers", null);
        EntityGroup groupB = groups.create(b.tenantId(), null, "Routers", null);
        Tag tagA = tags.create(a.tenantId(), "env", "prod");
        Tag tagB = tags.create(b.tenantId(), "env", "prod");

        Entity entityA = entities.create(
                a.tenantId(), "NETWORK_DEVICE", "Edge-A", "desc", "discovered-a",
                LifecycleState.ACTIVE, siteA.siteId(), "HIGH", null, null, null, "tester",
                List.of(new EntityService.IdentifierInput("IP", "default", "10.10.10.10", new BigDecimal("1.00000"), null, null))
        );
        Entity entityB = entities.create(
                b.tenantId(), "NETWORK_DEVICE", "Edge-B", null, null,
                LifecycleState.ACTIVE, siteB.siteId(), null, null, null, null, "tester",
                List.of(
                        new EntityService.IdentifierInput("IP", "default", "10.10.10.10", new BigDecimal("0.90000"), null, null),
                        new EntityService.IdentifierInput("IP", "default", "2001:db8::10", new BigDecimal("1.00000"), null, null)
                )
        );
        entities.addIdentifier(
                a.tenantId(), entityA.entityId(),
                new EntityService.IdentifierInput("IP", "default", "2001:db8::10", new BigDecimal("1.00000"), null, null)
        );

        DomainException dup = assertThrows(DomainException.class, () -> entities.addIdentifier(
                a.tenantId(), entityA.entityId(),
                new EntityService.IdentifierInput("IP", "default", "10.10.10.10", new BigDecimal("1.00000"), null, null)
        ));
        assertEquals("IDENTIFIER_CONFLICT", dup.getCode());

        DomainException wrongTenantGet = assertThrows(DomainException.class,
                () -> entities.get(a.tenantId(), entityB.entityId()));
        assertEquals("ENTITY_NOT_FOUND", wrongTenantGet.getCode());

        DomainException wrongTenantUpdate = assertThrows(DomainException.class,
                () -> entities.update(a.tenantId(), entityB.entityId(), "x", "x", null, null, null, null, false,
                        null, null, false, null, null, null));
        assertEquals("ENTITY_NOT_FOUND", wrongTenantUpdate.getCode());

        PageResult<Entity> listA = entities.list(a.tenantId(), 1, 50);
        assertEquals(1, listA.items().size());
        assertEquals(entityA.entityId(), listA.items().get(0).entityId());

        assertThrows(DomainException.class, () -> sites.get(a.tenantId(), siteB.siteId()));
        assertThrows(DomainException.class, () -> groups.get(a.tenantId(), groupB.groupId()));
        assertThrows(DomainException.class, () -> tags.get(a.tenantId(), tagB.tagId()));

        // cross-tenant membership attempts fail as not-found of foreign resource
        assertThrows(DomainException.class, () -> entities.assignGroup(a.tenantId(), entityA.entityId(), groupB.groupId()));
        assertThrows(DomainException.class, () -> entities.assignTag(a.tenantId(), entityA.entityId(), tagB.tagId()));

        entities.assignGroup(a.tenantId(), entityA.entityId(), groupA.groupId());
        entities.assignTag(a.tenantId(), entityA.entityId(), tagA.tagId());

        // hierarchy cycle rejection
        Site child = sites.create(a.tenantId(), siteA.siteId(), "Child", null, null);
        DomainException cycle2 = assertThrows(DomainException.class,
                () -> sites.update(a.tenantId(), siteA.siteId(), child.siteId(), true, "HQ", null, null));
        assertEquals("HIERARCHY_CYCLE", cycle2.getCode());

        EntityGroup gChild = groups.create(a.tenantId(), groupA.groupId(), "ChildGroup", null);
        DomainException gCycle = assertThrows(DomainException.class,
                () -> groups.update(a.tenantId(), groupA.groupId(), gChild.groupId(), true, "Routers", null));
        assertEquals("HIERARCHY_CYCLE", gCycle.getCode());

        // entity without IP is legal
        Entity noIp = entities.create(
                a.tenantId(), "SERVICE", "svc-1", null, null, LifecycleState.DISCOVERED,
                null, null, null, null, null, "tester", List.of()
        );
        assertTrue(entities.listIdentifiers(a.tenantId(), noIp.entityId(), 1, 50).items().isEmpty());

        // identifier lookup tenant scoped
        assertEquals(entityA.entityId(),
                entities.resolveIdentifier(a.tenantId(), "default", "IP", "10.10.10.10").entityId());
        assertEquals(entityB.entityId(),
                entities.resolveIdentifier(b.tenantId(), "default", "IP", "10.10.10.10").entityId());

        // pagination does not leak
        PageResult<Entity> pageA = entities.list(a.tenantId(), 1, 1);
        assertEquals(1, pageA.items().size());
        assertTrue(pageA.items().stream().noneMatch(e -> e.tenantId().equals(b.tenantId())));
    }

    @Test
    void databaseRejectsCrossTenantFkEvenIfServiceBypassed() {
        Tenant a = tenants.create("A", TenantMode.SINGLE, TenantStatus.ACTIVE, "UTC");
        Tenant b = tenants.create("B", TenantMode.SINGLE, TenantStatus.ACTIVE, "UTC");
        Site siteB = sites.create(b.tenantId(), null, "B-Site", null, null);
        Entity entityA = entities.create(
                a.tenantId(), "VM", "vm-a", null, null, LifecycleState.ACTIVE,
                null, null, null, null, null, "tester", List.of()
        );
        assertThrows(Exception.class, () -> pg.jdbc().update(
                "UPDATE platform.entity SET site_id=? WHERE entity_id=?",
                siteB.siteId(), entityA.entityId()
        ));
        Entity entityB = entities.create(b.tenantId(), "DATABASE", "db-b", null, null, LifecycleState.ACTIVE,
                null, null, null, null, null, "tester", List.of());
        assertThrows(Exception.class, () -> pg.jdbc().update(
                """
                INSERT INTO platform.entity_identifier(identifier_id,tenant_id,entity_id,identifier_type,namespace,
                  identifier_value,confidence,first_seen_at,last_seen_at)
                VALUES (?::uuid, ?, ?, 'SERIAL', 'default', 'SN-1', 1.0, NOW(), NOW())
                """,
                UUID.randomUUID(), a.tenantId(), entityB.entityId()
        ));
    }
}
