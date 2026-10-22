package com.platform.core.security.http;

import org.springframework.boot.context.properties.ConfigurationProperties;

import java.time.Duration;

/**
 * Default-disabled JDBC identity/grant directory settings.
 * Limits are defensive implementation ceilings pending required-capacity validation,
 * not Phase-1 scope reductions, capacity claims, or licensing claims.
 * Production enablement is not authorized by this slice alone.
 */
@ConfigurationProperties(prefix = "platform.security.directories")
public class SecurityDirectoryProperties {

    public static final int LIMIT_MIN = 1;
    public static final int LIMIT_MAX = 65_536;

    /**
     * When false (default), EmptyIdentityDirectory / EmptyPermissionGrantDirectory remain selected.
     * When true, exactly one JDBC implementation per interface is required; failures never fall back to Empty.
     */
    private boolean persistenceEnabled = false;

    /** Per-statement PostgreSQL statement_timeout for directory reads. */
    private Duration queryTimeout = Duration.ofSeconds(2);

    /** Max ACTIVE tenant memberships materialized for one subject load. */
    private int maxTenantScopes = 256;

    /** Max non-revoked permission grants materialized for one subject load. */
    private int maxPermissionGrants = 1024;

    /** Max named resource ids allowed on a single NAMED_RESOURCES grant. */
    private int maxNamedResourcesPerGrant = 1024;

    /**
     * Max joined grant/resource rows materialized for one subject load (SQL LIMIT + overflow-detect row).
     * Master-selected default 8192; configurable 1..65536.
     */
    private int maxMaterializedAuthorityRows = 8192;

    public boolean isPersistenceEnabled() {
        return persistenceEnabled;
    }

    public void setPersistenceEnabled(boolean persistenceEnabled) {
        this.persistenceEnabled = persistenceEnabled;
    }

    public Duration getQueryTimeout() {
        return queryTimeout;
    }

    public void setQueryTimeout(Duration queryTimeout) {
        this.queryTimeout = queryTimeout == null ? Duration.ofSeconds(2) : queryTimeout;
    }

    public int getMaxTenantScopes() {
        return maxTenantScopes;
    }

    public void setMaxTenantScopes(int maxTenantScopes) {
        this.maxTenantScopes = maxTenantScopes;
    }

    public int getMaxPermissionGrants() {
        return maxPermissionGrants;
    }

    public void setMaxPermissionGrants(int maxPermissionGrants) {
        this.maxPermissionGrants = maxPermissionGrants;
    }

    public int getMaxNamedResourcesPerGrant() {
        return maxNamedResourcesPerGrant;
    }

    public void setMaxNamedResourcesPerGrant(int maxNamedResourcesPerGrant) {
        this.maxNamedResourcesPerGrant = maxNamedResourcesPerGrant;
    }

    public int getMaxMaterializedAuthorityRows() {
        return maxMaterializedAuthorityRows;
    }

    public void setMaxMaterializedAuthorityRows(int maxMaterializedAuthorityRows) {
        this.maxMaterializedAuthorityRows = maxMaterializedAuthorityRows;
    }

    /**
     * Validates enabled-mode configuration. Does not enable production or invent capacity claims.
     * Checked max+1 arithmetic is safe because every count is bounded to LIMIT_MAX.
     */
    public void requireValidEnabledConfiguration() {
        if (!persistenceEnabled) {
            return;
        }
        if (queryTimeout == null || queryTimeout.isZero() || queryTimeout.isNegative()) {
            throw new SecurityDirectoryConfiguration.DirectoryFailureException(
                    "SECURITY_DIRECTORY_CONFIG_INVALID");
        }
        if (queryTimeout.compareTo(Duration.ofSeconds(30)) > 0) {
            throw new SecurityDirectoryConfiguration.DirectoryFailureException(
                    "SECURITY_DIRECTORY_CONFIG_INVALID");
        }
        requireLimitInRange(maxTenantScopes);
        requireLimitInRange(maxPermissionGrants);
        requireLimitInRange(maxNamedResourcesPerGrant);
        requireLimitInRange(maxMaterializedAuthorityRows);
    }

    private static void requireLimitInRange(int value) {
        if (value < LIMIT_MIN || value > LIMIT_MAX) {
            throw new SecurityDirectoryConfiguration.DirectoryFailureException(
                    "SECURITY_DIRECTORY_CONFIG_INVALID");
        }
    }

    /** Overflow-detection fetch size: configured max + 1, with checked arithmetic. */
    public static int overflowDetectFetchSize(int configuredMax) {
        try {
            return Math.addExact(configuredMax, 1);
        } catch (ArithmeticException ex) {
            throw new SecurityDirectoryConfiguration.DirectoryFailureException(
                    "SECURITY_DIRECTORY_CONFIG_INVALID", ex);
        }
    }
}
