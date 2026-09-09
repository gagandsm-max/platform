package com.platform.core.domain.tenant;

import com.platform.core.api.PageResult;
import com.platform.core.persistence.TenantRepository;
import com.platform.core.support.PostgresTestSupport;
import org.junit.jupiter.api.AfterEach;
import org.junit.jupiter.api.BeforeEach;
import org.junit.jupiter.api.Tag;
import org.junit.jupiter.api.Test;

import java.util.List;
import java.util.Set;
import java.util.UUID;
import java.util.stream.Collectors;

import static org.junit.jupiter.api.Assertions.assertEquals;
import static org.junit.jupiter.api.Assertions.assertFalse;
import static org.junit.jupiter.api.Assertions.assertTrue;

@Tag("postgres")
class TenantScopedListIntegrationTest {

    private PostgresTestSupport pg;
    private TenantService tenants;
    private TenantRepository tenantRepo;

    @BeforeEach
    void setUp() throws Exception {
        pg = new PostgresTestSupport();
        pg.start();
        pg.migrate();
        tenantRepo = new TenantRepository(pg.namedJdbc());
        tenants = new TenantService(tenantRepo);
    }

    @AfterEach
    void tearDown() {
        if (pg != null) {
            pg.stop();
        }
    }

    @Test
    void scopedListAppliesTenantFilterBeforePagination() {
        Tenant a = tenants.create("A", TenantMode.SINGLE, TenantStatus.ACTIVE, "UTC");
        Tenant b = tenants.create("B", TenantMode.SINGLE, TenantStatus.ACTIVE, "UTC");
        Tenant c = tenants.create("C", TenantMode.SINGLE, TenantStatus.ACTIVE, "UTC");
        // Extra global tenants that would occupy early global pages.
        for (int i = 0; i < 5; i++) {
            tenants.create("Padding-" + i, TenantMode.SINGLE, TenantStatus.ACTIVE, "UTC");
        }

        PageResult<Tenant> page1 = tenants.listScoped(Set.of(a.tenantId(), b.tenantId()), 1, 1);
        assertEquals(1, page1.items().size());
        assertEquals(2L, page1.total());
        assertTrue(page1.hasMore());
        assertEquals(a.tenantId(), page1.items().getFirst().tenantId());

        PageResult<Tenant> page2 = tenants.listScoped(Set.of(a.tenantId(), b.tenantId()), 2, 1);
        assertEquals(1, page2.items().size());
        assertEquals(2L, page2.total());
        assertFalse(page2.hasMore());
        assertEquals(b.tenantId(), page2.items().getFirst().tenantId());

        Set<UUID> visible = tenants.listScoped(Set.of(a.tenantId(), b.tenantId()), 1, 50)
                .items().stream().map(Tenant::tenantId).collect(Collectors.toSet());
        assertEquals(Set.of(a.tenantId(), b.tenantId()), visible);
        assertFalse(visible.contains(c.tenantId()));
    }

    @Test
    void authorizedTenantOutsideGlobalFirstPageStillReturnedWhenScoped() {
        // Create many tenants so C would fall outside a global page of size 2.
        Tenant early1 = tenants.create("Early-1", TenantMode.SINGLE, TenantStatus.ACTIVE, "UTC");
        Tenant early2 = tenants.create("Early-2", TenantMode.SINGLE, TenantStatus.ACTIVE, "UTC");
        Tenant lateScoped = tenants.create("Late-Scoped", TenantMode.SINGLE, TenantStatus.ACTIVE, "UTC");

        PageResult<Tenant> globalFirst = tenants.list(1, 2);
        assertEquals(2, globalFirst.items().size());
        assertFalse(globalFirst.items().stream().anyMatch(t -> t.tenantId().equals(lateScoped.tenantId())));

        PageResult<Tenant> scoped = tenants.listScoped(Set.of(lateScoped.tenantId()), 1, 2);
        assertEquals(1, scoped.items().size());
        assertEquals(1L, scoped.total());
        assertFalse(scoped.hasMore());
        assertEquals(lateScoped.tenantId(), scoped.items().getFirst().tenantId());
        assertFalse(scoped.items().stream().anyMatch(t ->
                t.tenantId().equals(early1.tenantId()) || t.tenantId().equals(early2.tenantId())));
    }

    @Test
    void emptyScopeFailsClosed() {
        tenants.create("Only", TenantMode.SINGLE, TenantStatus.ACTIVE, "UTC");
        PageResult<Tenant> empty = tenants.listScoped(Set.of(), 1, 50);
        assertTrue(empty.items().isEmpty());
        assertEquals(0L, empty.total());
        assertFalse(empty.hasMore());

        assertEquals(0L, tenantRepo.countByIds(List.of()));
        assertTrue(tenantRepo.listByIds(List.of(), 50, 0).isEmpty());
    }

    @Test
    void pageLimitBoundariesEnforced() {
        Tenant a = tenants.create("Bound-A", TenantMode.SINGLE, TenantStatus.ACTIVE, "UTC");
        org.junit.jupiter.api.Assertions.assertThrows(
                com.platform.core.domain.DomainException.class,
                () -> tenants.listScoped(Set.of(a.tenantId()), 0, 50)
        );
        org.junit.jupiter.api.Assertions.assertThrows(
                com.platform.core.domain.DomainException.class,
                () -> tenants.listScoped(Set.of(a.tenantId()), 1, 0)
        );
        org.junit.jupiter.api.Assertions.assertThrows(
                com.platform.core.domain.DomainException.class,
                () -> tenants.listScoped(Set.of(a.tenantId()), 1, PageResult.MAX_LIMIT + 1)
        );
    }
}
