package com.platform.core.persistence;

import com.platform.core.domain.inventory.Entity;
import com.platform.core.domain.inventory.LifecycleState;
import com.platform.core.domain.tenant.TenantMode;
import com.platform.core.domain.tenant.TenantService;
import com.platform.core.domain.tenant.TenantStatus;
import com.platform.core.support.PostgresTestSupport;
import org.junit.jupiter.api.AfterEach;
import org.junit.jupiter.api.BeforeEach;
import org.junit.jupiter.api.Tag;
import org.junit.jupiter.api.Test;

import java.time.Instant;
import java.util.List;
import java.util.UUID;
import java.util.stream.Stream;

import static org.junit.jupiter.api.Assertions.assertEquals;
import static org.junit.jupiter.api.Assertions.assertThrows;
import static org.junit.jupiter.api.Assertions.assertTrue;

@Tag("postgres")
class EntityRepositoryKeysetIntegrationTest {

    private static final int PAGE_SIZE = 2;
    private static final int FETCH_LIMIT = PAGE_SIZE + 1;

    private static final Instant BEFORE_TIE = Instant.parse("2026-11-25T04:05:06.123455Z");
    private static final Instant TIE = Instant.parse("2026-11-25T04:05:06.123456Z");
    private static final Instant AFTER_TIE = Instant.parse("2026-11-25T04:05:06.123457Z");
    private static final Instant LAST = Instant.parse("2026-11-25T04:05:06.123458Z");

    private static final UUID FIRST_ID = UUID.fromString("f0000000-0000-0000-0000-000000000001");
    private static final UUID LOW_BIT_ID = UUID.fromString("7fffffff-ffff-ffff-ffff-ffffffffffff");
    private static final UUID HIGH_BIT_ID = UUID.fromString("80000000-0000-0000-0000-000000000000");
    private static final UUID NEXT_ID = UUID.fromString("10000000-0000-0000-0000-000000000004");
    private static final UUID LAST_ID = UUID.fromString("10000000-0000-0000-0000-000000000005");
    private static final UUID OTHER_TIE_ID = UUID.fromString("7fffffff-ffff-ffff-ffff-fffffffffffe");
    private static final UUID OTHER_LATER_ID = UUID.fromString("90000000-0000-0000-0000-000000000002");

    private PostgresTestSupport pg;
    private TenantService tenants;
    private EntityRepository entities;

    @BeforeEach
    void setUp() throws Exception {
        pg = new PostgresTestSupport();
        pg.start();
        pg.migrate();
        tenants = new TenantService(new TenantRepository(pg.namedJdbc()));
        entities = new EntityRepository(pg.namedJdbc());
    }

    @AfterEach
    void tearDown() {
        pg.stop();
    }

    @Test
    void pagesInPostgresTupleOrderAcrossExactMicrosecondBoundaries() {
        pg.tx().executeWithoutResult(transaction -> {
            transaction.setRollbackOnly();
            pg.jdbc().execute("SET LOCAL TIME ZONE 'UTC'");
            UUID tenantId = createTenant("Keyset primary");
            UUID otherTenantId = createTenant("Keyset other");
            UUID emptyTenantId = createTenant("Keyset empty");

            insert(tenantId, HIGH_BIT_ID, TIE);
            insert(otherTenantId, OTHER_LATER_ID, AFTER_TIE);
            insert(tenantId, LAST_ID, LAST);
            insert(tenantId, LOW_BIT_ID, TIE);
            insert(otherTenantId, OTHER_TIE_ID, TIE);
            insert(tenantId, NEXT_ID, AFTER_TIE);
            insert(tenantId, FIRST_ID, BEFORE_TIE);

            pg.jdbc().execute("SET LOCAL TIME ZONE 'Asia/Kolkata'");
            assertEquals("Asia/Kolkata", pg.jdbc().queryForObject("SHOW TimeZone", String.class));

            List<Entity> first = entities.listByTenantAfter(tenantId, FETCH_LIMIT, null, null);
            assertEquals(List.of(FIRST_ID, LOW_BIT_ID, HIGH_BIT_ID), ids(first));
            assertEquals(List.of(BEFORE_TIE, TIE, TIE), createdTimes(first));

            Entity firstCursor = first.get(PAGE_SIZE - 1);
            List<Entity> second = entities.listByTenantAfter(
                    tenantId, FETCH_LIMIT, firstCursor.createdAt(), firstCursor.entityId());
            assertEquals(List.of(HIGH_BIT_ID, NEXT_ID, LAST_ID), ids(second));
            assertEquals(List.of(TIE, AFTER_TIE, LAST), createdTimes(second));

            Entity secondCursor = second.get(PAGE_SIZE - 1);
            List<Entity> third = entities.listByTenantAfter(
                    tenantId, FETCH_LIMIT, secondCursor.createdAt(), secondCursor.entityId());
            assertEquals(List.of(LAST_ID), ids(third));
            assertEquals(List.of(LAST), createdTimes(third));

            List<Entity> emitted = Stream.of(first.subList(0, PAGE_SIZE), second.subList(0, PAGE_SIZE), third)
                    .flatMap(List::stream)
                    .toList();
            assertEquals(List.of(FIRST_ID, LOW_BIT_ID, HIGH_BIT_ID, NEXT_ID, LAST_ID), ids(emitted));
            assertTrue(emitted.stream().allMatch(row -> tenantId.equals(row.tenantId())));

            Entity lastCursor = third.getFirst();
            assertTrue(entities.listByTenantAfter(
                    tenantId, FETCH_LIMIT, lastCursor.createdAt(), lastCursor.entityId()).isEmpty());
            assertTrue(entities.listByTenantAfter(emptyTenantId, FETCH_LIMIT, null, null).isEmpty());

            List<Entity> otherRows = entities.listByTenantAfter(otherTenantId, FETCH_LIMIT, null, null);
            assertEquals(List.of(OTHER_TIE_ID, OTHER_LATER_ID), ids(otherRows));
            assertTrue(otherRows.stream().allMatch(row -> otherTenantId.equals(row.tenantId())));

            assertEquals(List.of(FIRST_ID), ids(entities.listByTenantAfter(tenantId, 1, null, null)));
            assertEquals(List.of(LOW_BIT_ID), ids(entities.listByTenantAfter(tenantId, 1, BEFORE_TIE, FIRST_ID)));
            assertEquals(ids(emitted), ids(entities.listByTenantAfter(tenantId, 201, null, null)));
        });
    }

    @Test
    void rejectsOutOfRangeLimitsAndPartialCursors() {
        pg.tx().executeWithoutResult(transaction -> {
            transaction.setRollbackOnly();
            UUID tenantId = createTenant("Keyset validation");

            assertThrows(IllegalArgumentException.class,
                    () -> entities.listByTenantAfter(tenantId, 0, null, null));
            assertThrows(IllegalArgumentException.class,
                    () -> entities.listByTenantAfter(tenantId, 202, null, null));
            assertThrows(IllegalArgumentException.class,
                    () -> entities.listByTenantAfter(tenantId, FETCH_LIMIT, TIE, null));
            assertThrows(IllegalArgumentException.class,
                    () -> entities.listByTenantAfter(tenantId, FETCH_LIMIT, null, LOW_BIT_ID));
        });
    }

    private UUID createTenant(String name) {
        return tenants.create(name, TenantMode.SINGLE, TenantStatus.ACTIVE, "UTC").tenantId();
    }

    private void insert(UUID tenantId, UUID entityId, Instant createdAt) {
        entities.insert(new Entity(
                entityId, tenantId, "DEVICE", entityId.toString(), null, null, LifecycleState.ACTIVE,
                null, null, null, null, null, createdAt, createdAt, null, createdAt, createdAt));
    }

    private static List<UUID> ids(List<Entity> rows) {
        return rows.stream().map(Entity::entityId).toList();
    }

    private static List<Instant> createdTimes(List<Entity> rows) {
        return rows.stream().map(Entity::createdAt).toList();
    }
}
