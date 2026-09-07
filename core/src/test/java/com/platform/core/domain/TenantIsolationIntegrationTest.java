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
import java.time.Instant;
import java.util.List;
import java.util.UUID;
import java.util.concurrent.CountDownLatch;
import java.util.concurrent.ExecutorService;
import java.util.concurrent.Executors;
import java.util.concurrent.Future;
import java.util.concurrent.TimeUnit;

import static org.junit.jupiter.api.Assertions.assertEquals;
import static org.junit.jupiter.api.Assertions.assertNotEquals;
import static org.junit.jupiter.api.Assertions.assertNull;
import static org.junit.jupiter.api.Assertions.assertThrows;
import static org.junit.jupiter.api.Assertions.assertTrue;

@org.junit.jupiter.api.Tag("postgres")
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

        // Wrong-tenant and foreign-resource DELETE paths do not disclose or mutate assignments.
        UUID identifierA = entities.resolveIdentifier(a.tenantId(), "default", "IP", "10.10.10.10").identifierId();
        DomainException wrongDelete = assertThrows(DomainException.class,
                () -> entities.removeIdentifier(b.tenantId(), entityA.entityId(), identifierA));
        assertEquals("ENTITY_NOT_FOUND", wrongDelete.getCode());
        assertThrows(DomainException.class,
                () -> entities.unassignGroup(a.tenantId(), entityA.entityId(), groupB.groupId()));
        assertThrows(DomainException.class,
                () -> entities.unassignTag(a.tenantId(), entityA.entityId(), tagB.tagId()));
        assertEquals(1, entities.listGroups(a.tenantId(), entityA.entityId(), 1, 50).items().size());
        assertEquals(1, entities.listTags(a.tenantId(), entityA.entityId(), 1, 50).items().size());

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

    @Test
    void concurrentHierarchyUpdatesCannotCreateSiteOrGroupCycles() throws Exception {
        Tenant tenant = tenants.create("Hierarchy", TenantMode.SINGLE, TenantStatus.ACTIVE, "UTC");

        Site siteA = sites.create(tenant.tenantId(), null, "Site A", null, null);
        Site siteB = sites.create(tenant.tenantId(), null, "Site B", null, null);
        List<String> siteResults = raceHierarchyUpdates(
                () -> sites.update(tenant.tenantId(), siteA.siteId(), siteB.siteId(), true, null, null, null),
                () -> sites.update(tenant.tenantId(), siteB.siteId(), siteA.siteId(), true, null, null, null)
        );
        assertEquals(1, siteResults.stream().filter("OK"::equals).count());
        assertEquals(1, siteResults.stream().filter("HIERARCHY_CYCLE"::equals).count());

        EntityGroup groupA = groups.create(tenant.tenantId(), null, "Group A", null);
        EntityGroup groupB = groups.create(tenant.tenantId(), null, "Group B", null);
        List<String> groupResults = raceHierarchyUpdates(
                () -> groups.update(tenant.tenantId(), groupA.groupId(), groupB.groupId(), true, null, null),
                () -> groups.update(tenant.tenantId(), groupB.groupId(), groupA.groupId(), true, null, null)
        );
        assertEquals(1, groupResults.stream().filter("OK"::equals).count());
        assertEquals(1, groupResults.stream().filter("HIERARCHY_CYCLE"::equals).count());
    }

    @Test
    void concurrentMultiLevelHierarchyUpdatesCannotCreateSiteOrGroupCycles() throws Exception {
        Tenant tenant = tenants.create("MultiHierarchy", TenantMode.SINGLE, TenantStatus.ACTIVE, "UTC");

        // Chain 1: Site A -> Site B; Chain 2: Site C -> Site D
        Site siteA = sites.create(tenant.tenantId(), null, "Site A", null, null);
        Site siteB = sites.create(tenant.tenantId(), siteA.siteId(), "Site B", null, null);
        Site siteC = sites.create(tenant.tenantId(), null, "Site C", null, null);
        Site siteD = sites.create(tenant.tenantId(), siteC.siteId(), "Site D", null, null);

        // Thread 1 connects Chain 2 to Chain 1 (D -> A -> B)
        // Thread 2 connects Chain 1 to Chain 2 (B -> C -> D)
        // If both were allowed to commit, C -> D -> A -> B -> C cycle would form
        List<String> siteResults = raceHierarchyUpdates(
                () -> sites.update(tenant.tenantId(), siteA.siteId(), siteD.siteId(), true, null, null, null),
                () -> sites.update(tenant.tenantId(), siteC.siteId(), siteB.siteId(), true, null, null, null)
        );
        assertEquals(1, siteResults.stream().filter("OK"::equals).count());
        assertEquals(1, siteResults.stream().filter("HIERARCHY_CYCLE"::equals).count());

        // Same multi-level check for Entity Groups: G1 -> G2 and G3 -> G4
        EntityGroup groupA = groups.create(tenant.tenantId(), null, "Group A", null);
        EntityGroup groupB = groups.create(tenant.tenantId(), groupA.groupId(), "Group B", null);
        EntityGroup groupC = groups.create(tenant.tenantId(), null, "Group C", null);
        EntityGroup groupD = groups.create(tenant.tenantId(), groupC.groupId(), "Group D", null);

        List<String> groupResults = raceHierarchyUpdates(
                () -> groups.update(tenant.tenantId(), groupA.groupId(), groupD.groupId(), true, null, null),
                () -> groups.update(tenant.tenantId(), groupC.groupId(), groupB.groupId(), true, null, null)
        );
        assertEquals(1, groupResults.stream().filter("OK"::equals).count());
        assertEquals(1, groupResults.stream().filter("HIERARCHY_CYCLE"::equals).count());
    }

    @Test
    void tagNullValueUniquenessEnforcesLabelDeduplicationPerTenant() {
        Tenant a = tenants.create("TenantTagA", TenantMode.SINGLE, TenantStatus.ACTIVE, "UTC");
        Tenant b = tenants.create("TenantTagB", TenantMode.SINGLE, TenantStatus.ACTIVE, "UTC");

        // 1. TagService: ("environment", null) succeeds in Tenant A
        Tag tagA1 = tags.create(a.tenantId(), "environment", null);
        assertEquals("environment", tagA1.tagKey());
        assertNull(tagA1.tagValue());

        // 2. TagService: second ("environment", null) in Tenant A must fail with RESOURCE_CONFLICT
        DomainException conflictA = assertThrows(DomainException.class,
                () -> tags.create(a.tenantId(), "environment", null));
        assertEquals("RESOURCE_CONFLICT", conflictA.getCode());

        // 3. TagService: identical ("environment", null) in Tenant B is allowed
        Tag tagB1 = tags.create(b.tenantId(), "environment", null);
        assertEquals("environment", tagB1.tagKey());
        assertNull(tagB1.tagValue());

        // 4. TagService: ("environment", "production") is distinct from null and succeeds in Tenant A
        Tag tagA2 = tags.create(a.tenantId(), "environment", "production");
        assertEquals("production", tagA2.tagValue());

        // 5. TagService: second ("environment", "production") in Tenant A must fail
        assertThrows(DomainException.class,
                () -> tags.create(a.tenantId(), "environment", "production"));

        // 6. Direct SQL constraint test on uq_tag_tenant_key_null_value
        pg.jdbc().update("""
                INSERT INTO platform.tag (tag_id, tenant_id, tag_key, tag_value, created_at)
                VALUES (?::uuid, ?::uuid, 'sql-label', NULL, NOW())
                """, UUID.randomUUID().toString(), a.tenantId().toString());

        assertThrows(Exception.class, () -> pg.jdbc().update("""
                INSERT INTO platform.tag (tag_id, tenant_id, tag_key, tag_value, created_at)
                VALUES (?::uuid, ?::uuid, 'sql-label', NULL, NOW())
                """, UUID.randomUUID().toString(), a.tenantId().toString()));

        // Allowed for Tenant B
        assertEquals(1, pg.jdbc().update("""
                INSERT INTO platform.tag (tag_id, tenant_id, tag_key, tag_value, created_at)
                VALUES (?::uuid, ?::uuid, 'sql-label', NULL, NOW())
                """, UUID.randomUUID().toString(), b.tenantId().toString()));

        // 7. Entity Identifier non-nullable uniqueness verification
        Entity entityA1 = entities.create(a.tenantId(), "NETWORK_DEVICE", "dev-A1", null, null,
                LifecycleState.ACTIVE, null, null, null, null, null, "tester", List.of());
        Entity entityA2 = entities.create(a.tenantId(), "NETWORK_DEVICE", "dev-A2", null, null,
                LifecycleState.ACTIVE, null, null, null, null, null, "tester", List.of());
        Entity entityB1 = entities.create(b.tenantId(), "NETWORK_DEVICE", "dev-B1", null, null,
                LifecycleState.ACTIVE, null, null, null, null, null, "tester", List.of());

        entities.addIdentifier(a.tenantId(), entityA1.entityId(),
                new EntityService.IdentifierInput("IP", "default", "192.168.1.1", BigDecimal.ONE, Instant.now(), Instant.now()));

        DomainException idConflict = assertThrows(DomainException.class,
                () -> entities.addIdentifier(a.tenantId(), entityA2.entityId(),
                        new EntityService.IdentifierInput("IP", "default", "192.168.1.1", BigDecimal.ONE, Instant.now(), Instant.now())));
        assertEquals("IDENTIFIER_CONFLICT", idConflict.getCode());

        // Cross-tenant same identifier is allowed
        entities.addIdentifier(b.tenantId(), entityB1.entityId(),
                new EntityService.IdentifierInput("IP", "default", "192.168.1.1", BigDecimal.ONE, Instant.now(), Instant.now()));
    }

    @Test
    void entityAttributePayloadIsBounded() {
        Tenant tenant = tenants.create("Attributes", TenantMode.SINGLE, TenantStatus.ACTIVE, "UTC");
        Entity entity = entities.create(
                tenant.tenantId(), "SERVICE", "bounded", null, null, LifecycleState.ACTIVE,
                null, null, null, null, null, "tester", List.of());

        entities.upsertAttribute(
                tenant.tenantId(), entity.entityId(), "metadata", "{\"vendor\":\"acme\"}", "api", null);
        assertEquals("acme", pg.jdbc().queryForObject(
                "SELECT value_json->>'vendor' FROM platform.entity_attribute WHERE tenant_id=? AND entity_id=?",
                String.class, tenant.tenantId(), entity.entityId()));

        DomainException oversized = assertThrows(DomainException.class, () -> entities.upsertAttribute(
                tenant.tenantId(), entity.entityId(), "metadata", "x".repeat(65_537), "api", null));
        assertEquals("VALIDATION_FAILED", oversized.getCode());
    }

    @Test
    void identifierInputsEnforceBoundsOrderingAndRequiredNamespace() {
        Tenant tenant = tenants.create("Identifiers", TenantMode.SINGLE, TenantStatus.ACTIVE, "UTC");
        Entity entity = entities.create(
                tenant.tenantId(), "NETWORK_DEVICE", "device", null, null, LifecycleState.ACTIVE,
                null, null, null, null, null, "tester", List.of());
        Instant now = Instant.now();

        assertValidationFailure(() -> entities.addIdentifier(tenant.tenantId(), entity.entityId(),
                new EntityService.IdentifierInput("IP", null, "10.0.0.1", BigDecimal.ONE, now, now)));
        assertValidationFailure(() -> entities.addIdentifier(tenant.tenantId(), entity.entityId(),
                new EntityService.IdentifierInput("IP", "default", "bad\u0007value", BigDecimal.ONE, now, now)));
        assertValidationFailure(() -> entities.addIdentifier(tenant.tenantId(), entity.entityId(),
                new EntityService.IdentifierInput("IP", "default", "x".repeat(513), BigDecimal.ONE, now, now)));
        assertValidationFailure(() -> entities.addIdentifier(tenant.tenantId(), entity.entityId(),
                new EntityService.IdentifierInput("IP", "default", "10.0.0.2", new BigDecimal("1.00001"), now, now)));
        assertValidationFailure(() -> entities.addIdentifier(tenant.tenantId(), entity.entityId(),
                new EntityService.IdentifierInput("IP", "default", "10.0.0.3", BigDecimal.ONE,
                        now, now.minusSeconds(1))));
    }

    private List<String> raceHierarchyUpdates(Runnable first, Runnable second) throws Exception {
        ExecutorService pool = Executors.newFixedThreadPool(2);
        CountDownLatch ready = new CountDownLatch(2);
        CountDownLatch start = new CountDownLatch(1);
        try {
            Future<String> firstResult = pool.submit(() -> runHierarchyUpdate(first, ready, start));
            Future<String> secondResult = pool.submit(() -> runHierarchyUpdate(second, ready, start));
            assertTrue(ready.await(5, TimeUnit.SECONDS));
            start.countDown();
            return List.of(firstResult.get(10, TimeUnit.SECONDS), secondResult.get(10, TimeUnit.SECONDS));
        } finally {
            pool.shutdownNow();
        }
    }

    private String runHierarchyUpdate(Runnable update, CountDownLatch ready, CountDownLatch start) {
        ready.countDown();
        try {
            if (!start.await(5, TimeUnit.SECONDS)) {
                throw new IllegalStateException("Hierarchy race start barrier timed out");
            }
            pg.tx().executeWithoutResult(status -> update.run());
            return "OK";
        } catch (DomainException ex) {
            return ex.getCode();
        } catch (InterruptedException ex) {
            Thread.currentThread().interrupt();
            throw new IllegalStateException("Hierarchy race interrupted", ex);
        }
    }

    private static void assertValidationFailure(Runnable action) {
        DomainException failure = assertThrows(DomainException.class, action::run);
        assertEquals("VALIDATION_FAILED", failure.getCode());
    }
}
