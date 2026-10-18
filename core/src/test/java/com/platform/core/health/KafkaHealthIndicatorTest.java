package com.platform.core.health;

import com.platform.core.logging.CoreSupportabilityEventCodes;
import org.apache.kafka.clients.admin.AdminClient;
import org.apache.kafka.clients.admin.AdminClientConfig;
import org.apache.kafka.clients.admin.DescribeClusterResult;
import org.apache.kafka.common.KafkaFuture;
import org.apache.kafka.common.Node;
import org.apache.kafka.common.errors.InterruptException;
import org.apache.kafka.common.internals.KafkaFutureImpl;
import org.apache.kafka.common.utils.AppInfoParser;
import org.junit.jupiter.api.AfterEach;
import org.junit.jupiter.api.Test;
import org.junit.jupiter.api.Timeout;
import org.mockito.ArgumentCaptor;
import org.mockito.MockedStatic;
import org.springframework.boot.actuate.health.Health;
import org.springframework.boot.actuate.health.Status;
import org.springframework.test.util.ReflectionTestUtils;

import java.net.InetAddress;
import java.net.ServerSocket;
import java.time.Duration;
import java.util.Collection;
import java.util.List;
import java.util.Map;
import java.util.Properties;
import java.util.concurrent.ExecutionException;
import java.util.concurrent.TimeUnit;
import java.util.concurrent.atomic.AtomicBoolean;
import java.util.concurrent.atomic.AtomicReference;

import static org.junit.jupiter.api.Assertions.*;
import static org.mockito.ArgumentMatchers.*;
import static org.mockito.Mockito.*;

@Timeout(8)
class KafkaHealthIndicatorTest {
    private static final String SECRET = "password=private-bootstrap private-cluster-id";
    private static final List<Node> NODES = List.of(new Node(1, "private-host", 9092), new Node(2, "private-host", 9093));

    @AfterEach
    void clearTestInterruption() {
        Thread.interrupted();
    }

    @Test
    void healthyResultPreservesNodeCountAndUsesCompatibleFiniteClientTimeouts() throws Exception {
        try (Fixture f = new Fixture()) {
            System.out.println("Resolved kafka-clients=" + AppInfoParser.getVersion());
            Health health = f.indicator.health();
            assertEquals(Status.UP, health.getStatus());
            assertEquals(Map.of("dependency", "kafka", "nodeCount", 2), health.getDetails());
            int request = Integer.parseInt(f.properties.getProperty(AdminClientConfig.REQUEST_TIMEOUT_MS_CONFIG));
            int api = Integer.parseInt(f.properties.getProperty(AdminClientConfig.DEFAULT_API_TIMEOUT_MS_CONFIG));
            assertTrue(request > 0 && request <= api && api <= 2400);
            assertTrue(allowanceNanos(f.nodes) > 0 && allowanceNanos(f.nodes) <= TimeUnit.MILLISECONDS.toNanos(2400));
            assertClosedOnce(f);
        }
    }

    @Test
    void firstStalledFutureExhaustsOneBudgetAndStillReleasesClient() throws Exception {
        try (Fixture f = new Fixture()) {
            when(f.cluster.nodes()).thenReturn(new KafkaFutureImpl<>());
            long start = System.nanoTime();
            assertDown(f.indicator.health());
            assertTrue(elapsedMillis(start) < 2800, "first future must leave client/response headroom");
            verify(f.cluster, never()).clusterId();
            assertClosedOnce(f);
        }
    }

    @Test
    void partialFirstWaitLeavesOnlyRemainderForRealStalledSecondFuture() throws Exception {
        try (Fixture f = new Fixture()) {
            when(f.nodes.get(anyLong(), any(TimeUnit.class))).thenAnswer(invocation -> {
                Thread.sleep(200); // Deliberate synchronous work, not a readiness sleep.
                return NODES;
            });
            KafkaFuture<String> stalled = spy(new KafkaFutureImpl<String>());
            when(f.cluster.clusterId()).thenReturn(stalled);
            long start = System.nanoTime();
            assertDown(f.indicator.health());
            long first = allowanceNanos(f.nodes);
            long second = allowanceNanos(stalled);
            assertTrue(second < first - TimeUnit.MILLISECONDS.toNanos(100), "second wait must not renew the budget");
            assertTrue(elapsedMillis(start) < 2800);
            assertClosedOnce(f);
        }
    }

    @Test
    void clientCreationTimeIsChargedBeforeFirstWait() throws Exception {
        try (Fixture f = new Fixture()) {
            f.creationDelayMillis = 350;
            assertEquals(Status.UP, f.indicator.health().getStatus());
            assertTrue(allowanceNanos(f.nodes) <= TimeUnit.MILLISECONDS.toNanos(2100));
            assertClosedOnce(f);
        }
    }

    @Test
    void overBudgetSynchronousCreationCannotBePreemptedButCannotReportUp() throws Exception {
        try (Fixture f = new Fixture()) {
            f.creationDelayMillis = 3100;
            long start = System.nanoTime();
            assertDown(f.indicator.health());
            assertTrue(elapsedMillis(start) >= 3000, "accounting is not a constructor preemption mechanism");
            verify(f.admin, never()).describeCluster();
            assertEquals(1, assertClosedOnce(f).toMillis(), "expired budget still needs whole-millisecond close, never join(0)");
        }
    }

    @Test
    void creationFailureIsSanitizedAndNoUnreturnedClientIsClosed() throws Exception {
        try (Fixture f = new Fixture()) {
            f.creationFailure = new IllegalArgumentException(SECRET);
            assertDown(f.indicator.health());
            verifyNoInteractions(f.admin);
        }
    }

    @Test
    void describeFailureReleasesReturnedClient() throws Exception {
        try (Fixture f = new Fixture()) {
            when(f.admin.describeCluster()).thenThrow(new IllegalStateException(SECRET));
            assertDown(f.indicator.health());
            assertClosedOnce(f);
        }
    }

    @Test
    void firstGetFailureIsSanitizedAndReleasesClient() throws Exception {
        try (Fixture f = new Fixture()) {
            when(f.nodes.get(anyLong(), any(TimeUnit.class))).thenThrow(new ExecutionException(new IllegalStateException(SECRET)));
            assertDown(f.indicator.health());
            verify(f.cluster, never()).clusterId();
            assertClosedOnce(f);
        }
    }

    @Test
    void secondGetFailureIsSanitizedAndReleasesClient() throws Exception {
        try (Fixture f = new Fixture()) {
            when(f.clusterId.get(anyLong(), any(TimeUnit.class))).thenThrow(new ExecutionException(new IllegalStateException(SECRET)));
            assertDown(f.indicator.health());
            assertClosedOnce(f);
        }
    }

    @Test
    void directCallerInterruptionIsRestoredAfterBoundedCleanup() throws Exception {
        try (Fixture f = new Fixture()) {
            when(f.nodes.get(anyLong(), any(TimeUnit.class))).thenThrow(new InterruptedException(SECRET));
            assertDown(f.indicator.health());
            assertTrue(Thread.currentThread().isInterrupted(), "direct caller interruption must not be swallowed");
            assertClosedOnce(f);
        }
    }

    @Test
    void closeFailureCannotConvertSuccessfulFuturesIntoUp() throws Exception {
        try (Fixture f = new Fixture()) {
            doThrow(new IllegalStateException(SECRET)).when(f.admin).close(any(Duration.class));
            assertDown(f.indicator.health());
            assertCloseDuration(f.admin);
            verify(f.admin, never()).close();
        }
    }

    @Test
    void kafkaCloseInterruptionIsPreservedAndCannotProduceUp() throws Exception {
        try (Fixture f = new Fixture()) {
            doAnswer(invocation -> { throw new InterruptException(SECRET); }).when(f.admin).close(any(Duration.class));
            assertDown(f.indicator.health());
            assertTrue(Thread.currentThread().isInterrupted());
            assertCloseDuration(f.admin);
        }
    }

    @Test
    void closeThatReturnsWithInterruptFlagSetCannotProduceUp() throws Exception {
        try (Fixture f = new Fixture()) {
            doAnswer(invocation -> { Thread.currentThread().interrupt(); return null; }).when(f.admin).close(any(Duration.class));
            assertDown(f.indicator.health());
            assertTrue(Thread.currentThread().isInterrupted());
            assertCloseDuration(f.admin);
        }
    }

    @Test
    void lateFirstSuccessCannotGrantFreshTimeToSecondWait() throws Exception {
        try (Fixture f = new Fixture()) {
            when(f.nodes.get(anyLong(), any(TimeUnit.class))).thenAnswer(invocation -> { Thread.sleep(2500); return NODES; });
            assertDown(f.indicator.health());
            verify(f.cluster, never()).clusterId();
            assertClosedOnce(f);
        }
    }

    @Test
    void preInterruptedCallerDoesNotCreateClient() throws Exception {
        try (Fixture f = new Fixture()) {
            Thread.currentThread().interrupt();
            assertDown(f.indicator.health());
            assertTrue(Thread.currentThread().isInterrupted());
            f.factory.verify(() -> AdminClient.create(any(Properties.class)), never());
        }
    }

    @Test
    void lateSecondSuccessCannotProduceUp() throws Exception {
        try (Fixture f = new Fixture()) {
            when(f.clusterId.get(anyLong(), any(TimeUnit.class))).thenAnswer(invocation -> {
                Thread.sleep(2500);
                return "private-cluster-id";
            });
            assertDown(f.indicator.health());
            assertClosedOnce(f);
        }
    }

    @Test
    void realAdminClientReleasesItsIoThreadAfterUnresponsiveLoopbackPeer() throws Exception {
        // A listening socket with no accept/response is an actual stalled Kafka
        // transport, not a broker or physical Core/Test19 certification claim.
        String clientId = "issue240-health-release-" + Long.toUnsignedString(System.nanoTime());
        AtomicReference<List<Thread>> owned = new AtomicReference<>(List.of());
        try (ServerSocket peer = new ServerSocket(0, 1, InetAddress.getByName("127.0.0.1"));
             MockedStatic<AdminClient> factory = mockStatic(AdminClient.class)) {
            KafkaHealthIndicator indicator = new KafkaHealthIndicator();
            ReflectionTestUtils.setField(indicator, "bootstrapServers", "127.0.0.1:" + peer.getLocalPort());
            factory.when(() -> AdminClient.create(any(Properties.class))).thenAnswer(invocation -> {
                Properties properties = invocation.getArgument(0);
                properties.put(AdminClientConfig.CLIENT_ID_CONFIG, clientId);
                AdminClient client = (AdminClient) invocation.callRealMethod();
                owned.set(Thread.getAllStackTraces().keySet().stream()
                        .filter(thread -> thread.getName().contains(clientId)).toList());
                return client;
            });
            long start = System.nanoTime();
            assertDown(indicator.health());
            assertTrue(elapsedMillis(start) < 2800);
            assertFalse(owned.get().isEmpty(), "real Kafka I/O thread must have been observed");
            for (Thread thread : owned.get()) {
                thread.join(500); // Positive finite wait on this test's own resource.
                assertFalse(thread.isAlive(), "health must release its real Kafka I/O thread");
            }
        }
    }

    private static long elapsedMillis(long start) {
        return TimeUnit.NANOSECONDS.toMillis(System.nanoTime() - start);
    }

    private static void assertDown(Health health) {
        assertEquals(Status.DOWN, health.getStatus());
        assertEquals(Map.of("dependency", "kafka", "reasonCode", CoreSupportabilityEventCodes.KAFKA_HEALTH_PROBE_FAILED), health.getDetails());
        assertFalse(health.toString().contains(SECRET));
        assertFalse(health.getDetails().containsKey("nodeCount"));
    }

    private static long allowanceNanos(KafkaFuture<?> future) throws Exception {
        ArgumentCaptor<Long> amount = ArgumentCaptor.forClass(Long.class);
        ArgumentCaptor<TimeUnit> unit = ArgumentCaptor.forClass(TimeUnit.class);
        verify(future).get(amount.capture(), unit.capture());
        return unit.getValue().toNanos(amount.getValue());
    }

    private static Duration assertClosedOnce(Fixture f) {
        Duration duration = assertCloseDuration(f.admin);
        verify(f.admin, never()).close();
        assertTrue(f.released.get(), "returned client must be released on every exit");
        return duration;
    }

    private static Duration assertCloseDuration(AdminClient admin) {
        ArgumentCaptor<Duration> cap = ArgumentCaptor.forClass(Duration.class);
        verify(admin, times(1)).close(cap.capture());
        assertTrue(cap.getValue().toMillis() >= 1 && cap.getValue().toMillis() <= 100,
                "finite close must convert to 1..100 whole ms, not zero or the default unlimited close");
        return cap.getValue();
    }

    private static final class Fixture implements AutoCloseable {
        final AdminClient admin = mock(AdminClient.class);
        final DescribeClusterResult cluster = mock(DescribeClusterResult.class);
        @SuppressWarnings("unchecked")
        final KafkaFuture<Collection<Node>> nodes = mock(KafkaFuture.class);
        @SuppressWarnings("unchecked")
        final KafkaFuture<String> clusterId = mock(KafkaFuture.class);
        final KafkaHealthIndicator indicator = new KafkaHealthIndicator();
        final AtomicBoolean released = new AtomicBoolean();
        final MockedStatic<AdminClient> factory;
        Properties properties;
        long creationDelayMillis;
        RuntimeException creationFailure;

        Fixture() throws Exception {
            ReflectionTestUtils.setField(indicator, "bootstrapServers", SECRET);
            when(admin.describeCluster()).thenReturn(cluster);
            when(cluster.nodes()).thenReturn(nodes);
            when(cluster.clusterId()).thenReturn(clusterId);
            when(nodes.get(anyLong(), any(TimeUnit.class))).thenReturn(NODES);
            when(clusterId.get(anyLong(), any(TimeUnit.class))).thenReturn("private-cluster-id");
            // Exercise the actual legacy Admin.close() delegation without blocking
            // for its Long.MAX_VALUE duration on a physical broker.
            doCallRealMethod().when(admin).close();
            doAnswer(invocation -> { released.set(true); return null; }).when(admin).close(any(Duration.class));
            factory = mockStatic(AdminClient.class);
            factory.when(() -> AdminClient.create(any(Properties.class))).thenAnswer(invocation -> {
                properties = invocation.getArgument(0);
                if (creationDelayMillis > 0) Thread.sleep(creationDelayMillis);
                if (creationFailure != null) throw creationFailure;
                return admin;
            });
        }

        @Override
        public void close() {
            factory.close();
        }
    }
}
