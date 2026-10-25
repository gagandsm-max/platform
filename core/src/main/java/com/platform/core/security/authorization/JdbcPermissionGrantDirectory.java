package com.platform.core.security.authorization;

import com.platform.core.security.http.SecurityDirectoryConfiguration;
import com.platform.core.security.http.SecurityDirectoryProperties;
import com.platform.core.security.identity.PlatformSubject;
import org.springframework.beans.factory.annotation.Qualifier;
import org.springframework.dao.QueryTimeoutException;
import org.springframework.jdbc.core.JdbcTemplate;
import org.springframework.transaction.PlatformTransactionManager;
import org.springframework.transaction.TransactionDefinition;
import org.springframework.transaction.support.TransactionTemplate;

import java.sql.PreparedStatement;
import java.sql.SQLException;
import java.time.Duration;
import java.util.HashMap;
import java.util.HashSet;
import java.util.LinkedHashSet;
import java.util.List;
import java.util.Map;
import java.util.Objects;
import java.util.Set;
import java.util.UUID;

/**
 * PostgreSQL-backed {@link PermissionGrantDirectory}.
 *
 * <p>Membership and grants are loaded inside one locally scoped, read-only
 * {@code REPEATABLE_READ} transaction on the named PostgreSQL {@code transactionManager}
 * (never Kafka), using {@code PROPAGATION_REQUIRES_NEW} so an ambient weaker/stale
 * transaction cannot silently override the snapshot guarantee.
 *
 * <p>Snapshot boundary: both membership and grant/resource queries observe the same
 * committed database state as of the start of that transaction. Later-load boundary:
 * a subsequent {@link #load} begun after a committed revocation observes the new state
 * (no cross-request cache). This does not cancel authorization already evaluated for
 * an in-flight request.
 *
 * <p>Joined grant/resource rows are bounded in SQL ({@code LIMIT max+1}) before the
 * driver materializes an unbounded result list. Aggregate overflow fails the whole load.
 */
public final class JdbcPermissionGrantDirectory implements PermissionGrantDirectory {

    private final JdbcTemplate jdbcTemplate;
    private final PlatformTransactionManager transactionManager;
    private final Duration queryTimeout;
    private final int maxTenantScopes;
    private final int maxPermissionGrants;
    private final int maxNamedResourcesPerGrant;
    private final int maxMaterializedAuthorityRows;

    public JdbcPermissionGrantDirectory(
            JdbcTemplate jdbcTemplate,
            @Qualifier("transactionManager") PlatformTransactionManager transactionManager,
            SecurityDirectoryProperties directoryProperties
    ) {
        this.jdbcTemplate = Objects.requireNonNull(jdbcTemplate, "jdbcTemplate");
        this.transactionManager = Objects.requireNonNull(transactionManager, "transactionManager");
        Objects.requireNonNull(directoryProperties, "directoryProperties");
        directoryProperties.requireValidEnabledConfiguration();
        this.queryTimeout = directoryProperties.getQueryTimeout();
        this.maxTenantScopes = directoryProperties.getMaxTenantScopes();
        this.maxPermissionGrants = directoryProperties.getMaxPermissionGrants();
        this.maxNamedResourcesPerGrant = directoryProperties.getMaxNamedResourcesPerGrant();
        this.maxMaterializedAuthorityRows = directoryProperties.getMaxMaterializedAuthorityRows();
    }

    @Override
    public PrincipalAuthorization load(PlatformSubject subject) {
        Objects.requireNonNull(subject, "subject");
        try {
            // Fresh REQUIRES_NEW + REPEATABLE_READ template per load: ambient transactions cannot
            // dilute isolation, and no authorization state is cached across requests.
            TransactionTemplate tx = newAuthorizationSnapshotTemplate();
            PrincipalAuthorization loaded = tx.execute(status -> loadInTransaction(subject));
            if (loaded == null) {
                throw new SecurityDirectoryConfiguration.DirectoryFailureException(
                        "SECURITY_DIRECTORY_UNAVAILABLE");
            }
            return loaded;
        } catch (SecurityDirectoryConfiguration.DirectoryFailureException ex) {
            throw ex;
        } catch (QueryTimeoutException ex) {
            throw new SecurityDirectoryConfiguration.DirectoryFailureException(
                    "SECURITY_DIRECTORY_TIMEOUT", ex);
        } catch (RuntimeException ex) {
            if (isStatementTimeout(ex)) {
                throw new SecurityDirectoryConfiguration.DirectoryFailureException(
                        "SECURITY_DIRECTORY_TIMEOUT", ex);
            }
            throw new SecurityDirectoryConfiguration.DirectoryFailureException(
                    "SECURITY_DIRECTORY_UNAVAILABLE", ex);
        }
    }

    private TransactionTemplate newAuthorizationSnapshotTemplate() {
        TransactionTemplate tx = new TransactionTemplate(transactionManager);
        tx.setReadOnly(true);
        tx.setPropagationBehavior(TransactionDefinition.PROPAGATION_REQUIRES_NEW);
        tx.setIsolationLevel(TransactionDefinition.ISOLATION_REPEATABLE_READ);
        return tx;
    }

    private static boolean isStatementTimeout(Throwable ex) {
        Throwable cursor = ex;
        while (cursor != null) {
            if (cursor instanceof SQLException sql && "57014".equals(sql.getSQLState())) {
                return true;
            }
            cursor = cursor.getCause();
        }
        return false;
    }

    private PrincipalAuthorization loadInTransaction(PlatformSubject subject) {
        applyStatementTimeout();

        int membershipFetch = SecurityDirectoryProperties.overflowDetectFetchSize(maxTenantScopes);
        List<UUID> memberships = jdbcTemplate.query(
                """
                SELECT tenant_id
                  FROM platform.subject_tenant_scope
                 WHERE subject_id = ?
                   AND status = 'ACTIVE'
                 ORDER BY tenant_id
                 LIMIT ?
                """,
                (rs, rowNum) -> rs.getObject("tenant_id", UUID.class),
                subject.subjectId(),
                membershipFetch
        );
        if (memberships.size() > maxTenantScopes) {
            throw new SecurityDirectoryConfiguration.DirectoryFailureException(
                    "SECURITY_DIRECTORY_LIMIT");
        }
        Set<UUID> tenantScopes = Set.copyOf(memberships);

        // Bound joined grant/resource rows in SQL before driver builds a full unbounded list.
        int authorityFetch = SecurityDirectoryProperties.overflowDetectFetchSize(
                maxMaterializedAuthorityRows);
        List<GrantRow> grantRows = jdbcTemplate.query(
                """
                SELECT g.grant_id,
                       g.tenant_id,
                       g.permission,
                       g.resource_type,
                       g.scope_kind,
                       r.resource_id
                  FROM platform.permission_grant g
                  LEFT JOIN platform.permission_grant_resource r
                         ON r.grant_id = g.grant_id
                 WHERE g.subject_id = ?
                   AND g.revoked_at IS NULL
                 ORDER BY g.grant_id, r.resource_id
                 LIMIT ?
                """,
                (rs, rowNum) -> new GrantRow(
                        rs.getObject("grant_id", UUID.class),
                        rs.getObject("tenant_id", UUID.class),
                        rs.getString("permission"),
                        rs.getString("resource_type"),
                        rs.getString("scope_kind"),
                        rs.getObject("resource_id", UUID.class)
                ),
                subject.subjectId(),
                authorityFetch
        );
        if (grantRows.size() > maxMaterializedAuthorityRows) {
            throw new SecurityDirectoryConfiguration.DirectoryFailureException(
                    "SECURITY_DIRECTORY_LIMIT");
        }

        Map<UUID, MutableGrant> byGrant = new HashMap<>();
        for (GrantRow row : grantRows) {
            MutableGrant grant = byGrant.computeIfAbsent(row.grantId(), id -> new MutableGrant(
                    row.tenantId(), row.permission(), row.resourceType(), row.scopeKind()));
            if (!Objects.equals(grant.tenantId, row.tenantId())
                    || !Objects.equals(grant.permission, row.permission())
                    || !Objects.equals(grant.resourceType, row.resourceType())
                    || !Objects.equals(grant.scopeKind, row.scopeKind())) {
                throw new SecurityDirectoryConfiguration.DirectoryFailureException(
                        "SECURITY_DIRECTORY_CORRUPT");
            }
            if (row.resourceId() != null) {
                grant.resourceIds.add(row.resourceId());
                if (grant.resourceIds.size() > maxNamedResourcesPerGrant) {
                    throw new SecurityDirectoryConfiguration.DirectoryFailureException(
                            "SECURITY_DIRECTORY_LIMIT");
                }
            }
        }

        if (byGrant.size() > maxPermissionGrants) {
            throw new SecurityDirectoryConfiguration.DirectoryFailureException(
                    "SECURITY_DIRECTORY_LIMIT");
        }

        Set<PermissionGrant> grants = new LinkedHashSet<>();
        for (MutableGrant mutable : byGrant.values()) {
            if (!tenantScopes.contains(mutable.tenantId)) {
                throw new SecurityDirectoryConfiguration.DirectoryFailureException(
                        "SECURITY_DIRECTORY_CORRUPT");
            }
            try {
                ResourceType resourceType = ResourceType.valueOf(mutable.resourceType);
                ResourceScope.Kind kind = ResourceScope.Kind.valueOf(mutable.scopeKind);
                ResourceScope scope;
                if (kind == ResourceScope.Kind.TENANT_WIDE) {
                    if (!mutable.resourceIds.isEmpty()) {
                        throw new SecurityDirectoryConfiguration.DirectoryFailureException(
                                "SECURITY_DIRECTORY_CORRUPT");
                    }
                    scope = ResourceScope.tenantWide(resourceType);
                } else {
                    if (mutable.resourceIds.isEmpty()) {
                        throw new SecurityDirectoryConfiguration.DirectoryFailureException(
                                "SECURITY_DIRECTORY_CORRUPT");
                    }
                    scope = ResourceScope.named(resourceType, mutable.resourceIds);
                }
                grants.add(new PermissionGrant(
                        mutable.tenantId,
                        Permission.of(mutable.permission),
                        scope
                ));
            } catch (SecurityDirectoryConfiguration.DirectoryFailureException ex) {
                throw ex;
            } catch (RuntimeException ex) {
                throw new SecurityDirectoryConfiguration.DirectoryFailureException(
                        "SECURITY_DIRECTORY_CORRUPT", ex);
            }
        }

        return new PrincipalAuthorization(tenantScopes, grants);
    }

    private void applyStatementTimeout() {
        long millis = Math.max(1L, queryTimeout.toMillis());
        jdbcTemplate.execute((java.sql.Connection connection) -> {
            try (PreparedStatement ps = connection.prepareStatement(
                    "SET LOCAL statement_timeout = '" + millis + "ms'")) {
                ps.execute();
            }
            return null;
        });
    }

    private record GrantRow(
            UUID grantId,
            UUID tenantId,
            String permission,
            String resourceType,
            String scopeKind,
            UUID resourceId
    ) {
    }

    private static final class MutableGrant {
        private final UUID tenantId;
        private final String permission;
        private final String resourceType;
        private final String scopeKind;
        private final Set<UUID> resourceIds = new HashSet<>();

        private MutableGrant(UUID tenantId, String permission, String resourceType, String scopeKind) {
            this.tenantId = tenantId;
            this.permission = permission;
            this.resourceType = resourceType;
            this.scopeKind = scopeKind;
        }
    }
}
