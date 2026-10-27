package com.platform.core.ingestion.authority;

import org.junit.jupiter.api.Test;

import java.util.Collections;
import java.util.HashSet;
import java.util.Iterator;
import java.util.Set;
import java.util.UUID;

import static org.junit.jupiter.api.Assertions.assertEquals;
import static org.junit.jupiter.api.Assertions.assertFalse;
import static org.junit.jupiter.api.Assertions.assertThrows;
import static org.junit.jupiter.api.Assertions.assertTrue;

class SensorScopeTest {

    private static final UUID TENANT = UUID.fromString("018f0e0a-0000-7000-8000-000000000001");
    private static final UUID SITE = UUID.fromString("018f0e0a-0000-7000-8000-000000000011");
    private static final UUID ENTITY_A = UUID.fromString("018f0e0a-0000-7000-8000-0000000000a1");
    private static final UUID ENTITY_B = UUID.fromString("018f0e0a-0000-7000-8000-0000000000b2");
    private static final UUID CREDENTIAL = UUID.fromString("018f0e0a-0000-7000-8000-0000000000c1");
    private static final String FINGERPRINT =
            "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef";

    @Test
    void addingToOriginalMutableSetCannotAuthorizeNewEntity() {
        Set<UUID> source = new HashSet<>();
        source.add(ENTITY_A);
        SensorScope scope = new SensorScope("sensor-net-01", TENANT, SITE, source, false);

        source.add(ENTITY_B);

        assertTrue(scope.isEntityAllowed(ENTITY_A));
        assertFalse(scope.isEntityAllowed(ENTITY_B));
        assertEquals(Set.of(ENTITY_A), scope.allowedEntityIds());
    }

    @Test
    void removingOrClearingOriginalSetCannotRemoveCapturedMembership() {
        Set<UUID> source = new HashSet<>();
        source.add(ENTITY_A);
        source.add(ENTITY_B);
        SensorScope scope = new SensorScope("sensor-net-01", TENANT, null, source, false);

        source.remove(ENTITY_A);
        assertTrue(scope.isEntityAllowed(ENTITY_A));
        assertTrue(scope.isEntityAllowed(ENTITY_B));

        source.clear();
        assertTrue(scope.isEntityAllowed(ENTITY_A));
        assertTrue(scope.isEntityAllowed(ENTITY_B));
        assertEquals(Set.of(ENTITY_A, ENTITY_B), scope.allowedEntityIds());
    }

    @Test
    void unmodifiableViewOverMutableBackingSetIsAlsoSnapshotted() {
        Set<UUID> backing = new HashSet<>();
        backing.add(ENTITY_A);
        Set<UUID> view = Collections.unmodifiableSet(backing);
        SensorScope scope = new SensorScope("sensor-net-01", TENANT, SITE, view, false);

        backing.add(ENTITY_B);
        backing.remove(ENTITY_A);

        assertTrue(scope.isEntityAllowed(ENTITY_A));
        assertFalse(scope.isEntityAllowed(ENTITY_B));
        assertEquals(Set.of(ENTITY_A), scope.allowedEntityIds());
    }

    @Test
    void exposedSetCannotBeMutatedIncludingThroughIterator() {
        SensorScope scope = new SensorScope(
                "sensor-net-01", TENANT, null, Set.of(ENTITY_A), false);

        assertThrows(UnsupportedOperationException.class,
                () -> scope.allowedEntityIds().add(ENTITY_B));
        assertThrows(UnsupportedOperationException.class,
                () -> scope.allowedEntityIds().remove(ENTITY_A));
        assertThrows(UnsupportedOperationException.class,
                scope.allowedEntityIds()::clear);

        Iterator<UUID> iterator = scope.allowedEntityIds().iterator();
        assertTrue(iterator.hasNext());
        iterator.next();
        assertThrows(UnsupportedOperationException.class, iterator::remove);

        assertEquals(Set.of(ENTITY_A), scope.allowedEntityIds());
        assertTrue(scope.isEntityAllowed(ENTITY_A));
        assertFalse(scope.isEntityAllowed(ENTITY_B));
    }

    @Test
    void resolvedSensorAuthorityRetainsCapturedMembershipWhenSourceSetChanges() {
        Set<UUID> source = new HashSet<>();
        source.add(ENTITY_A);
        SensorScope scope = new SensorScope("sensor-net-01", TENANT, SITE, source, false);
        ResolvedSensorAuthority authority = new ResolvedSensorAuthority(scope, CREDENTIAL, FINGERPRINT);

        source.add(ENTITY_B);
        source.remove(ENTITY_A);

        assertTrue(authority.scope().isEntityAllowed(ENTITY_A));
        assertFalse(authority.scope().isEntityAllowed(ENTITY_B));
        assertEquals(Set.of(ENTITY_A), authority.scope().allowedEntityIds());
    }

    @Test
    void nullAllowedEntityIdsMeansEmptySet() {
        SensorScope scope = new SensorScope("sensor-net-01", TENANT, null, null, false);

        assertEquals(Set.of(), scope.allowedEntityIds());
        assertFalse(scope.isEntityAllowed(ENTITY_A));
        assertThrows(UnsupportedOperationException.class,
                () -> scope.allowedEntityIds().add(ENTITY_A));
    }

    @Test
    void allowAllTenantEntitiesIgnoresExplicitMembership() {
        SensorScope scope = new SensorScope(
                "sensor-net-01", TENANT, SITE, Set.of(ENTITY_A), true);

        assertTrue(scope.isEntityAllowed(ENTITY_A));
        assertTrue(scope.isEntityAllowed(ENTITY_B));
        assertTrue(scope.allowAllTenantEntities());
    }

    @Test
    void nullEntityIdIsRejected() {
        SensorScope scoped = new SensorScope(
                "sensor-net-01", TENANT, null, Set.of(ENTITY_A), false);
        SensorScope allowAll = new SensorScope(
                "sensor-net-01", TENANT, null, Set.of(), true);

        assertFalse(scoped.isEntityAllowed((UUID) null));
        assertFalse(allowAll.isEntityAllowed((UUID) null));
        assertFalse(scoped.isEntityAllowed((String) null));
        assertFalse(scoped.isEntityAllowed(""));
        assertFalse(scoped.isEntityAllowed("   "));
    }

    @Test
    void suppliedSetContainingNullElementIsAccepted() {
        Set<UUID> withNull = new HashSet<>();
        withNull.add(ENTITY_A);
        withNull.add(null);

        SensorScope scope = new SensorScope("sensor-net-01", TENANT, null, withNull, false);

        assertTrue(scope.allowedEntityIds().contains(null));
        assertTrue(scope.isEntityAllowed(ENTITY_A));
        assertFalse(scope.isEntityAllowed(ENTITY_B));
        assertEquals(2, scope.allowedEntityIds().size());
    }

    @Test
    void stringUuidTrimmingAndMalformedInputBehaviorUnchanged() {
        SensorScope scope = new SensorScope(
                "sensor-net-01", TENANT, null, Set.of(ENTITY_A), false);

        assertTrue(scope.isEntityAllowed("  " + ENTITY_A + "  "));
        assertFalse(scope.isEntityAllowed(ENTITY_B.toString()));
        assertFalse(scope.isEntityAllowed("not-a-uuid"));
        assertFalse(scope.isEntityAllowed("018f0e0a-0000-7000-8000-0000000000zz"));
    }

    @Test
    void sensorIdAndTenantIdRemainMandatoryWhileSiteIdIsOptional() {
        assertThrows(NullPointerException.class,
                () -> new SensorScope(null, TENANT, SITE, Set.of(), false));
        assertThrows(NullPointerException.class,
                () -> new SensorScope("sensor-net-01", null, SITE, Set.of(), false));

        SensorScope withoutSite = new SensorScope(
                "sensor-net-01", TENANT, null, Set.of(ENTITY_A), false);
        assertEquals(null, withoutSite.siteId());
        assertEquals("sensor-net-01", withoutSite.sensorId());
        assertEquals(TENANT, withoutSite.tenantId());
    }
}
