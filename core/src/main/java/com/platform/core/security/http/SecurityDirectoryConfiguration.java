package com.platform.core.security.http;

import com.platform.core.security.authorization.EmptyPermissionGrantDirectory;
import com.platform.core.security.authorization.JdbcPermissionGrantDirectory;
import com.platform.core.security.authorization.PermissionGrantDirectory;
import com.platform.core.security.identity.DirectoryIdentityResolver;
import com.platform.core.security.identity.EmptyIdentityDirectory;
import com.platform.core.security.identity.IdentityDirectory;
import com.platform.core.security.identity.IdentityResolver;
import com.platform.core.security.identity.JdbcIdentityDirectory;
import org.springframework.beans.factory.annotation.Qualifier;
import org.springframework.boot.autoconfigure.condition.ConditionalOnMissingBean;
import org.springframework.boot.autoconfigure.condition.ConditionalOnProperty;
import org.springframework.boot.context.properties.EnableConfigurationProperties;
import org.springframework.context.annotation.Bean;
import org.springframework.context.annotation.Configuration;
import org.springframework.jdbc.core.JdbcTemplate;
import org.springframework.transaction.PlatformTransactionManager;

/**
 * Default-off Empty directories; enabled mode selects exactly one JDBC implementation per interface.
 * Enabled-mode failures must never fall back to Empty.
 */
@Configuration
@EnableConfigurationProperties(SecurityDirectoryProperties.class)
public class SecurityDirectoryConfiguration {

    @Bean
    @ConditionalOnProperty(
            prefix = "platform.security.directories",
            name = "persistence-enabled",
            havingValue = "true"
    )
    IdentityDirectory jdbcIdentityDirectory(
            JdbcTemplate jdbcTemplate,
            @Qualifier("transactionManager") PlatformTransactionManager transactionManager,
            OidcResourceServerProperties oidcProperties,
            SecurityDirectoryProperties directoryProperties
    ) {
        directoryProperties.requireValidEnabledConfiguration();
        return new JdbcIdentityDirectory(jdbcTemplate, transactionManager, oidcProperties, directoryProperties);
    }

    @Bean
    @ConditionalOnProperty(
            prefix = "platform.security.directories",
            name = "persistence-enabled",
            havingValue = "true"
    )
    PermissionGrantDirectory jdbcPermissionGrantDirectory(
            JdbcTemplate jdbcTemplate,
            @Qualifier("transactionManager") PlatformTransactionManager transactionManager,
            SecurityDirectoryProperties directoryProperties
    ) {
        directoryProperties.requireValidEnabledConfiguration();
        return new JdbcPermissionGrantDirectory(jdbcTemplate, transactionManager, directoryProperties);
    }

    @Bean
    @ConditionalOnMissingBean(IdentityDirectory.class)
    IdentityDirectory identityDirectory() {
        return new EmptyIdentityDirectory();
    }

    @Bean
    @ConditionalOnMissingBean(PermissionGrantDirectory.class)
    PermissionGrantDirectory permissionGrantDirectory() {
        return new EmptyPermissionGrantDirectory();
    }

    @Bean
    @ConditionalOnMissingBean(IdentityResolver.class)
    IdentityResolver identityResolver(IdentityDirectory identityDirectory) {
        return new DirectoryIdentityResolver(identityDirectory);
    }

    /**
     * Controlled directory failure. Reason codes are audit-safe; causes must not be surfaced to clients.
     */
    public static final class DirectoryFailureException extends RuntimeException {
        private final String reasonCode;

        public DirectoryFailureException(String reasonCode) {
            super(sanitize(reasonCode));
            this.reasonCode = sanitize(reasonCode);
        }

        public DirectoryFailureException(String reasonCode, Throwable cause) {
            super(sanitize(reasonCode), cause);
            this.reasonCode = sanitize(reasonCode);
        }

        public String reasonCode() {
            return reasonCode;
        }

        private static String sanitize(String reasonCode) {
            if (reasonCode == null || reasonCode.isBlank()) {
                return "SECURITY_DIRECTORY_UNAVAILABLE";
            }
            return reasonCode;
        }
    }
}
