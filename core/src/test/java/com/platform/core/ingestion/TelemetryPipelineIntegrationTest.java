package com.platform.core.ingestion;

import com.fasterxml.jackson.databind.ObjectMapper;
import com.fasterxml.jackson.datatype.jsr310.JavaTimeModule;
import com.platform.core.ingestion.authority.FailClosedSensorAuthorityResolver;
import com.platform.core.ingestion.authority.SensorScope;
import com.platform.core.ingestion.authority.TestSensorAuthorityResolver;
import com.platform.core.ingestion.dto.CanonicalCounterObservation;
import com.platform.core.ingestion.dto.CanonicalObservation;
import com.platform.core.ingestion.dto.TelemetryObservationBatch;
import com.platform.core.ingestion.dto.TelemetryAckResponse;
import com.platform.core.ingestion.dto.TelemetryBatchRequest;
import com.platform.core.ingestion.replay.PublisherClaimResult;
import com.platform.core.ingestion.replay.TelemetryWatermarkStore;
import com.platform.core.ingestion.replay.ReplayReservation;
import org.apache.kafka.clients.consumer.ConsumerRecord;
import org.junit.jupiter.api.BeforeEach;
import org.junit.jupiter.api.DisplayName;
import org.junit.jupiter.api.Nested;
import org.junit.jupiter.api.Test;
import org.springframework.boot.actuate.health.Status;
import org.springframework.http.HttpStatus;
import org.springframework.kafka.support.Acknowledgment;

import java.security.cert.X509Certificate;
import java.time.Instant;
import java.util.*;

import static org.junit.jupiter.api.Assertions.*;
import static org.mockito.Mockito.*;

/**
 * End-to-end telemetry pipeline integration test covering:
 * 1. Authority Seam & Fail-Closed Behavior
 * 2. Scope & Canonical Payload Validation
 * 3. Watermark Replay Protection & Concurrency Safety
 * 4. Transactional Kafka Producer Delivery & Abort
 * 5. ClickHouse Consumer Processing, Retry, and Health Degrade
 */
public class TelemetryPipelineIntegrationTest {

    private final ObjectMapper objectMapper = new ObjectMapper().registerModule(new JavaTimeModule());
    private final UUID tenantId = UUID.fromString("018f0e0a-0000-7000-8000-000000000001");
    private final UUID entityId = UUID.fromString("018f0e0a-0000-7000-8000-000000000011");
    private final String sensorId = "sensor-tor-01";

    @Nested
    @DisplayName("1. Authority Seam & Scope Verification")
    class AuthorityAndScopeTests {

        @Test
        @DisplayName("Production default FailClosedSensorAuthorityResolver rejects all ingestion requests")
        void testProductionAuthorityFailsClosed() {
            FailClosedSensorAuthorityResolver failClosedResolver = new FailClosedSensorAuthorityResolver();
            IngestionValidator validator = new IngestionValidator(objectMapper, failClosedResolver);
            TelemetryWatermarkStore watermarkStore = mock(TelemetryWatermarkStore.class);
            KafkaTelemetryProducer kafkaProducer = mock(KafkaTelemetryProducer.class);
            SensorIngestionService service = new SensorIngestionService(validator, failClosedResolver, watermarkStore, kafkaProducer, new IngestionMetrics());

            X509Certificate cert = mock(X509Certificate.class);
            when(cert.getSubjectX500Principal()).thenReturn(new javax.security.auth.x500.X500Principal("CN=" + sensorId));
            TelemetryBatchRequest req = new TelemetryBatchRequest(1, sensorId, "ep-1", "b-1", 1, 1, "c-1", List.of());

            IngestionException ex = assertThrows(IngestionException.class, () ->
                    service.ingestTelemetry("{}".getBytes(), req, "b-1", cert)
            );

            assertEquals("ERR_SENSOR_AUTHORITY_UNAVAILABLE", ex.getCode());
            assertEquals(HttpStatus.FORBIDDEN, ex.getStatus());
            verifyNoInteractions(kafkaProducer, watermarkStore);
        }

        @Test
        @DisplayName("Tenant mismatch between scope and observation is rejected")
        void testTenantScopeMismatchRejected() throws Exception {
            SensorScope scope = new SensorScope(sensorId, tenantId, null, Set.of(entityId), true);
            TestSensorAuthorityResolver resolver = new TestSensorAuthorityResolver(scope);
            IngestionValidator validator = new IngestionValidator(objectMapper, resolver);
            TelemetryWatermarkStore watermarkStore = mock(TelemetryWatermarkStore.class);
            KafkaTelemetryProducer kafkaProducer = mock(KafkaTelemetryProducer.class);
            SensorIngestionService service = new SensorIngestionService(validator, resolver, watermarkStore, kafkaProducer, new IngestionMetrics());

            X509Certificate cert = mock(X509Certificate.class);
            when(cert.getSubjectX500Principal()).thenReturn(new javax.security.auth.x500.X500Principal("CN=" + sensorId));

            UUID wrongTenant = UUID.randomUUID();
            Map<String, Object> protoPayload = Map.of("metric_id", "cpu.util", "value", 42.0);
            Map<String, Object> envelope = Map.of(
                    "tenantId", wrongTenant.toString(),
                    "entityId", entityId.toString(),
                    "collectedAt", Instant.now().toString(),
                    "protocol", "snmp",
                    "protocolPayload", protoPayload
            );
            Map<String, Object> rec = Map.of("sequence", 1L, "payload", envelope);
            var recNode = objectMapper.valueToTree(rec);
            byte[] rawNodeBytes = objectMapper.writeValueAsBytes(recNode);
            java.security.MessageDigest md = java.security.MessageDigest.getInstance("SHA-256");
            byte[] hash = md.digest(rawNodeBytes);
            StringBuilder sb = new StringBuilder();
            for (byte b : hash) sb.append(String.format("%02x", b));
            String checksum = sb.toString();

            TelemetryBatchRequest req = new TelemetryBatchRequest(1, sensorId, "ep-1", "b-1", 1, 1, checksum, List.of(recNode));

            IngestionException ex = assertThrows(IngestionException.class, () ->
                    service.ingestTelemetry(null, req, "b-1", cert)
            );

            assertEquals("ERR_TENANT_SCOPE_MISMATCH", ex.getCode());
            assertEquals(HttpStatus.FORBIDDEN, ex.getStatus());
        }

        @Test
        @DisplayName("Observation targeting entity outside sensor authorized scope is rejected")
        void testEntityScopeMismatchRejected() throws Exception {
            SensorScope scope = new SensorScope(sensorId, tenantId, null, Set.of(entityId), false);
            TestSensorAuthorityResolver resolver = new TestSensorAuthorityResolver(scope);
            IngestionValidator validator = new IngestionValidator(objectMapper, resolver);
            TelemetryWatermarkStore watermarkStore = mock(TelemetryWatermarkStore.class);
            KafkaTelemetryProducer kafkaProducer = mock(KafkaTelemetryProducer.class);
            SensorIngestionService service = new SensorIngestionService(validator, resolver, watermarkStore, kafkaProducer, new IngestionMetrics());

            X509Certificate cert = mock(X509Certificate.class);
            when(cert.getSubjectX500Principal()).thenReturn(new javax.security.auth.x500.X500Principal("CN=" + sensorId));
            UUID unauthorizedEntity = UUID.randomUUID();

            Map<String, Object> protoPayload = Map.of("metric_id", "cpu.util", "value", 42.0);
            Map<String, Object> envelope = Map.of(
                    "tenantId", tenantId.toString(),
                    "entityId", unauthorizedEntity.toString(),
                    "collectedAt", Instant.now().toString(),
                    "protocol", "snmp",
                    "protocolPayload", protoPayload
            );
            Map<String, Object> rec = Map.of("sequence", 1L, "payload", envelope);
            var recNode = objectMapper.valueToTree(rec);
            byte[] rawNodeBytes = objectMapper.writeValueAsBytes(recNode);
            java.security.MessageDigest md = java.security.MessageDigest.getInstance("SHA-256");
            byte[] hash = md.digest(rawNodeBytes);
            StringBuilder sb = new StringBuilder();
            for (byte b : hash) sb.append(String.format("%02x", b));
            String checksum = sb.toString();

            TelemetryBatchRequest req = new TelemetryBatchRequest(1, sensorId, "ep-1", "b-1", 1, 1, checksum, List.of(recNode));

            IngestionException ex = assertThrows(IngestionException.class, () ->
                    service.ingestTelemetry(null, req, "b-1", cert)
            );

            assertEquals("ERR_ENTITY_SCOPE_UNAUTHORIZED", ex.getCode());
            assertEquals(HttpStatus.FORBIDDEN, ex.getStatus());
        }
    }

    @Nested
    @DisplayName("2. Replay & Watermark Protection")
    class ReplayProtectionTests {

        private SensorIngestionService service;
        private TelemetryWatermarkStore watermarkStore;
        private KafkaTelemetryProducer kafkaProducer;
        private IngestionValidator validator;
        private SensorScope scope;

        @BeforeEach
        void setUp() {
            scope = new SensorScope(sensorId, tenantId, null, Set.of(entityId), true);
            TestSensorAuthorityResolver resolver = new TestSensorAuthorityResolver(scope);
            validator = mock(IngestionValidator.class);
            watermarkStore = mock(TelemetryWatermarkStore.class);
            kafkaProducer = mock(KafkaTelemetryProducer.class);
            service = new SensorIngestionService(validator, resolver, watermarkStore, kafkaProducer, new IngestionMetrics());
        }

        @Test
        @DisplayName("Exact retry of recorded watermark returns ACK without Kafka publish")
        void testExactRetry() {
            X509Certificate cert = mock(X509Certificate.class);
            when(cert.getSubjectX500Principal()).thenReturn(new javax.security.auth.x500.X500Principal("CN=" + sensorId));
            TelemetryBatchRequest req = new TelemetryBatchRequest(1, sensorId, "ep-1", "b-10", 10, 10, "chk-10", List.of());
            TelemetryObservationBatch obsList = TelemetryObservationBatch.ofNumeric(List.of(
                    new CanonicalObservation(tenantId.toString(), entityId.toString(), "metric", sensorId, Instant.now(), 1.0, 1, 0, "10", "snmp")
            ));

            when(validator.resolveAuthority(cert)).thenReturn(scope);
            when(validator.validateAndExtract(any(), eq(req), eq(scope))).thenReturn(obsList);

            when(watermarkStore.reserveBatch(
                    tenantId, sensorId, "ep-1", 10L, 10L, "chk-10", "b-10"))
                    .thenReturn(ReplayReservation.alreadyAcked(10L));

            TelemetryAckResponse ack = service.ingestTelemetry("{}".getBytes(), req, "b-10", cert);

            assertEquals("ACCEPTED", ack.status());
            assertEquals(10L, ack.acceptedThroughSequence());
            verify(kafkaProducer, never()).publishBatchTransactionally(any(TelemetryObservationBatch.class));
            verify(watermarkStore, never()).finalizeReservedBatch(
                    any(), any(), any(), anyLong(), anyLong(), any(), any(), any(), anyLong());
        }

        @Test
        @DisplayName("Conflicting replay with same sequence but different checksum is rejected with HTTP 409")
        void testConflictingReplayRejected() {
            X509Certificate cert = mock(X509Certificate.class);
            when(cert.getSubjectX500Principal()).thenReturn(new javax.security.auth.x500.X500Principal("CN=" + sensorId));
            TelemetryBatchRequest req = new TelemetryBatchRequest(1, sensorId, "ep-1", "b-10-tampered", 10, 10, "chk-DIFFERENT", List.of());
            TelemetryObservationBatch obsList = TelemetryObservationBatch.ofNumeric(List.of(
                    new CanonicalObservation(tenantId.toString(), entityId.toString(), "metric", sensorId, Instant.now(), 1.0, 1, 0, "10", "snmp")
            ));

            when(validator.resolveAuthority(cert)).thenReturn(scope);
            when(validator.validateAndExtract(any(), eq(req), eq(scope))).thenReturn(obsList);

            when(watermarkStore.reserveBatch(
                    tenantId, sensorId, "ep-1", 10L, 10L, "chk-DIFFERENT", "b-10-tampered"))
                    .thenReturn(ReplayReservation.rejected(
                            10L, "ERR_IDEMPOTENCY_CONFLICT", "checksum conflict"));

            IngestionException ex = assertThrows(IngestionException.class, () ->
                    service.ingestTelemetry("{}".getBytes(), req, "b-10-tampered", cert)
            );

            assertEquals("ERR_IDEMPOTENCY_CONFLICT", ex.getCode());
            assertEquals(HttpStatus.CONFLICT, ex.getStatus());
            verify(kafkaProducer, never()).publishBatchTransactionally(any(TelemetryObservationBatch.class));
        }

        @Test
        @DisplayName("Sequence regression below current watermark is rejected with HTTP 409")
        void testSequenceRegressionRejected() {
            X509Certificate cert = mock(X509Certificate.class);
            when(cert.getSubjectX500Principal()).thenReturn(new javax.security.auth.x500.X500Principal("CN=" + sensorId));
            TelemetryBatchRequest req = new TelemetryBatchRequest(1, sensorId, "ep-1", "b-5", 5, 5, "chk-5", List.of());
            TelemetryObservationBatch obsList = TelemetryObservationBatch.ofNumeric(List.of(
                    new CanonicalObservation(tenantId.toString(), entityId.toString(), "metric", sensorId, Instant.now(), 1.0, 1, 0, "5", "snmp")
            ));

            when(validator.resolveAuthority(cert)).thenReturn(scope);
            when(validator.validateAndExtract(any(), eq(req), eq(scope))).thenReturn(obsList);

            when(watermarkStore.reserveBatch(
                    tenantId, sensorId, "ep-1", 5L, 5L, "chk-5", "b-5"))
                    .thenReturn(ReplayReservation.rejected(
                            10L, "ERR_SEQUENCE_REGRESSION", "sequence already accepted"));

            IngestionException ex = assertThrows(IngestionException.class, () ->
                    service.ingestTelemetry("{}".getBytes(), req, "b-5", cert)
            );

            assertEquals("ERR_SEQUENCE_REGRESSION", ex.getCode());
            assertEquals(HttpStatus.CONFLICT, ex.getStatus());
        }

        @Test
        @DisplayName("Sequence gap beyond next expected sequence is rejected with HTTP 400")
        void testSequenceGapRejected() {
            X509Certificate cert = mock(X509Certificate.class);
            when(cert.getSubjectX500Principal()).thenReturn(new javax.security.auth.x500.X500Principal("CN=" + sensorId));
            TelemetryBatchRequest req = new TelemetryBatchRequest(1, sensorId, "ep-1", "b-12", 12, 12, "chk-12", List.of());
            TelemetryObservationBatch obsList = TelemetryObservationBatch.ofNumeric(List.of(
                    new CanonicalObservation(tenantId.toString(), entityId.toString(), "metric", sensorId, Instant.now(), 1.0, 1, 0, "12", "snmp")
            ));

            when(validator.resolveAuthority(cert)).thenReturn(scope);
            when(validator.validateAndExtract(any(), eq(req), eq(scope))).thenReturn(obsList);

            when(watermarkStore.reserveBatch(
                    tenantId, sensorId, "ep-1", 12L, 12L, "chk-12", "b-12"))
                    .thenReturn(ReplayReservation.rejected(
                            10L, "ERR_SEQUENCE_GAP", "unexpected sequence gap"));

            IngestionException ex = assertThrows(IngestionException.class, () ->
                    service.ingestTelemetry("{}".getBytes(), req, "b-12", cert)
            );

            assertEquals("ERR_SEQUENCE_GAP", ex.getCode());
            assertEquals(HttpStatus.BAD_REQUEST, ex.getStatus());
        }
    }

    @Nested
    @DisplayName("3. Kafka Atomic Publishing & ClickHouse Consumer")
    class KafkaAndClickHouseTests {

        @Test
        @DisplayName("Kafka publish failure aborts ingestion and does not commit watermark")
        void testKafkaPublishFailureAbortsWatermark() {
            SensorScope scope = new SensorScope(sensorId, tenantId, null, Set.of(entityId), true);
            TestSensorAuthorityResolver resolver = new TestSensorAuthorityResolver(scope);
            IngestionValidator validator = mock(IngestionValidator.class);
            TelemetryWatermarkStore watermarkStore = mock(TelemetryWatermarkStore.class);
            KafkaTelemetryProducer kafkaProducer = mock(KafkaTelemetryProducer.class);
            SensorIngestionService service = new SensorIngestionService(validator, resolver, watermarkStore, kafkaProducer, new IngestionMetrics());

            X509Certificate cert = mock(X509Certificate.class);
            when(cert.getSubjectX500Principal()).thenReturn(new javax.security.auth.x500.X500Principal("CN=" + sensorId));
            TelemetryBatchRequest req = new TelemetryBatchRequest(1, sensorId, "ep-1", "b-1", 1, 1, "chk-1", List.of());
            TelemetryObservationBatch obsList = TelemetryObservationBatch.ofNumeric(List.of(
                    new CanonicalObservation(tenantId.toString(), entityId.toString(), "metric", sensorId, Instant.now(), 1.0, 1, 0, "1", "snmp")
            ));

            when(validator.resolveAuthority(cert)).thenReturn(scope);
            when(validator.validateAndExtract(any(), eq(req), eq(scope))).thenReturn(obsList);
            when(watermarkStore.reserveBatch(
                    tenantId, sensorId, "ep-1", 1L, 1L, "chk-1", "b-1"))
                    .thenReturn(ReplayReservation.reserved(0L));
            when(watermarkStore.claimPublisherLease(
                    tenantId, sensorId, "ep-1", 1L, 1L, "chk-1", "b-1"))
                    .thenReturn(PublisherClaimResult.claimed(UUID.randomUUID(), 1L));
            doThrow(new IngestionException(HttpStatus.SERVICE_UNAVAILABLE, "ERR_KAFKA_PUBLISH_FAILED", "Broker connection failed"))
                    .when(kafkaProducer).publishBatchTransactionally(any(TelemetryObservationBatch.class));

            IngestionException ex = assertThrows(IngestionException.class, () ->
                    service.ingestTelemetry("{}".getBytes(), req, "b-1", cert)
            );

            assertEquals("ERR_KAFKA_PUBLISH_FAILED", ex.getCode());
            verify(watermarkStore, never()).finalizeReservedBatch(
                    any(), any(), any(), anyLong(), anyLong(), any(), any(), any(), anyLong());
        }

        @Test
        @DisplayName("ClickHouse consumer successfully writes batch and commits Kafka offset")
        void testClickHouseConsumerSuccess() throws Exception {
            ClickHouseClient clickHouseClient = mock(ClickHouseClient.class);
            IngestionMetrics metrics = new IngestionMetrics();
            TelemetryConsumerHealthIndicator healthIndicator = new TelemetryConsumerHealthIndicator(metrics);
            ClickHouseTelemetryConsumer consumer = new ClickHouseTelemetryConsumer(clickHouseClient, objectMapper, metrics);

            Acknowledgment ack = mock(Acknowledgment.class);
            CanonicalObservation obs = new CanonicalObservation(
                    tenantId.toString(), entityId.toString(), ".1.3.6.1.2.1.1.3.0", sensorId, Instant.now(), 12345.0, 1, 0, "1", "snmp"
            );
            String jsonPayload = objectMapper.writeValueAsString(obs);
            ConsumerRecord<String, String> record = new ConsumerRecord<>("platform.telemetry.canonical", 0, 100L, entityId.toString(), jsonPayload);

            consumer.consume(record, ack);

            verify(clickHouseClient, times(1)).insertNumericBatch(List.of(obs));
            verify(ack, times(1)).acknowledge();
            assertEquals(Status.UP, healthIndicator.health().getStatus());
        }

        @Test
        @DisplayName("ClickHouse consumer writes exact Counter64 and commits Kafka offset")
        void testClickHouseConsumerCounter64Success() throws Exception {
            ClickHouseClient clickHouseClient = mock(ClickHouseClient.class);
            IngestionMetrics metrics = new IngestionMetrics();
            TelemetryConsumerHealthIndicator healthIndicator = new TelemetryConsumerHealthIndicator(metrics);
            ClickHouseTelemetryConsumer consumer = new ClickHouseTelemetryConsumer(clickHouseClient, objectMapper, metrics);

            Acknowledgment ack = mock(Acknowledgment.class);
            CanonicalCounterObservation counter = new CanonicalCounterObservation(
                    tenantId.toString(), entityId.toString(), ".1.3.6.1.2.1.31.1.1.1.6.1", sensorId,
                    Instant.now(), new java.math.BigInteger("18446744073709551615"), 1, 0, "seq-c64", "snmp"
            );
            String jsonPayload = objectMapper.writeValueAsString(counter);
            assertTrue(jsonPayload.contains("\"counterValue\":\"18446744073709551615\""));
            ConsumerRecord<String, String> record = new ConsumerRecord<>(
                    "platform.telemetry.canonical", 0, 101L, "seq-c64", jsonPayload);

            consumer.consume(record, ack);

            verify(clickHouseClient, times(1)).insertCounterBatch(List.of(counter));
            verify(ack, times(1)).acknowledge();
            assertEquals(Status.UP, healthIndicator.health().getStatus());
        }

        @Test
        @DisplayName("ClickHouse consumer rejects ambiguous Counter64 payload without offset acknowledgment")
        void testClickHouseConsumerCounter64AmbiguousValueNoAck() throws Exception {
            ClickHouseClient clickHouseClient = mock(ClickHouseClient.class);
            IngestionMetrics metrics = new IngestionMetrics();
            ClickHouseTelemetryConsumer consumer = new ClickHouseTelemetryConsumer(clickHouseClient, objectMapper, metrics);

            Acknowledgment ack = mock(Acknowledgment.class);
            CanonicalCounterObservation counter = new CanonicalCounterObservation(
                    tenantId.toString(), entityId.toString(), ".1.3.6.1.2.1.31.1.1.1.6.1", sensorId,
                    Instant.now(), new java.math.BigInteger("9007199254740993"), 1, 0, "seq-c64-ambiguous", "snmp"
            );
            com.fasterxml.jackson.databind.node.ObjectNode root =
                    (com.fasterxml.jackson.databind.node.ObjectNode) objectMapper.valueToTree(counter);
            root.put("value", 1L);

            ConsumerRecord<String, String> record = new ConsumerRecord<>(
                    "platform.telemetry.canonical", 0, 102L, "seq-c64-ambiguous",
                    objectMapper.writeValueAsString(root));

            IngestionException ex = assertThrows(IngestionException.class, () ->
                    consumer.consume(record, ack)
            );

            assertEquals("ERR_KAFKA_POISON_EVENT", ex.getCode());
            verify(ack, never()).acknowledge();
            verifyNoInteractions(clickHouseClient);
        }

        @Test
        @DisplayName("ClickHouse consumer rejects poison message without offset acknowledgment")
        void testClickHouseConsumerPoisonMessageNoAck() {
            ClickHouseClient clickHouseClient = mock(ClickHouseClient.class);
            IngestionMetrics metrics = new IngestionMetrics();
            TelemetryConsumerHealthIndicator healthIndicator = new TelemetryConsumerHealthIndicator(metrics);
            ClickHouseTelemetryConsumer consumer = new ClickHouseTelemetryConsumer(clickHouseClient, objectMapper, metrics);

            Acknowledgment ack = mock(Acknowledgment.class);
            // Malformed JSON is poison
            ConsumerRecord<String, String> record = new ConsumerRecord<>("platform.telemetry.canonical", 0, 100L, "key", "{unparseable-json");

            IngestionException ex = assertThrows(IngestionException.class, () ->
                    consumer.consume(record, ack)
            );

            assertEquals("ERR_KAFKA_POISON_EVENT", ex.getCode());
            verify(ack, never()).acknowledge();
            verifyNoInteractions(clickHouseClient);
        }

        @Test
        @DisplayName("ClickHouse consumer retry exhaustion marks health DOWN and refuses offset commit")
        void testClickHouseConsumerRetryExhaustionMarksHealthDown() throws Exception {
            ClickHouseClient clickHouseClient = mock(ClickHouseClient.class);
            IngestionMetrics metrics = new IngestionMetrics();
            TelemetryConsumerHealthIndicator healthIndicator = new TelemetryConsumerHealthIndicator(metrics);
            // 2 retries, 10ms backoff for fast unit test
            ClickHouseTelemetryConsumer consumer = new ClickHouseTelemetryConsumer(clickHouseClient, objectMapper, metrics, 2, 10);

            Acknowledgment ack = mock(Acknowledgment.class);
            CanonicalObservation obs = new CanonicalObservation(
                    tenantId.toString(), entityId.toString(), ".1.3.6.1.2.1.1.3.0", sensorId, Instant.now(), 12345.0, 1, 0, "1", "snmp"
            );
            String jsonPayload = objectMapper.writeValueAsString(obs);
            ConsumerRecord<String, String> record = new ConsumerRecord<>("platform.telemetry.canonical", 0, 100L, entityId.toString(), jsonPayload);

            doThrow(new RuntimeException("ClickHouse connection refused"))
                    .when(clickHouseClient).insertNumericBatch(anyList());

            RuntimeException ex = assertThrows(RuntimeException.class, () ->
                    consumer.consume(record, ack)
            );

            assertTrue(ex.getMessage().contains("ClickHouse persistence unavailable"));
            verify(ack, never()).acknowledge();
            assertEquals(Status.DOWN, healthIndicator.health().getStatus());
            assertEquals(
                    "CORE_TELEMETRY_CLICKHOUSE_UNAVAILABLE",
                    healthIndicator.health().getDetails().get("reasonCode").toString()
            );
        }
    }
}
