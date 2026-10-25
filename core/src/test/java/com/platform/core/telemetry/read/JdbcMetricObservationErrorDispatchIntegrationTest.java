package com.platform.core.telemetry.read;

import ch.qos.logback.classic.Logger;
import ch.qos.logback.classic.spi.ILoggingEvent;
import ch.qos.logback.core.read.ListAppender;
import com.fasterxml.jackson.databind.JsonNode;
import com.fasterxml.jackson.databind.ObjectMapper;
import com.nimbusds.jose.JWSAlgorithm;
import com.nimbusds.jose.JWSHeader;
import com.nimbusds.jose.crypto.RSASSASigner;
import com.nimbusds.jwt.JWTClaimsSet;
import com.nimbusds.jwt.SignedJWT;
import com.platform.core.api.EntityMetricObservationController;
import com.platform.core.api.error.ApiExceptionHandler;
import com.platform.core.domain.inventory.Entity;
import com.platform.core.domain.inventory.EntityService;
import com.platform.core.domain.inventory.LifecycleState;
import com.platform.core.logging.CorrelationIdFilter;
import com.platform.core.security.authorization.JdbcPermissionGrantDirectory;
import com.platform.core.security.authorization.PermissionGrantDirectory;
import com.platform.core.security.http.*;
import com.platform.core.security.identity.IdentityDirectory;
import com.platform.core.security.identity.JdbcIdentityDirectory;
import com.platform.core.support.PostgresTestSupport;
import jakarta.servlet.DispatcherType;
import jakarta.servlet.Filter;
import jakarta.servlet.FilterChain;
import jakarta.servlet.ServletException;
import jakarta.servlet.ServletRequest;
import jakarta.servlet.ServletResponse;
import jakarta.servlet.http.HttpServletRequest;
import jakarta.servlet.http.HttpServletResponse;
import org.junit.jupiter.api.*;
import org.junit.jupiter.api.parallel.Execution;
import org.junit.jupiter.api.parallel.ExecutionMode;
import org.junit.jupiter.api.parallel.Isolated;
import org.slf4j.LoggerFactory;
import org.springframework.beans.factory.annotation.Autowired;
import org.springframework.boot.SpringBootConfiguration;
import org.springframework.boot.autoconfigure.EnableAutoConfiguration;
import org.springframework.boot.autoconfigure.jdbc.DataSourceAutoConfiguration;
import org.springframework.boot.autoconfigure.kafka.KafkaAutoConfiguration;
import org.springframework.boot.test.context.SpringBootTest;
import org.springframework.boot.test.mock.mockito.MockBean;
import org.springframework.boot.test.web.server.LocalServerPort;
import org.springframework.boot.web.servlet.FilterRegistrationBean;
import org.springframework.context.annotation.Bean;
import org.springframework.context.annotation.Configuration;
import org.springframework.context.annotation.Import;
import org.springframework.context.annotation.Primary;
import org.springframework.core.Ordered;
import org.springframework.http.MediaType;
import org.springframework.jdbc.core.JdbcTemplate;
import org.springframework.jdbc.datasource.DataSourceTransactionManager;
import org.springframework.security.core.context.SecurityContextHolder;
import org.springframework.security.oauth2.core.DelegatingOAuth2TokenValidator;
import org.springframework.security.oauth2.core.OAuth2Error;
import org.springframework.security.oauth2.core.OAuth2TokenValidatorResult;
import org.springframework.security.oauth2.jwt.JwtDecoder;
import org.springframework.security.oauth2.jwt.JwtValidators;
import org.springframework.security.oauth2.jwt.NimbusJwtDecoder;
import org.springframework.test.annotation.DirtiesContext;
import org.springframework.test.context.DynamicPropertyRegistry;
import org.springframework.test.context.DynamicPropertySource;
import org.springframework.transaction.PlatformTransactionManager;
import org.springframework.transaction.support.TransactionTemplate;

import javax.sql.DataSource;
import java.io.ByteArrayOutputStream;
import java.io.IOException;
import java.net.Proxy;
import java.net.ProxySelector;
import java.net.SocketAddress;
import java.net.URI;
import java.net.http.HttpClient;
import java.net.http.HttpRequest;
import java.net.http.HttpResponse;
import java.nio.ByteBuffer;
import java.nio.charset.StandardCharsets;
import java.security.KeyPair;
import java.security.KeyPairGenerator;
import java.security.interfaces.RSAPrivateKey;
import java.security.interfaces.RSAPublicKey;
import java.time.Duration;
import java.time.Instant;
import java.util.ArrayList;
import java.util.Date;
import java.util.EnumSet;
import java.util.List;
import java.util.Locale;
import java.util.Map;
import java.util.Set;
import java.util.TreeSet;
import java.util.UUID;
import java.util.concurrent.CompletableFuture;
import java.util.concurrent.CompletionStage;
import java.util.concurrent.CountDownLatch;
import java.util.concurrent.Flow;
import java.util.concurrent.TimeUnit;
import java.util.concurrent.atomic.AtomicReference;

import static org.junit.jupiter.api.Assertions.*;
import static org.mockito.ArgumentMatchers.any;
import static org.mockito.Mockito.*;

/**
 * SOURCE_ONLY / REAL_HTTP_RUNTIME_NOT_RUN / DIRECTORY_FAILURE_HTTP_CONTRACT_SPECIFIED.
 * Real loopback TCP and embedded servlet ERROR dispatch, not MockMvc or an /error override.
 * Nimbus and JDBC authority are real. Only entity lookup and physical ClickHouse are doubles.
 * Requires separately admitted isolated PostgreSQL; no customer IdP or production activation.
 * Execute sequentially: this class owns only its synthetic IDs, never another suite's rows.
 */
@Tag("postgres")
@Execution(ExecutionMode.SAME_THREAD)
@TestInstance(TestInstance.Lifecycle.PER_METHOD)
@Isolated("Owned JDBC fixture mutations and real HTTP exchanges must not overlap another test class")
@Timeout(45)
@SpringBootTest(classes = JdbcMetricObservationErrorDispatchIntegrationTest.TestApplication.class,
        webEnvironment = SpringBootTest.WebEnvironment.RANDOM_PORT,
        properties = {"server.address=127.0.0.1", "server.ssl.enabled=false",
                "server.forward-headers-strategy=none"})
@DirtiesContext(classMode = DirtiesContext.ClassMode.AFTER_CLASS)
class JdbcMetricObservationErrorDispatchIntegrationTest {
    private static final String ISSUER = "https://idp.platform.test/realms/platform";
    private static final String AUDIENCE = "platform-core-api";
    private static final String PROVIDER = "platform-oidc";
    private static final String COUNTER = "18446744073709551615";
    private static final String METRIC = ".1.3.6.1.2.1.31.1.1.1.6.1";
    private static final Instant FROM = Instant.parse("2026-10-16T00:00:00Z");
    private static final Instant TO = FROM.plusSeconds(3600);
    // Class-specific v7 namespace source-scanned across Core tests.
    // Source uniqueness does not establish database ownership: the pre-seed guard below is required.
    private static final UUID TENANT = id("a1");
    private static final UUID ENTITY = id("e1");
    private static final UUID SUBJECT = id("aa");
    private static final UUID GRANT = id("c1");
    private static final int MAX_BODY_BYTES = 32 * 1024;
    private static final Duration REQUEST_TIMEOUT = Duration.ofSeconds(10);
    private static final KeyPair KEY = signingKey();
    private static final PostgresTestSupport PG = startPostgres();

    @LocalServerPort private int port;
    @Autowired private ObjectMapper mapper;
    @Autowired private JdbcTemplate jdbc;
    @Autowired private PlatformTransactionManager transactions;
    @Autowired private IdentityDirectory identities;
    @Autowired private PermissionGrantDirectory grants;
    @Autowired private JwtDecoder decoder;
    @Autowired private DispatchObserver observer;
    @MockBean private EntityService entities;
    @MockBean private ClickHouseMetricObservationReader reader;
    private HttpClient client;
    private boolean fixtureRowsOwned;

    @DynamicPropertySource
    static void properties(DynamicPropertyRegistry registry) {
        registry.add("platform.security.directories.persistence-enabled", () -> "true");
        registry.add("platform.security.directories.query-timeout", () -> "2s");
        registry.add("platform.security.oidc.issuer-uri", () -> ISSUER);
        registry.add("platform.security.oidc.audience", () -> AUDIENCE);
        registry.add("platform.security.oidc.provider-instance-id", () -> PROVIDER);
    }

    @BeforeAll
    static void immutableTrust() {
        PG.jdbc().update("""
                INSERT INTO platform.identity_provider_instance
                    (provider_instance_id, provider_type, issuer_uri, audience, status)
                VALUES (?, 'OIDC', ?, ?, 'ACTIVE')
                ON CONFLICT (provider_instance_id) DO NOTHING
                """, PROVIDER, ISSUER, AUDIENCE);
        var providers = PG.jdbc().queryForList("""
                SELECT provider_type, issuer_uri, audience, status
                FROM platform.identity_provider_instance WHERE provider_instance_id=?
                """, PROVIDER);
        assertEquals(1, providers.size());
        var provider = providers.get(0);
        assertEquals("OIDC", provider.get("provider_type"));
        assertEquals(ISSUER, provider.get("issuer_uri"));
        assertEquals(AUDIENCE, provider.get("audience"));
        assertEquals("ACTIVE", provider.get("status"));
    }

    @AfterAll
    static void stopPostgres() { PG.stop(); }

    @BeforeEach
    void seedOwnedRows() {
        assertNull(observer.active.get(), "previous server dispatch is unresolved; do not reuse fixture rows");
        assertInstanceOf(JdbcIdentityDirectory.class, identities);
        assertInstanceOf(JdbcPermissionGrantDirectory.class, grants);
        assertInstanceOf(NimbusJwtDecoder.class, decoder);
        assertFalse(mockingDetails(identities).isMock());
        assertFalse(mockingDetails(grants).isMock());
        assertFalse(mockingDetails(decoder).isMock());
        assertFalse(fixtureRowsOwned, "an earlier owned fixture has not completed cleanup");
        new TransactionTemplate(transactions).executeWithoutResult(transaction -> {
            assertFixtureIdsUnused();
            jdbc.update("""
                    INSERT INTO platform.tenant (tenant_id, name, mode, status, timezone, created_at, updated_at)
                    VALUES (?, 'HTTP dispatch synthetic tenant', 'SINGLE', 'ACTIVE', 'UTC', NOW(), NOW())
                    """, TENANT);
            jdbc.update("INSERT INTO platform.platform_subject (subject_id, status) VALUES (?, 'ACTIVE')", SUBJECT);
            jdbc.update("""
                    INSERT INTO platform.identity_mapping
                        (provider_type, provider_instance_id, external_subject, subject_id, disabled)
                    VALUES ('OIDC', ?, ?, ?, FALSE)
                    """, PROVIDER, SUBJECT.toString(), SUBJECT);
            jdbc.update("""
                    INSERT INTO platform.subject_tenant_scope (subject_id, tenant_id, status)
                    VALUES (?, ?, 'ACTIVE')
                    """, SUBJECT, TENANT);
            jdbc.update("""
                    INSERT INTO platform.permission_grant
                        (grant_id, subject_id, tenant_id, permission, resource_type, scope_kind)
                    VALUES (?, ?, ?, 'entity.read', 'ENTITY', 'NAMED_RESOURCES')
                    """, GRANT, SUBJECT, TENANT);
            jdbc.update("INSERT INTO platform.permission_grant_resource (grant_id, resource_id) VALUES (?, ?)",
                    GRANT, ENTITY);
        });
        // Set only after the entire seed transaction has committed normally. A rejected seed,
        // rollback or ambiguous commit must never authorize AfterEach to delete existing rows.
        fixtureRowsOwned = true;
        when(entities.get(TENANT, ENTITY)).thenReturn(new Entity(
                ENTITY, TENANT, "DEVICE", "HTTP dispatch fixture", null, null, LifecycleState.ACTIVE,
                null, null, null, null, null, FROM, FROM, "test", FROM, FROM));
        when(reader.read(any())).thenReturn(new MetricObservationPage(List.of(
                new MetricObservationRow(FROM.plusSeconds(1), null, COUNTER, 1, 0,
                        "http-dispatch-sensor", "7", "SNMP")), false));
        client = HttpClient.newBuilder().connectTimeout(Duration.ofSeconds(2))
                .followRedirects(HttpClient.Redirect.NEVER).version(HttpClient.Version.HTTP_1_1)
                .proxy(new ProxySelector() {
                    @Override public List<Proxy> select(URI uri) { return List.of(Proxy.NO_PROXY); }
                    @Override public void connectFailed(URI uri, SocketAddress address, IOException failure) { }
                }).build();
    }

    @AfterEach
    @Timeout(35)
    void boundedOwnedCleanup() throws Exception {
        if (!fixtureRowsOwned) return; // BeforeEach failure before ownership confers no cleanup authority.
        try {
            if (client != null) {
                client.shutdown();
                if (!client.awaitTermination(Duration.ofSeconds(2))) {
                    client.shutdownNow();
                    assertTrue(client.awaitTermination(Duration.ofSeconds(2)), "HTTP client did not terminate");
                }
            }
        } finally {
            DispatchProbe remaining = observer.active.get();
            if (remaining != null) {
                boolean requestDone = remaining.requestFinished.await(2, TimeUnit.SECONDS);
                boolean errorDone = !remaining.expectError || remaining.errorFinished.await(2, TimeUnit.SECONDS);
                assertTrue(requestDone && errorDone,
                        "server dispatch completion unknown; retain owned rows and refuse fixture reuse until context teardown");
                observer.active.compareAndSet(remaining, null);
            }
            deleteOwnedRows();
        }
    }

    private void assertFixtureIdsUnused() {
        // Check all four UUIDs across stored identities, including named-resource references:
        // ENTITY is a mocked lookup boundary here and has no entity row inserted by this class.
        assertEquals(Boolean.FALSE, jdbc.queryForObject("""
                WITH fixture_ids(id) AS (
                    VALUES (CAST(? AS UUID)), (CAST(? AS UUID)), (CAST(? AS UUID)), (CAST(? AS UUID))
                )
                SELECT EXISTS (
                    SELECT 1 FROM platform.tenant t JOIN fixture_ids f ON t.tenant_id=f.id
                    UNION ALL
                    SELECT 1 FROM platform.entity e JOIN fixture_ids f ON e.entity_id=f.id
                    UNION ALL
                    SELECT 1 FROM platform.platform_subject s JOIN fixture_ids f ON s.subject_id=f.id
                    UNION ALL
                    SELECT 1 FROM platform.permission_grant g JOIN fixture_ids f ON g.grant_id=f.id
                    UNION ALL
                    SELECT 1 FROM platform.permission_grant_resource r JOIN fixture_ids f ON r.resource_id=f.id
                    UNION ALL
                    SELECT 1 FROM platform.identity_mapping m JOIN fixture_ids f ON m.external_subject=CAST(f.id AS TEXT)
                    WHERE m.provider_instance_id=?
                )
                """, Boolean.class, TENANT, ENTITY, SUBJECT, GRANT, PROVIDER),
                "fixture ID or external-subject binding already exists; refuse seeding without deleting it");
    }

    private void deleteOwnedRows() {
        assertTrue(fixtureRowsOwned, "refuse deletion without a successfully committed owned seed");
        // Exact rows inserted by the successful seed only; one bounded transaction, never trust/provider deletion.
        new TransactionTemplate(transactions).executeWithoutResult(transaction -> {
            assertEquals(1, jdbc.update("""
                    DELETE FROM platform.permission_grant_resource WHERE grant_id=? AND resource_id=?
                    """, GRANT, ENTITY));
            assertEquals(1, jdbc.update("""
                    DELETE FROM platform.permission_grant WHERE grant_id=? AND subject_id=? AND tenant_id=?
                    """, GRANT, SUBJECT, TENANT));
            assertEquals(1, jdbc.update("""
                    DELETE FROM platform.subject_tenant_scope WHERE subject_id=? AND tenant_id=?
                    """, SUBJECT, TENANT));
            assertEquals(1, jdbc.update("""
                    DELETE FROM platform.identity_mapping
                    WHERE provider_type='OIDC' AND provider_instance_id=? AND external_subject=? AND subject_id=?
                    """, PROVIDER, SUBJECT.toString(), SUBJECT));
            assertEquals(1, jdbc.update("DELETE FROM platform.platform_subject WHERE subject_id=?", SUBJECT));
            assertEquals(1, jdbc.update("DELETE FROM platform.tenant WHERE tenant_id=?", TENANT));
        });
        fixtureRowsOwned = false; // Clear only after the cleanup transaction has committed normally.
    }

    @Test
    void validSignedMappedPrincipalPreservesExactCounter64OverRealHttp() throws Exception {
        assertEquals(7, TENANT.version());
        assertEquals(7, ENTITY.version());
        allowed(token());
    }

    @Test
    void activeGrantAgainstInactiveMembershipFailsClosedOverRealHttp() throws Exception {
        revokeMembershipOnly();
        corrupted(token());
    }

    @Test
    void membershipOnlyRevocationFailsClosedForIdenticalBearerOverRealHttp() throws Exception {
        String bearer = token();
        allowed(bearer);
        clearInvocations(entities, reader);
        revokeMembershipOnly();
        corrupted(bearer);
    }

    @Test
    void atomicGrantAndMembershipRevocationReturnsOpaque404ForIdenticalBearerOverRealHttp() throws Exception {
        String bearer = token();
        allowed(bearer);
        clearInvocations(entities, reader);
        new TransactionTemplate(transactions).executeWithoutResult(transaction -> {
            assertEquals(1, jdbc.update("""
                    UPDATE platform.permission_grant SET revoked_at=NOW()
                    WHERE subject_id=? AND tenant_id=? AND revoked_at IS NULL
                    """, SUBJECT, TENANT));
            assertEquals(1, jdbc.update("""
                    UPDATE platform.subject_tenant_scope SET status='REVOKED'
                    WHERE subject_id=? AND tenant_id=? AND status='ACTIVE'
                    """, SUBJECT, TENANT));
        });
        Wire response = exchange(bearer, false);
        assertEquals(404, response.status());
        JsonNode error = mapper.readTree(response.body());
        assertEquals(Set.of("code", "message", "correlationId", "timestamp"), fields(error));
        assertEquals("RESOURCE_NOT_FOUND", error.path("code").asText());
        assertEquals("Resource not found", error.path("message").asText());
        assertEquals(response.correlation(), error.path("correlationId").asText());
        assertEquals(response.correlation(), response.correlationHeader());
        assertSanitized(response.body(), bearer);
        verifyNoInteractions(entities, reader);
    }

    private void allowed(String bearer) throws Exception {
        Wire response = exchange(bearer, false);
        assertEquals(200, response.status());
        assertEquals(response.correlation(), response.correlationHeader());
        JsonNode page = mapper.readTree(response.body());
        assertEquals(TENANT.toString(), page.path("tenantId").asText());
        assertEquals(ENTITY.toString(), page.path("entityId").asText());
        assertEquals(1, page.path("returned").asInt());
        assertEquals(COUNTER, page.path("observations").get(0).path("counterValue").asText());
        assertTrue(page.path("observations").get(0).path("counterValue").isTextual());
        assertFalse(page.path("observations").get(0).has("value"));
        verify(entities).get(TENANT, ENTITY);
        verify(reader).read(new MetricObservationQuery(TENANT, ENTITY,
                AllowedMetricSelector.find(METRIC, MetricValueKind.COUNTER64).orElseThrow(), FROM, TO, 100));
        verifyNoMoreInteractions(entities, reader);
    }

    private void revokeMembershipOnly() {
        assertEquals(1, jdbc.update("""
                UPDATE platform.subject_tenant_scope SET status='REVOKED'
                WHERE subject_id=? AND tenant_id=? AND status='ACTIVE'
                """, SUBJECT, TENANT));
        assertEquals(1, jdbc.queryForObject("""
                SELECT COUNT(*) FROM platform.permission_grant
                WHERE grant_id=? AND subject_id=? AND tenant_id=? AND revoked_at IS NULL
                """, Integer.class, GRANT, SUBJECT, TENANT));
    }

    private void corrupted(String bearer) throws Exception {
        Logger auditLogger = (Logger) LoggerFactory.getLogger(SecurityAuditEmitter.class);
        ListAppender<ILoggingEvent> appender = new ListAppender<>();
        appender.start();
        auditLogger.addAppender(appender);
        try {
            Wire response = exchange(bearer, false, true);
            assertEquals(500, response.status());
            assertTrue(response.contentTypePresent(), "opaque error response must declare JSON Content-Type");
            assertTrue(MediaType.APPLICATION_JSON.isCompatibleWith(MediaType.parseMediaType(response.contentType())));
            JsonNode error = mapper.readTree(response.body());
            assertEquals(Set.of("code", "message", "correlationId", "timestamp"), fields(error));
            assertEquals("INTERNAL_ERROR", error.path("code").asText());
            assertEquals("Unexpected internal error", error.path("message").asText());
            assertEquals(response.correlation(), response.correlationHeader());
            assertEquals(response.correlation(), error.path("correlationId").asText());
            assertDoesNotThrow(() -> Instant.parse(error.path("timestamp").asText()));
            assertSanitized(response.body(), bearer);
            verifyNoInteractions(entities, reader);
            List<ILoggingEvent> audit = List.copyOf(appender.list);
            assertTrue(audit.stream().anyMatch(event -> event.getFormattedMessage()
                            .contains("reason=SECURITY_DIRECTORY_CORRUPT")
                            && (response.correlation().equals(event.getMDCPropertyMap().get("correlation_id"))
                            || event.getFormattedMessage().contains("correlation_id=" + response.correlation()))),
                    "expected corruption audit for this exact request");
            for (ILoggingEvent event : audit) {
                assertSanitized(event.getFormattedMessage(), bearer);
                assertNull(event.getThrowableProxy(), "audit must not include exception/stack details");
            }
        } finally {
            auditLogger.detachAppender(appender);
            appender.stop();
        }
    }

    private Wire exchange(String bearer, boolean requireErrorDispatch) throws Exception {
        return exchange(bearer, requireErrorDispatch, false);
    }

    private Wire exchange(String bearer, boolean requireErrorDispatch, boolean inspectErrorSanitization)
            throws Exception {
        String correlation = "http-dispatch-" + UUID.randomUUID();
        DispatchProbe probe = new DispatchProbe(correlation, Thread.currentThread().threadId(), requireErrorDispatch);
        assertTrue(observer.active.compareAndSet(null, probe), "another HTTP observation is still active");
        BoundedBody subscriber = new BoundedBody();
        CompletableFuture<HttpResponse<byte[]>> pending = null;
        try {
            URI target = URI.create("http://127.0.0.1:" + port + route()
                    + "?metricId=" + METRIC + "&valueKind=COUNTER64&from=" + FROM + "&to=" + TO + "&limit=100");
            HttpRequest request = HttpRequest.newBuilder(target).timeout(REQUEST_TIMEOUT)
                    .header("Accept", "application/json").header("X-Correlation-ID", correlation)
                    .header("Authorization", "Bearer " + bearer).GET().build();
            pending = client.sendAsync(request, info -> subscriber);
            // Covers body completion as well as headers. A stalled/truncated body is never a wire response.
            HttpResponse<byte[]> response = pending.get(REQUEST_TIMEOUT.plusSeconds(2).toMillis(), TimeUnit.MILLISECONDS);
            var contentType = response.headers().firstValue("Content-Type");
            Wire wire = new Wire(response.statusCode(), contentType.isPresent(), contentType.orElse(""),
                    new String(response.body(), StandardCharsets.UTF_8), correlation,
                    response.headers().firstValue("X-Correlation-ID").orElse(""));
            if (inspectErrorSanitization) {
                // Check error wire headers too; a standard Bearer challenge is syntax, not token disclosure.
                for (List<String> values : response.headers().map().values()) {
                    for (String value : values) {
                        assertSanitized(value.replaceFirst("(?i)^Bearer(?:\\s+|$)", ""), bearer);
                    }
                }
                assertSanitized(wire.body(), bearer);
            }
            assertTrue(response.previousResponse().isEmpty(), "redirects must never be followed");
            boolean requestFinished = probe.requestFinished.await(2, TimeUnit.SECONDS);
            boolean errorFinished = !requireErrorDispatch || probe.errorFinished.await(2, TimeUnit.SECONDS);
            List<DispatchObservation> observations = probe.snapshot();
            assertTrue(requestFinished, "server REQUEST finally not observed");
            assertTrue(errorFinished, "expected server dispatch did not complete");
            assertFalse(probe.overflow, "unexpected repeated dispatches exceed observation bound");
            assertTrue(observations.stream().anyMatch(o -> o.dispatcher() == DispatcherType.REQUEST));
            assertEquals(requireErrorDispatch,
                    observations.stream().anyMatch(o -> o.dispatcher() == DispatcherType.ERROR),
                    "server dispatch type does not match the handled response path");
            for (DispatchObservation observed : observations) {
                assertNotEquals(probe.clientThread, observed.threadId(), "context must be observed on server thread");
                assertTrue(observed.contextCleared(), "authenticated context survived server dispatch finally");
                assertEquals(correlation, observed.requestCorrelation());
                assertTrue(observed.responseCorrelationMatches(), "response correlation header changed in dispatch");
                if (observed.dispatcher() == DispatcherType.ERROR) {
                    assertTrue(observed.originalErrorUriMatches(), "ERROR must refer to the metric request");
                }
            }
            return wire;
        } finally {
            if (pending != null && !pending.isDone()) pending.cancel(true);
            subscriber.abort();
            if (probe.requestFinished.getCount() == 0
                    && (!probe.expectError || probe.errorFinished.getCount() == 0)) {
                observer.active.compareAndSet(probe, null);
            } // Otherwise AfterEach bounds completion; never claim cancellation proved server termination.
        }
    }

    private static void assertSanitized(String text, String bearer) {
        assertFalse(text.contains(bearer), "wire/audit must exclude signed bearer");
        String lower = text.toLowerCase(Locale.ROOT);
        for (String forbidden : List.of("bearer ", "password", "credential", "secret", "platform_test",
                "platform_dev", "select ", "insert ", "update ", "delete ", "identity_mapping",
                "permission_grant", "subject_tenant_scope", "jdbc:", "org.postgresql", "java.lang",
                "org.springframework", "com.platform", "exception", "stacktrace", "stack trace",
                "authorization directory failed", "observations", "countervalue", COUNTER, "http-dispatch-sensor")) {
            assertFalse(lower.contains(forbidden), "wire/audit contains forbidden sensitive category: " + forbidden);
        }
        assertFalse(java.util.regex.Pattern.compile("(?m)^\\s*at\\s+[a-zA-Z_$][\\w.$]*\\(").matcher(text).find(),
                "wire/audit must not contain stack frames");
    }

    private static Set<String> fields(JsonNode node) {
        Set<String> fields = new TreeSet<>();
        node.fieldNames().forEachRemaining(fields::add);
        return fields;
    }

    private static String route() {
        return "/api/v1/tenants/" + TENANT + "/entities/" + ENTITY + "/metric-observations";
    }

    private static String token() throws Exception {
        Instant now = Instant.now();
        SignedJWT jwt = new SignedJWT(new JWSHeader(JWSAlgorithm.RS256), new JWTClaimsSet.Builder()
                .issuer(ISSUER).subject(SUBJECT.toString()).audience(AUDIENCE)
                .issueTime(Date.from(now.minusSeconds(5))).notBeforeTime(Date.from(now.minusSeconds(5)))
                .expirationTime(Date.from(now.plusSeconds(600))).build());
        jwt.sign(new RSASSASigner((RSAPrivateKey) KEY.getPrivate()));
        return jwt.serialize();
    }

    private static UUID id(String suffix) {
        return UUID.fromString("019a126a-2490-7000-a022-0000000000" + suffix);
    }

    private static KeyPair signingKey() {
        try {
            KeyPairGenerator generator = KeyPairGenerator.getInstance("RSA");
            generator.initialize(2048);
            return generator.generateKeyPair();
        } catch (Exception failure) {
            throw new IllegalStateException("Cannot create test-only signing key", failure);
        }
    }

    private static PostgresTestSupport startPostgres() {
        PostgresTestSupport pg = new PostgresTestSupport();
        try {
            pg.start();
            pg.migrate();
            return pg;
        } catch (Exception failure) {
            pg.stop();
            throw new IllegalStateException("Cannot initialize isolated HTTP security PostgreSQL fixture", failure);
        }
    }

    private record Wire(int status, boolean contentTypePresent, String contentType, String body,
                        String correlation, String correlationHeader) { }
    private record DispatchObservation(DispatcherType dispatcher, long threadId, boolean contextCleared,
                                       String requestCorrelation, boolean originalErrorUriMatches,
                                       boolean responseCorrelationMatches) { }

    private static final class DispatchProbe {
        final String correlation;
        final long clientThread;
        final boolean expectError;
        final CountDownLatch requestFinished = new CountDownLatch(1);
        final CountDownLatch errorFinished = new CountDownLatch(1);
        private final List<DispatchObservation> observations = new ArrayList<>();
        volatile boolean overflow;
        DispatchProbe(String correlation, long clientThread, boolean expectError) {
            this.correlation = correlation;
            this.clientThread = clientThread;
            this.expectError = expectError;
        }
        synchronized void record(DispatchObservation observation) {
            if (observations.size() < 4) observations.add(observation);
            else overflow = true;
            if (observation.dispatcher() == DispatcherType.REQUEST) requestFinished.countDown();
            if (observation.dispatcher() == DispatcherType.ERROR) errorFinished.countDown();
        }
        synchronized List<DispatchObservation> snapshot() { return List.copyOf(observations); }
    }

    /** Before Spring Security, outside its context holder; never handles or translates an exception. */
    static final class DispatchObserver implements Filter {
        private static final String PROBE_ATTRIBUTE = DispatchObserver.class.getName() + ".probe";
        final AtomicReference<DispatchProbe> active = new AtomicReference<>();
        @Override
        public void doFilter(ServletRequest input, ServletResponse output, FilterChain chain)
                throws IOException, ServletException {
            HttpServletRequest request = (HttpServletRequest) input;
            DispatchProbe probe = active.get();
            if (probe != null && probe.correlation.equals(request.getHeader("X-Correlation-ID"))) {
                request.setAttribute(PROBE_ATTRIBUTE, probe);
            } else {
                probe = request.getAttribute(PROBE_ATTRIBUTE) instanceof DispatchProbe value ? value : null;
            }
            DispatcherType actualDispatch = request.getDispatcherType();
            try {
                chain.doFilter(input, output);
            } finally {
                if (probe != null) {
                    var authentication = SecurityContextHolder.getContext().getAuthentication();
                    probe.record(new DispatchObservation(actualDispatch, Thread.currentThread().threadId(),
                            authentication == null || !authentication.isAuthenticated(),
                            request.getHeader("X-Correlation-ID"),
                            route().equals(request.getAttribute("jakarta.servlet.error.request_uri")),
                            probe.correlation.equals(((HttpServletResponse) output).getHeader("X-Correlation-ID"))));
                }
            }
        }
    }

    /** Backpressure-aware byte cap; only onComplete fulfills the response body. No unbounded ofInputStream/readAllBytes. */
    private static final class BoundedBody implements HttpResponse.BodySubscriber<byte[]> {
        private final CompletableFuture<byte[]> body = new CompletableFuture<>();
        private final ByteArrayOutputStream bytes = new ByteArrayOutputStream();
        private Flow.Subscription subscription;
        @Override public CompletionStage<byte[]> getBody() { return body; }
        @Override public synchronized void onSubscribe(Flow.Subscription value) {
            if (subscription != null || body.isDone()) { value.cancel(); return; }
            subscription = value;
            subscription.request(1);
        }
        @Override public synchronized void onNext(List<ByteBuffer> buffers) {
            for (ByteBuffer buffer : buffers) {
                if (buffer.remaining() > MAX_BODY_BYTES - bytes.size()) {
                    abort();
                    return;
                }
                byte[] chunk = new byte[buffer.remaining()];
                buffer.get(chunk);
                bytes.writeBytes(chunk);
            }
            if (!body.isDone()) subscription.request(1);
        }
        @Override public synchronized void onError(Throwable failure) { body.completeExceptionally(failure); }
        @Override public synchronized void onComplete() { body.complete(bytes.toByteArray()); }
        synchronized void abort() {
            if (subscription != null) subscription.cancel();
            if (!body.isDone()) body.completeExceptionally(new IOException("HTTP body incomplete or exceeds byte bound"));
        }
    }

    @SpringBootConfiguration
    @EnableAutoConfiguration(exclude = {DataSourceAutoConfiguration.class, KafkaAutoConfiguration.class})
    @Import({HttpSecurityConfiguration.class, PlatformJwtAuthenticationConverter.class,
            AuthorizationRouteMapper.class, AuthorizationEnforcementFilter.class,
            SecurityErrorResponseWriter.class, SecurityAuditEmitter.class, SecurityDirectoryConfiguration.class,
            ApiExceptionHandler.class, CorrelationIdFilter.class, EntityMetricObservationController.class,
            EntityMetricObservationService.class, PersistenceAndJwtFixtures.class})
    static class TestApplication { }

    @Configuration
    static class PersistenceAndJwtFixtures {
        @Bean @Primary DataSource dataSource() { return PG.dataSource(); }
        @Bean JdbcTemplate jdbcTemplate(DataSource source) {
            JdbcTemplate template = new JdbcTemplate(source);
            template.setQueryTimeout(3);
            return template;
        }
        @Bean(name = "transactionManager")
        PlatformTransactionManager transactionManager(DataSource source) { return new DataSourceTransactionManager(source); }
        @Bean @Primary
        OidcResourceServerProperties oidcResourceServerProperties() {
            OidcResourceServerProperties properties = new OidcResourceServerProperties();
            properties.setIssuerUri(ISSUER);
            properties.setAudience(AUDIENCE);
            properties.setProviderInstanceId(PROVIDER);
            return properties;
        }
        @Bean @Primary
        JwtDecoder jwtDecoder(OidcResourceServerProperties properties) {
            NimbusJwtDecoder decoder = NimbusJwtDecoder.withPublicKey((RSAPublicKey) KEY.getPublic()).build();
            decoder.setJwtValidator(new DelegatingOAuth2TokenValidator<>(
                    JwtValidators.createDefaultWithIssuer(properties.getIssuerUri()),
                    jwt -> jwt.getAudience().contains(properties.getAudience())
                            ? OAuth2TokenValidatorResult.success()
                            : OAuth2TokenValidatorResult.failure(new OAuth2Error(
                                    "invalid_token", "Required audience is missing", null))));
            return decoder;
        }
        @Bean DispatchObserver dispatchObserver() { return new DispatchObserver(); }
        @Bean
        FilterRegistrationBean<DispatchObserver> dispatchObserverRegistration(DispatchObserver observer) {
            FilterRegistrationBean<DispatchObserver> registration = new FilterRegistrationBean<>(observer);
            registration.setName("testOnlyDispatchObservation");
            registration.setOrder(Ordered.HIGHEST_PRECEDENCE + 1);
            registration.setDispatcherTypes(EnumSet.of(DispatcherType.REQUEST, DispatcherType.ERROR));
            return registration;
        }
    }
}
