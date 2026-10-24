package com.platform.core.security.http;

import com.platform.core.security.authorization.EmptyPermissionGrantDirectory;
import com.platform.core.security.authorization.JdbcPermissionGrantDirectory;
import com.platform.core.security.authorization.PermissionGrantDirectory;
import com.platform.core.security.identity.EmptyIdentityDirectory;
import com.platform.core.security.identity.IdentityDirectory;
import com.platform.core.security.identity.JdbcIdentityDirectory;
import org.junit.jupiter.api.Test;
import org.springframework.beans.factory.BeanCreationException;
import org.springframework.boot.test.context.runner.ApplicationContextRunner;
import org.springframework.context.annotation.Bean;
import org.springframework.context.annotation.Configuration;
import org.springframework.jdbc.core.JdbcTemplate;
import org.springframework.jdbc.datasource.DataSourceTransactionManager;
import org.springframework.transaction.PlatformTransactionManager;

import javax.sql.DataSource;
import java.time.Duration;

import static org.assertj.core.api.Assertions.assertThat;
import static org.mockito.Mockito.mock;

/**
 * Default-off Empty selection vs enabled JDBC selection and invalid enabled configuration.
 * Uses a non-connecting mock DataSource (no H2 / embedded runtime assumption).
 * SOURCE_DRAFT / NOT_RUN.
 */
class SecurityDirectoryConfigurationTest {

    private final ApplicationContextRunner runner = new ApplicationContextRunner()
            .withUserConfiguration(SecurityDirectoryConfiguration.class, StubPersistenceConfig.class);

    @Test
    void defaultOffSelectsEmptyDirectories() {
        runner.run(ctx -> {
            assertThat(ctx).hasSingleBean(IdentityDirectory.class);
            assertThat(ctx).hasSingleBean(PermissionGrantDirectory.class);
            assertThat(ctx.getBean(IdentityDirectory.class)).isInstanceOf(EmptyIdentityDirectory.class);
            assertThat(ctx.getBean(PermissionGrantDirectory.class))
                    .isInstanceOf(EmptyPermissionGrantDirectory.class);
            assertThat(ctx).doesNotHaveBean(JdbcIdentityDirectory.class);
            assertThat(ctx).doesNotHaveBean(JdbcPermissionGrantDirectory.class);
        });
    }

    @Test
    void enabledSelectsExactlyOneJdbcImplementationPerInterface() {
        runner.withPropertyValues(
                "platform.security.directories.persistence-enabled=true",
                "platform.security.oidc.issuer-uri=https://issuer.example/realms/platform",
                "platform.security.oidc.audience=platform-api",
                "platform.security.oidc.provider-instance-id=platform-oidc"
        ).run(ctx -> {
            assertThat(ctx).hasSingleBean(IdentityDirectory.class);
            assertThat(ctx).hasSingleBean(PermissionGrantDirectory.class);
            assertThat(ctx.getBean(IdentityDirectory.class)).isInstanceOf(JdbcIdentityDirectory.class);
            assertThat(ctx.getBean(PermissionGrantDirectory.class))
                    .isInstanceOf(JdbcPermissionGrantDirectory.class);
            assertThat(ctx.getBeansOfType(EmptyIdentityDirectory.class)).isEmpty();
            assertThat(ctx.getBeansOfType(EmptyPermissionGrantDirectory.class)).isEmpty();
        });
    }

    @Test
    void enabledInvalidLimitsFailClosedWithoutEmptyFallback() {
        runner.withPropertyValues(
                "platform.security.directories.persistence-enabled=true",
                "platform.security.directories.max-tenant-scopes=0",
                "platform.security.oidc.issuer-uri=https://issuer.example/realms/platform",
                "platform.security.oidc.audience=platform-api",
                "platform.security.oidc.provider-instance-id=platform-oidc"
        ).run(ctx -> {
            assertThat(ctx).hasFailed();
            assertThat(ctx.getStartupFailure())
                    .isInstanceOf(BeanCreationException.class)
                    .hasRootCauseInstanceOf(SecurityDirectoryConfiguration.DirectoryFailureException.class);
        });
    }

    @Test
    void enabledInvalidTimeoutFailClosed() {
        SecurityDirectoryProperties props = new SecurityDirectoryProperties();
        props.setPersistenceEnabled(true);
        props.setQueryTimeout(Duration.ZERO);
        try {
            props.requireValidEnabledConfiguration();
            throw new AssertionError("expected SECURITY_DIRECTORY_CONFIG_INVALID");
        } catch (SecurityDirectoryConfiguration.DirectoryFailureException ex) {
            assertThat(ex.reasonCode()).isEqualTo("SECURITY_DIRECTORY_CONFIG_INVALID");
        }
    }

    @Test
    void enabledOverflowProneLimitAboveCeilingFailClosed() {
        SecurityDirectoryProperties props = new SecurityDirectoryProperties();
        props.setPersistenceEnabled(true);
        props.setMaxMaterializedAuthorityRows(SecurityDirectoryProperties.LIMIT_MAX + 1);
        try {
            props.requireValidEnabledConfiguration();
            throw new AssertionError("expected SECURITY_DIRECTORY_CONFIG_INVALID");
        } catch (SecurityDirectoryConfiguration.DirectoryFailureException ex) {
            assertThat(ex.reasonCode()).isEqualTo("SECURITY_DIRECTORY_CONFIG_INVALID");
        }
    }

    @Test
    void enabledValidRangeEndpointsAcceptedByConfigurationOnly() {
        SecurityDirectoryProperties atMin = new SecurityDirectoryProperties();
        atMin.setPersistenceEnabled(true);
        atMin.setMaxTenantScopes(SecurityDirectoryProperties.LIMIT_MIN);
        atMin.setMaxPermissionGrants(SecurityDirectoryProperties.LIMIT_MIN);
        atMin.setMaxNamedResourcesPerGrant(SecurityDirectoryProperties.LIMIT_MIN);
        atMin.setMaxMaterializedAuthorityRows(SecurityDirectoryProperties.LIMIT_MIN);
        atMin.requireValidEnabledConfiguration();

        SecurityDirectoryProperties atMax = new SecurityDirectoryProperties();
        atMax.setPersistenceEnabled(true);
        atMax.setMaxTenantScopes(SecurityDirectoryProperties.LIMIT_MAX);
        atMax.setMaxPermissionGrants(SecurityDirectoryProperties.LIMIT_MAX);
        atMax.setMaxNamedResourcesPerGrant(SecurityDirectoryProperties.LIMIT_MAX);
        atMax.setMaxMaterializedAuthorityRows(SecurityDirectoryProperties.LIMIT_MAX);
        atMax.requireValidEnabledConfiguration();

        assertThat(SecurityDirectoryProperties.overflowDetectFetchSize(SecurityDirectoryProperties.LIMIT_MAX))
                .isEqualTo(SecurityDirectoryProperties.LIMIT_MAX + 1);
    }

    @Test
    void enabledOverflowProneInputRejectedByConfigurationOnly() {
        SecurityDirectoryProperties props = new SecurityDirectoryProperties();
        props.setPersistenceEnabled(true);
        props.setMaxTenantScopes(0);
        try {
            props.requireValidEnabledConfiguration();
            throw new AssertionError("expected SECURITY_DIRECTORY_CONFIG_INVALID");
        } catch (SecurityDirectoryConfiguration.DirectoryFailureException ex) {
            assertThat(ex.reasonCode()).isEqualTo("SECURITY_DIRECTORY_CONFIG_INVALID");
        }
        props.setMaxTenantScopes(SecurityDirectoryProperties.LIMIT_MIN);
        props.setMaxMaterializedAuthorityRows(Integer.MAX_VALUE);
        try {
            props.requireValidEnabledConfiguration();
            throw new AssertionError("expected SECURITY_DIRECTORY_CONFIG_INVALID for Integer.MAX_VALUE");
        } catch (SecurityDirectoryConfiguration.DirectoryFailureException ex) {
            assertThat(ex.reasonCode()).isEqualTo("SECURITY_DIRECTORY_CONFIG_INVALID");
        }
    }

    @Test
    void enabledZeroMaterializedAuthorityRowsFailClosed() {
        runner.withPropertyValues(
                "platform.security.directories.persistence-enabled=true",
                "platform.security.directories.max-materialized-authority-rows=0",
                "platform.security.oidc.issuer-uri=https://issuer.example/realms/platform",
                "platform.security.oidc.audience=platform-api",
                "platform.security.oidc.provider-instance-id=platform-oidc"
        ).run(ctx -> {
            assertThat(ctx).hasFailed();
            assertThat(ctx.getStartupFailure())
                    .hasRootCauseInstanceOf(SecurityDirectoryConfiguration.DirectoryFailureException.class);
        });
    }

    @Configuration
    static class StubPersistenceConfig {
        @Bean
        DataSource dataSource() {
            // Non-connecting mock: bean selection must not require an embedded/H2 runtime.
            return mock(DataSource.class);
        }

        @Bean
        JdbcTemplate jdbcTemplate(DataSource dataSource) {
            return new JdbcTemplate(dataSource);
        }

        @Bean(name = "transactionManager")
        PlatformTransactionManager transactionManager(DataSource dataSource) {
            return new DataSourceTransactionManager(dataSource);
        }

        @Bean
        OidcResourceServerProperties oidcResourceServerProperties() {
            OidcResourceServerProperties props = new OidcResourceServerProperties();
            props.setIssuerUri("https://issuer.example/realms/platform");
            props.setAudience("platform-api");
            props.setProviderInstanceId("platform-oidc");
            return props;
        }
    }
}
