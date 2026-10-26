package com.platform.core.security.identity;

import com.platform.core.security.http.OidcResourceServerProperties;
import com.platform.core.security.http.SecurityDirectoryConfiguration;
import com.platform.core.security.http.SecurityDirectoryProperties;
import org.springframework.beans.factory.annotation.Qualifier;
import org.springframework.dao.QueryTimeoutException;
import org.springframework.jdbc.core.JdbcTemplate;
import org.springframework.transaction.PlatformTransactionManager;
import org.springframework.transaction.support.TransactionTemplate;

import java.sql.PreparedStatement;
import java.sql.SQLException;
import java.time.Duration;
import java.util.List;
import java.util.Objects;
import java.util.Optional;
import java.util.UUID;

/**
 * PostgreSQL-backed {@link IdentityDirectory}. Exact provider type / instance / opaque subject match.
 * Does not trim, case-fold, or use email as identity. OIDC trust must match configured issuer/audience.
 * Provider, mapping, and subject lifecycle are evaluated separately. No cross-request cache.
 */
public final class JdbcIdentityDirectory implements IdentityDirectory {

    private final JdbcTemplate jdbcTemplate;
    private final TransactionTemplate transactionTemplate;
    private final OidcResourceServerProperties oidcProperties;
    private final Duration queryTimeout;

    public JdbcIdentityDirectory(
            JdbcTemplate jdbcTemplate,
            @Qualifier("transactionManager") PlatformTransactionManager transactionManager,
            OidcResourceServerProperties oidcProperties,
            SecurityDirectoryProperties directoryProperties
    ) {
        this.jdbcTemplate = Objects.requireNonNull(jdbcTemplate, "jdbcTemplate");
        Objects.requireNonNull(transactionManager, "transactionManager");
        this.oidcProperties = Objects.requireNonNull(oidcProperties, "oidcProperties");
        Objects.requireNonNull(directoryProperties, "directoryProperties");
        directoryProperties.requireValidEnabledConfiguration();
        this.queryTimeout = directoryProperties.getQueryTimeout();
        this.transactionTemplate = new TransactionTemplate(transactionManager);
        this.transactionTemplate.setReadOnly(true);
    }

    @Override
    public Optional<MappedIdentity> find(ProviderIdentityKey key) {
        Objects.requireNonNull(key, "key");
        try {
            return transactionTemplate.execute(status -> findInTransaction(key));
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

    private Optional<MappedIdentity> findInTransaction(ProviderIdentityKey key) {
        applyStatementTimeout();

        // Exact equality on type/instance/opaque subject. No LOWER/TRIM/email rewrite.
        // Provider lifecycle: only ACTIVE providers participate.
        // OIDC trust: stored issuer/audience must equal configured resource-server props.
        List<MappingRow> rows = jdbcTemplate.query(
                """
                SELECT m.subject_id,
                       m.disabled AS mapping_disabled,
                       s.status AS subject_status,
                       p.status AS provider_status,
                       p.provider_type AS provider_type,
                       p.issuer_uri AS issuer_uri,
                       p.audience AS audience
                  FROM platform.identity_mapping m
                  INNER JOIN platform.identity_provider_instance p
                          ON p.provider_instance_id = m.provider_instance_id
                         AND p.provider_type = m.provider_type
                  INNER JOIN platform.platform_subject s
                          ON s.subject_id = m.subject_id
                 WHERE m.provider_type = ?
                   AND m.provider_instance_id = ?
                   AND m.external_subject = ?
                """,
                (rs, rowNum) -> new MappingRow(
                        rs.getObject("subject_id", UUID.class),
                        rs.getBoolean("mapping_disabled"),
                        rs.getString("subject_status"),
                        rs.getString("provider_status"),
                        rs.getString("provider_type"),
                        rs.getString("issuer_uri"),
                        rs.getString("audience")
                ),
                key.providerType().name(),
                key.providerInstanceId().value(),
                key.externalSubject().value()
        );

        if (rows.isEmpty()) {
            return Optional.empty();
        }
        if (rows.size() != 1) {
            throw new SecurityDirectoryConfiguration.DirectoryFailureException(
                    "SECURITY_DIRECTORY_CORRUPT");
        }

        MappingRow row = rows.get(0);

        // Provider lifecycle (separate from mapping.disabled / subject.status).
        if (!"ACTIVE".equals(row.providerStatus())) {
            return Optional.empty();
        }

        // Persisted OIDC trust must match configured issuer, audience, and instance (instance already in key).
        if (IdentityProviderType.OIDC.name().equals(row.providerType())) {
            if (!oidcProperties.isConfigured()) {
                return Optional.empty();
            }
            if (!Objects.equals(oidcProperties.getIssuerUri(), nullToEmpty(row.issuerUri()))
                    || !Objects.equals(oidcProperties.getAudience(), nullToEmpty(row.audience()))) {
                return Optional.empty();
            }
            if (!Objects.equals(oidcProperties.getProviderInstanceId(), key.providerInstanceId().value())) {
                return Optional.empty();
            }
        }

        boolean disabled = row.mappingDisabled() || !"ACTIVE".equals(row.subjectStatus());
        return Optional.of(new MappedIdentity(new PlatformSubject(row.subjectId()), disabled));
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

    private static String nullToEmpty(String value) {
        return value == null ? "" : value;
    }

    private record MappingRow(
            UUID subjectId,
            boolean mappingDisabled,
            String subjectStatus,
            String providerStatus,
            String providerType,
            String issuerUri,
            String audience
    ) {
    }
}
