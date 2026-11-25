package com.platform.core.persistence;

import org.junit.jupiter.api.Test;
import org.mockito.ArgumentCaptor;
import org.springframework.jdbc.core.RowMapper;
import org.springframework.jdbc.core.namedparam.MapSqlParameterSource;
import org.springframework.jdbc.core.namedparam.NamedParameterJdbcTemplate;

import java.sql.Timestamp;
import java.sql.Types;
import java.time.Instant;
import java.util.Map;
import java.util.UUID;

import static org.junit.jupiter.api.Assertions.assertEquals;
import static org.junit.jupiter.api.Assertions.assertThrows;
import static org.mockito.ArgumentMatchers.any;
import static org.mockito.ArgumentMatchers.eq;
import static org.mockito.Mockito.mock;
import static org.mockito.Mockito.verify;
import static org.mockito.Mockito.verifyNoInteractions;
import static org.mockito.Mockito.verifyNoMoreInteractions;

class EntityRepositoryKeysetTest {

    private static final UUID TENANT_ID = UUID.fromString("10000000-0000-0000-0000-000000000001");
    private static final UUID AFTER_ENTITY_ID = UUID.fromString("80000000-0000-0000-0000-000000000002");
    private static final Instant AFTER_CREATED_AT = Instant.parse("2026-11-25T04:05:06.123456Z");

    private final NamedParameterJdbcTemplate jdbc = mock(NamedParameterJdbcTemplate.class);
    private final EntityRepository repository = new EntityRepository(jdbc);

    @Test
    void firstPageBindsOnlyTenantAndLimit() {
        repository.listByTenantAfter(TENANT_ID, 1, null, null);

        ArgumentCaptor<MapSqlParameterSource> parameters = ArgumentCaptor.forClass(MapSqlParameterSource.class);
        verify(jdbc).query(eq("""
                SELECT * FROM platform.entity
                 WHERE tenant_id = :tenantId
                 ORDER BY created_at ASC, entity_id ASC
                 LIMIT :fetchLimit
                """), parameters.capture(), any(RowMapper.class));

        assertEquals(Map.of("tenantId", TENANT_ID, "fetchLimit", 1), parameters.getValue().getValues());
        assertEquals(Types.OTHER, parameters.getValue().getSqlType("tenantId"));
        assertEquals(Types.INTEGER, parameters.getValue().getSqlType("fetchLimit"));
        verifyNoMoreInteractions(jdbc);
    }

    @Test
    void continuationUsesStrictTupleComparisonToBreakTiesAtTheSameTimestamp() {
        repository.listByTenantAfter(TENANT_ID, 201, AFTER_CREATED_AT, AFTER_ENTITY_ID);

        ArgumentCaptor<MapSqlParameterSource> parameters = ArgumentCaptor.forClass(MapSqlParameterSource.class);
        verify(jdbc).query(eq("""
                SELECT * FROM platform.entity
                 WHERE tenant_id = :tenantId
                   AND (created_at, entity_id) > (:afterCreatedAt, :afterEntityId)
                 ORDER BY created_at ASC, entity_id ASC
                 LIMIT :fetchLimit
                """), parameters.capture(), any(RowMapper.class));

        MapSqlParameterSource bound = parameters.getValue();
        assertEquals(Map.of(
                "tenantId", TENANT_ID,
                "fetchLimit", 201,
                "afterCreatedAt", Timestamp.from(AFTER_CREATED_AT),
                "afterEntityId", AFTER_ENTITY_ID), bound.getValues());
        assertEquals(Types.OTHER, bound.getSqlType("tenantId"));
        assertEquals(Types.INTEGER, bound.getSqlType("fetchLimit"));
        assertEquals(Types.TIMESTAMP, bound.getSqlType("afterCreatedAt"));
        assertEquals(Types.OTHER, bound.getSqlType("afterEntityId"));
        verifyNoMoreInteractions(jdbc);
    }

    @Test
    void rejectsEitherPartialCursorTupleBeforeQuerying() {
        assertThrows(IllegalArgumentException.class,
                () -> repository.listByTenantAfter(TENANT_ID, 50, AFTER_CREATED_AT, null));
        assertThrows(IllegalArgumentException.class,
                () -> repository.listByTenantAfter(TENANT_ID, 50, null, AFTER_ENTITY_ID));
        verifyNoInteractions(jdbc);
    }

    @Test
    void rejectsNullTenantBeforeQuerying() {
        assertThrows(IllegalArgumentException.class,
                () -> repository.listByTenantAfter(null, 50, null, null));
        assertThrows(IllegalArgumentException.class,
                () -> repository.listByTenantAfter(null, 50, AFTER_CREATED_AT, AFTER_ENTITY_ID));
        verifyNoInteractions(jdbc);
    }

    @Test
    void rejectsFetchLimitsOutsideTheLookaheadRangeBeforeQuerying() {
        assertThrows(IllegalArgumentException.class,
                () -> repository.listByTenantAfter(TENANT_ID, 0, null, null));
        assertThrows(IllegalArgumentException.class,
                () -> repository.listByTenantAfter(TENANT_ID, 202, null, null));
        verifyNoInteractions(jdbc);
    }

    @Test
    void legacyListStillBindsLimitAndOffset() {
        repository.listByTenant(TENANT_ID, 25, 75L);

        verify(jdbc).query(eq("""
                SELECT * FROM platform.entity
                 WHERE tenant_id = :tenantId
                 ORDER BY created_at ASC, entity_id ASC
                 LIMIT :limit OFFSET :offset
                """), eq(Map.of("tenantId", TENANT_ID, "limit", 25, "offset", 75L)),
                any(RowMapper.class));
        verifyNoMoreInteractions(jdbc);
    }
}
