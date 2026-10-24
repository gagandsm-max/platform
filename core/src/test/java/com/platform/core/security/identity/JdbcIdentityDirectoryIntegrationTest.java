package com.platform.core.security.identity;

import com.platform.core.security.http.OidcResourceServerProperties;
import com.platform.core.security.http.SecurityDirectoryConfiguration;
import com.platform.core.security.http.SecurityDirectoryProperties;
import com.platform.core.support.PostgresTestSupport;
import org.junit.jupiter.api.AfterEach;
import org.junit.jupiter.api.BeforeEach;
import org.junit.jupiter.api.Tag;
import org.junit.jupiter.api.Test;
import org.springframework.jdbc.core.JdbcTemplate;
import org.springframework.jdbc.datasource.DataSourceTransactionManager;

import java.time.Duration;
import java.util.Optional;
import java.util.UUID;

import static org.junit.jupiter.api.Assertions.assertEquals;
import static org.junit.jupiter.api.Assertions.assertFalse;
import static org.junit.jupiter.api.Assertions.assertThrows;
import static org.junit.jupiter.api.Assertions.assertTrue;

/**
 * Real PostgreSQL identity matching and lifecycle behavior.
 * SOURCE_DRAFT / NOT_RUN — authored for Master review; not executed in this assignment.
 */
@Tag("postgres")
class JdbcIdentityDirectoryIntegrationTest {

    private static final String ISSUER = "https://issuer.example/realms/platform";
    private static final String AUDIENCE = "platform-api";
    private static final String PROVIDER = "platform-oidc";

    private PostgresTestSupport pg;
    private JdbcIdentityDirectory directory;

    @BeforeEach
    void setUp() throws Exception {
        pg = new PostgresTestSupport();
        pg.start();
        pg.migrate();
        directory = newDirectory(Duration.ofSeconds(2), 256, 1024, 1024);
    }

    @AfterEach
    void tearDown() {
        if (pg != null) {
            pg.stop();
        }
    }

    @Test
    void exactCaseAndUnicodeSubjectMatch() {
        UUID subjectId = insertActiveOidcSubject("user-Case-Ü");
        Optional<IdentityDirectory.MappedIdentity> hit = directory.find(oidcKey("user-Case-Ü"));
        assertTrue(hit.isPresent());
        assertEquals(subjectId, hit.get().platformSubject().subjectId());
        assertFalse(hit.get().disabled());

        assertTrue(directory.find(oidcKey("user-case-ü")).isEmpty());
        // Leading/trailing whitespace is rejected by ExternalSubject before directory lookup
        // (identity must never be trim-normalized at the directory boundary either).
        assertThrows(IllegalArgumentException.class, () -> oidcKey("user-Case-Ü "));
        assertThrows(IllegalArgumentException.class, () -> oidcKey(" user-Case-Ü"));
    }

    @Test
    void wrongTypeInstanceIssuerOrAudienceMisses() {
        insertActiveOidcSubject("user-a");
        assertTrue(directory.find(new ProviderIdentityKey(
                IdentityProviderType.SAML,
                new ProviderInstanceId(PROVIDER),
                new ExternalSubject("user-a")
        )).isEmpty());
        assertTrue(directory.find(new ProviderIdentityKey(
                IdentityProviderType.OIDC,
                new ProviderInstanceId("other-oidc"),
                new ExternalSubject("user-a")
        )).isEmpty());

        JdbcTemplate jdbc = pg.jdbc();
        jdbc.update("""
                UPDATE platform.identity_provider_instance
                   SET issuer_uri = issuer_uri
                 WHERE provider_instance_id = ?
                """, PROVIDER);

        OidcResourceServerProperties wrongIssuer = oidcProps();
        wrongIssuer.setIssuerUri("https://evil.example/realms/platform");
        JdbcIdentityDirectory mismatched = new JdbcIdentityDirectory(
                pg.jdbc(),
                new DataSourceTransactionManager(pg.dataSource()),
                wrongIssuer,
                enabledProps(Duration.ofSeconds(2), 256, 1024, 1024)
        );
        assertTrue(mismatched.find(oidcKey("user-a")).isEmpty());

        OidcResourceServerProperties wrongAudience = oidcProps();
        wrongAudience.setAudience("other-api");
        JdbcIdentityDirectory mismatchedAud = new JdbcIdentityDirectory(
                pg.jdbc(),
                new DataSourceTransactionManager(pg.dataSource()),
                wrongAudience,
                enabledProps(Duration.ofSeconds(2), 256, 1024, 1024)
        );
        assertTrue(mismatchedAud.find(oidcKey("user-a")).isEmpty());
    }

    @Test
    void providerDisabledOrRetiredMissesSeparatelyFromMappingDisabled() {
        UUID subjectId = insertActiveOidcSubject("user-b");
        JdbcTemplate jdbc = pg.jdbc();

        jdbc.update("UPDATE platform.identity_provider_instance SET status='DISABLED' WHERE provider_instance_id=?",
                PROVIDER);
        assertTrue(directory.find(oidcKey("user-b")).isEmpty());

        jdbc.update("UPDATE platform.identity_provider_instance SET status='ACTIVE' WHERE provider_instance_id=?",
                PROVIDER);
        jdbc.update("""
                UPDATE platform.identity_mapping SET disabled = TRUE
                 WHERE provider_instance_id = ? AND external_subject = ?
                """, PROVIDER, "user-b");
        Optional<IdentityDirectory.MappedIdentity> mappingDisabled = directory.find(oidcKey("user-b"));
        assertTrue(mappingDisabled.isPresent());
        assertTrue(mappingDisabled.get().disabled());
        assertEquals(subjectId, mappingDisabled.get().platformSubject().subjectId());

        jdbc.update("""
                UPDATE platform.identity_mapping SET disabled = FALSE
                 WHERE provider_instance_id = ? AND external_subject = ?
                """, PROVIDER, "user-b");
        jdbc.update("UPDATE platform.platform_subject SET status='DISABLED' WHERE subject_id=?", subjectId);
        Optional<IdentityDirectory.MappedIdentity> subjectDisabled = directory.find(oidcKey("user-b"));
        assertTrue(subjectDisabled.isPresent());
        assertTrue(subjectDisabled.get().disabled());

        jdbc.update("UPDATE platform.platform_subject SET status='ACTIVE' WHERE subject_id=?", subjectId);
        jdbc.update("UPDATE platform.identity_provider_instance SET status='RETIRED' WHERE provider_instance_id=?",
                PROVIDER);
        assertTrue(directory.find(oidcKey("user-b")).isEmpty());
    }

    @Test
    void providerTrustRetargetAndReactivationRejected() {
        insertActiveOidcSubject("user-c");
        JdbcTemplate jdbc = pg.jdbc();

        assertThrows(Exception.class, () -> jdbc.update("""
                UPDATE platform.identity_provider_instance
                   SET issuer_uri = 'https://retargeted.example/realms/platform'
                 WHERE provider_instance_id = ?
                """, PROVIDER));

        jdbc.update("UPDATE platform.identity_provider_instance SET status='RETIRED' WHERE provider_instance_id=?",
                PROVIDER);
        assertThrows(Exception.class, () -> jdbc.update("""
                UPDATE platform.identity_provider_instance
                   SET status = 'ACTIVE'
                 WHERE provider_instance_id = ?
                """, PROVIDER));
        assertThrows(Exception.class, () -> jdbc.update(
                "DELETE FROM platform.identity_provider_instance WHERE provider_instance_id=?", PROVIDER));
    }

    @Test
    void databaseFailureFailsClosedWithoutEmptyFallbackSemantics() {
        pg.stop();
        SecurityDirectoryConfiguration.DirectoryFailureException ex = assertThrows(
                SecurityDirectoryConfiguration.DirectoryFailureException.class,
                () -> directory.find(oidcKey("user-a"))
        );
        assertEquals("SECURITY_DIRECTORY_UNAVAILABLE", ex.reasonCode());
        assertFalse(ex.getMessage().toLowerCase().contains("password"));
        assertFalse(ex.getMessage().toLowerCase().contains("jdbc:"));
    }

    @Test
    void newlyConstructedDirectoryInstanceSeesCommittedMapping() {
        UUID subjectId = insertActiveOidcSubject("user-fresh");
        JdbcIdentityDirectory fresh = newDirectory(Duration.ofSeconds(2), 256, 1024, 1024);
        Optional<IdentityDirectory.MappedIdentity> hit = fresh.find(oidcKey("user-fresh"));
        assertTrue(hit.isPresent());
        assertEquals(subjectId, hit.get().platformSubject().subjectId());

        pg.jdbc().update("""
                UPDATE platform.identity_mapping SET disabled = TRUE
                 WHERE provider_instance_id = ? AND external_subject = ?
                """, PROVIDER, "user-fresh");
        JdbcIdentityDirectory after = newDirectory(Duration.ofSeconds(2), 256, 1024, 1024);
        Optional<IdentityDirectory.MappedIdentity> disabled = after.find(oidcKey("user-fresh"));
        assertTrue(disabled.isPresent());
        assertTrue(disabled.get().disabled());
    }

    @Test
    void statementTimeoutFailsClosedAsTimeoutWithoutDetailLeak() throws Exception {
        insertActiveOidcSubject("user-timeout");
        JdbcIdentityDirectory timed = newDirectory(Duration.ofMillis(1), 256, 1024, 1024);
        org.springframework.transaction.support.TransactionTemplate locker =
                new org.springframework.transaction.support.TransactionTemplate(
                        new DataSourceTransactionManager(pg.dataSource()));
        java.util.concurrent.CountDownLatch locked = new java.util.concurrent.CountDownLatch(1);
        java.util.concurrent.CountDownLatch release = new java.util.concurrent.CountDownLatch(1);
        java.util.concurrent.ExecutorService pool = java.util.concurrent.Executors.newSingleThreadExecutor();
        java.util.concurrent.Future<?> holder = null;
        try {
            holder = pool.submit(() -> locker.execute(status -> {
                pg.jdbc().execute("LOCK TABLE platform.identity_mapping IN ACCESS EXCLUSIVE MODE");
                locked.countDown();
                try {
                    if (!release.await(15, java.util.concurrent.TimeUnit.SECONDS)) {
                        throw new IllegalStateException("lock holder release deadline exceeded");
                    }
                } catch (InterruptedException ie) {
                    Thread.currentThread().interrupt();
                    throw new IllegalStateException("lock holder interrupted", ie);
                }
                return null;
            }));
            assertTrue(locked.await(10, java.util.concurrent.TimeUnit.SECONDS), "table lock was not acquired");
            SecurityDirectoryConfiguration.DirectoryFailureException ex = assertThrows(
                    SecurityDirectoryConfiguration.DirectoryFailureException.class,
                    () -> timed.find(oidcKey("user-timeout"))
            );
            assertEquals("SECURITY_DIRECTORY_TIMEOUT", ex.reasonCode());
            assertFalse(ex.getMessage().toLowerCase().contains("identity_mapping"));
            assertFalse(ex.getMessage().toLowerCase().contains("password"));
        } finally {
            release.countDown();
            if (holder != null) {
                holder.get(15, java.util.concurrent.TimeUnit.SECONDS);
            }
            pool.shutdownNow();
            assertTrue(pool.awaitTermination(10, java.util.concurrent.TimeUnit.SECONDS));
        }
    }

    private JdbcIdentityDirectory newDirectory(
            Duration timeout, int maxScopes, int maxGrants, int maxNamed
    ) {
        return new JdbcIdentityDirectory(
                pg.jdbc(),
                new DataSourceTransactionManager(pg.dataSource()),
                oidcProps(),
                enabledProps(timeout, maxScopes, maxGrants, maxNamed)
        );
    }

    private UUID insertActiveOidcSubject(String externalSubject) {
        JdbcTemplate jdbc = pg.jdbc();
        UUID subjectId = UUID.randomUUID();
        Integer providers = jdbc.queryForObject(
                "SELECT COUNT(*) FROM platform.identity_provider_instance WHERE provider_instance_id=?",
                Integer.class, PROVIDER);
        if (providers != null && providers == 0) {
            jdbc.update("""
                    INSERT INTO platform.identity_provider_instance
                        (provider_instance_id, provider_type, issuer_uri, audience, status)
                    VALUES (?, 'OIDC', ?, ?, 'ACTIVE')
                    """, PROVIDER, ISSUER, AUDIENCE);
        }
        jdbc.update("""
                INSERT INTO platform.platform_subject (subject_id, status)
                VALUES (?, 'ACTIVE')
                """, subjectId);
        jdbc.update("""
                INSERT INTO platform.identity_mapping
                    (provider_type, provider_instance_id, external_subject, subject_id, disabled)
                VALUES ('OIDC', ?, ?, ?, FALSE)
                """, PROVIDER, externalSubject, subjectId);
        return subjectId;
    }

    private static ProviderIdentityKey oidcKey(String externalSubject) {
        return new ProviderIdentityKey(
                IdentityProviderType.OIDC,
                new ProviderInstanceId(PROVIDER),
                new ExternalSubject(externalSubject)
        );
    }

    private static OidcResourceServerProperties oidcProps() {
        OidcResourceServerProperties props = new OidcResourceServerProperties();
        props.setIssuerUri(ISSUER);
        props.setAudience(AUDIENCE);
        props.setProviderInstanceId(PROVIDER);
        return props;
    }

    private static SecurityDirectoryProperties enabledProps(
            Duration timeout, int maxScopes, int maxGrants, int maxNamed
    ) {
        SecurityDirectoryProperties props = new SecurityDirectoryProperties();
        props.setPersistenceEnabled(true);
        props.setQueryTimeout(timeout);
        props.setMaxTenantScopes(maxScopes);
        props.setMaxPermissionGrants(maxGrants);
        props.setMaxNamedResourcesPerGrant(maxNamed);
        props.setMaxMaterializedAuthorityRows(8192);
        return props;
    }
}
