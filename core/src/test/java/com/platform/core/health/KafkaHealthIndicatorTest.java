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
import org.springframework.boot.actuate.health.Health;
import org.springframework.boot.actuate.health.Status;

import java.net.InetAddress;
import java.net.ServerSocket;
import java.time.Duration;
import java.util.ArrayList;
import java.util.Collection;
import java.util.List;
import java.util.Map;
import java.util.Properties;
import java.util.concurrent.CountDownLatch;
import java.util.concurrent.ExecutionException;
import java.util.concurrent.ExecutorService;
import java.util.concurrent.Executors;
import java.util.concurrent.Future;
import java.util.concurrent.TimeUnit;
import java.util.concurrent.atomic.AtomicBoolean;
import java.util.concurrent.atomic.AtomicInteger;
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
            assertEquals(1, f.indicator.probePoolSize(), "probe pool must remain single-threaded");
        }
    }

    @Test
    void firstStalledFutureExhaustsOneBudgetAndStillReleasesClient() throws Exception {
        try (Fixture f = new Fixture()) {
            when(f.cluster.nodes()).thenReturn(new KafkaFutureImpl<>());
            long start = System.nanoTime();
            Health health = f.indicator.health();
            assertNotEquals(Status.UP, health.getStatus());
            assertFalse(health.toString().contains(SECRET));
            assertTrue(elapsedMillis(start) < 2800, "first future must leave client/response headroom");
            verify(f.cluster, never()).clusterId();
            assertTrue(awaitReleased(f, 5000));
            assertCloseDuration(f.admin);
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
            Health health = f.indicator.health();
            assertNotEquals(Status.UP, health.getStatus());
            assertFalse(health.toString().contains(SECRET));
            long first = allowanceNanos(f.nodes);
            long second = allowanceNanos(stalled);
            assertTrue(second < first - TimeUnit.MILLISECONDS.toNanos(100), "second wait must not renew the budget");
            assertTrue(elapsedMillis(start) < 2800);
            assertTrue(awaitReleased(f, 5000));
            assertCloseDuration(f.admin);
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
    void blockingConstructorCannotMonopolizeHttpThreadOrReportUp() throws Exception {
        try (Fixture f = new Fixture()) {
            f.creationDelayMillis = 3100;
            long start = System.nanoTime();
            Health health = f.indicator.health();
            long httpElapsed = elapsedMillis(start);
            assertTrue(httpElapsed < 2800, "HTTP/actuator thread must return within probe budget even when create blocks");
            assertNotEquals(Status.UP, health.getStatus(), "blocking bootstrap must never publish UP");
            assertUnknown(health, "DEADLINE");
            verify(f.admin, never()).describeCluster();
            assertTrue(awaitReleased(f, 5000), "abandoned create must still close the late client");
            assertEquals(1, assertCloseDuration(f.admin).toMillis(), "expired budget still needs whole-millisecond close, never join(0)");
            assertTrue(f.indicator.probePoolSize() <= 1);
            assertTrue(f.indicator.probeThreadsLive() <= 1);
        }
    }

    @Test
    void concurrentHealthCallsCoalesceOntoOneCreateAndStayResourceBounded() throws Exception {
        try (Fixture f = new Fixture()) {
            CountDownLatch createEntered = new CountDownLatch(1);
            CountDownLatch releaseCreate = new CountDownLatch(1);
            f.createHook = () -> {
                createEntered.countDown();
                assertTrue(releaseCreate.await(5, TimeUnit.SECONDS));
            };
            ExecutorService callers = Executors.newFixedThreadPool(8);
            try {
                List<Future<Health>> futures = new ArrayList<>();
                for (int i = 0; i < 8; i++) {
                    futures.add(callers.submit(f.indicator::health));
                }
                assertTrue(createEntered.await(2, TimeUnit.SECONDS), "probe worker must enter create");
                assertEquals(1, f.createCount.get(), "repeated requests must not start parallel AdminClient.create calls");
                assertTrue(f.indicator.probePoolSize() <= 1);
                releaseCreate.countDown();
                for (Future<Health> future : futures) {
                    Health health = future.get(5, TimeUnit.SECONDS);
                    assertEquals(Status.UP, health.getStatus());
                    assertFalse(health.toString().contains(SECRET));
                }
                assertEquals(1, f.createCount.get());
                assertClosedOnce(f);
            } finally {
                releaseCreate.countDown();
                callers.shutdownNow();
            }
        }
    }

    @Test
    void lateAbandonedSuccessCannotPublishUpAfterDeadline() throws Exception {
        try (Fixture f = new Fixture()) {
            CountDownLatch createEntered = new CountDownLatch(1);
            CountDownLatch releaseCreate = new CountDownLatch(1);
            f.createHook = () -> {
                createEntered.countDown();
                assertTrue(releaseCreate.await(5, TimeUnit.SECONDS));
            };
            ExecutorService caller = Executors.newSingleThreadExecutor();
            try {
                Future<Health> pending = caller.submit(f.indicator::health);
                assertTrue(createEntered.await(2, TimeUnit.SECONDS));
                Health timedOut = pending.get(3, TimeUnit.SECONDS);
                assertUnknown(timedOut, "DEADLINE");
                releaseCreate.countDown();
                assertTrue(awaitReleased(f, 5000));
                assertCloseDuration(f.admin);
                // A subsequent probe after abandonment must observe a fresh create, not a late UP reuse.
                f.creationFailure = new IllegalStateException(SECRET);
                assertDown(f.indicator.health());
            } finally {
                releaseCreate.countDown();
                caller.shutdownNow();
            }
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
    void directCallerInterruptionIsRestoredWithoutStartingProbeWhenPreInterrupted() throws Exception {
        try (Fixture f = new Fixture()) {
            Thread.currentThread().interrupt();
            assertDown(f.indicator.health());
            assertTrue(Thread.currentThread().isInterrupted(), "direct caller interruption must not be swallowed");
            assertEquals(0, f.createCount.get());
        }
    }

    @Test
    void probeInterruptionDuringFutureWaitIsSanitizedAndReleasesClient() throws Exception {
        try (Fixture f = new Fixture()) {
            when(f.nodes.get(anyLong(), any(TimeUnit.class))).thenThrow(new InterruptedException(SECRET));
            assertDown(f.indicator.health());
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
            assertCloseDuration(f.admin);
        }
    }

    @Test
    void closeThatReturnsWithInterruptFlagSetCannotProduceUp() throws Exception {
        try (Fixture f = new Fixture()) {
            doAnswer(invocation -> {
                Thread.currentThread().interrupt();
                return null;
            }).when(f.admin).close(any(Duration.class));
            assertDown(f.indicator.health());
            assertCloseDuration(f.admin);
        }
    }

    @Test
    void lateFirstSuccessCannotGrantFreshTimeToSecondWait() throws Exception {
        try (Fixture f = new Fixture()) {
            when(f.nodes.get(anyLong(), any(TimeUnit.class))).thenAnswer(invocation -> {
                Thread.sleep(2500);
                return NODES;
            });
            Health health = f.indicator.health();
            assertNotEquals(Status.UP, health.getStatus());
            assertFalse(health.toString().contains(SECRET));
            verify(f.cluster, never()).clusterId();
            assertTrue(awaitReleased(f, 5000));
            assertCloseDuration(f.admin);
        }
    }

    @Test
    void lateSecondSuccessCannotProduceUp() throws Exception {
        try (Fixture f = new Fixture()) {
            when(f.clusterId.get(anyLong(), any(TimeUnit.class))).thenAnswer(invocation -> {
                Thread.sleep(2500);
                return "private-cluster-id";
            });
            Health health = f.indicator.health();
            assertNotEquals(Status.UP, health.getStatus());
            assertFalse(health.toString().contains(SECRET));
            assertTrue(awaitReleased(f, 5000));
            assertCloseDuration(f.admin);
        }
    }

    @Test
    void destroyAbandonsInFlightProbeAndKeepsPoolBounded() throws Exception {
        try (Fixture f = new Fixture()) {
            CountDownLatch createEntered = new CountDownLatch(1);
            CountDownLatch releaseCreate = new CountDownLatch(1);
            f.createHook = () -> {
                createEntered.countDown();
                assertTrue(releaseCreate.await(5, TimeUnit.SECONDS));
            };
            ExecutorService caller = Executors.newSingleThreadExecutor();
            try {
                Future<Health> pending = caller.submit(f.indicator::health);
                assertTrue(createEntered.await(2, TimeUnit.SECONDS));
                f.indicator.destroy();
                Health health = pending.get(3, TimeUnit.SECONDS);
                assertNotEquals(Status.UP, health.getStatus());
                releaseCreate.countDown();
                assertTrue(awaitReleased(f, 5000));
                assertTrue(f.indicator.probePoolSize() <= 1);
                Health afterStop = f.indicator.health();
                assertUnknown(afterStop, "STOPPED");
            } finally {
                releaseCreate.countDown();
                caller.shutdownNow();
            }
        }
    }

    @Test
    void realAdminClientReleasesItsIoThreadAfterUnresponsiveLoopbackPeer() throws Exception {
        // A listening socket with no accept/response is an actual stalled Kafka
        // transport, not a broker or physical Core/Test19 certification claim.
        String clientId = "issue260-health-release-" + Long.toUnsignedString(System.nanoTime());
        AtomicReference<List<Thread>> owned = new AtomicReference<>(List.of());
        try (ServerSocket peer = new ServerSocket(0, 1, InetAddress.getByName("127.0.0.1"))) {
            KafkaHealthIndicator indicator = new KafkaHealthIndicator(
                    "127.0.0.1:" + peer.getLocalPort(),
                    properties -> {
                        properties.put(AdminClientConfig.CLIENT_ID_CONFIG, clientId);
                        AdminClient client = AdminClient.create(properties);
                        owned.set(Thread.getAllStackTraces().keySet().stream()
                                .filter(thread -> thread.getName().contains(clientId)).toList());
                        return client;
                    });
            try {
                long start = System.nanoTime();
                Health health = indicator.health();
                assertNotEquals(Status.UP, health.getStatus());
                assertEquals(CoreSupportabilityEventCodes.KAFKA_HEALTH_PROBE_FAILED, health.getDetails().get("reasonCode"));
                assertFalse(health.toString().contains(SECRET));
                assertTrue(elapsedMillis(start) < 2800);
                assertFalse(owned.get().isEmpty(), "real Kafka I/O thread must have been observed");
                for (Thread thread : owned.get()) {
                    thread.join(500); // Positive finite wait on this test's own resource.
                    assertFalse(thread.isAlive(), "health must release its real Kafka I/O thread");
                }
            } finally {
                indicator.destroy();
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

    private static void assertUnknown(Health health, String probeState) {
        assertEquals(Status.UNKNOWN, health.getStatus());
        assertEquals("kafka", health.getDetails().get("dependency"));
        assertEquals(CoreSupportabilityEventCodes.KAFKA_HEALTH_PROBE_FAILED, health.getDetails().get("reasonCode"));
        assertEquals(probeState, health.getDetails().get("probeState"));
        assertFalse(health.toString().contains(SECRET));
        assertFalse(health.getDetails().containsKey("nodeCount"));
    }

    private static boolean awaitReleased(Fixture f, long timeoutMillis) throws InterruptedException {
        long deadline = System.nanoTime() + TimeUnit.MILLISECONDS.toNanos(timeoutMillis);
        while (System.nanoTime() < deadline) {
            if (f.released.get()) {
                return true;
            }
            Thread.sleep(10);
        }
        return f.released.get();
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
        final AtomicBoolean released = new AtomicBoolean();
        final AtomicInteger createCount = new AtomicInteger();
        final KafkaHealthIndicator indicator;
        Properties properties;
        volatile long creationDelayMillis;
        volatile RuntimeException creationFailure;
        volatile CreateHook createHook;

        Fixture() {
            when(admin.describeCluster()).thenReturn(cluster);
            when(cluster.nodes()).thenReturn(nodes);
            when(cluster.clusterId()).thenReturn(clusterId);
            when(nodes.get(anyLong(), any(TimeUnit.class))).thenReturn(NODES);
            when(clusterId.get(anyLong(), any(TimeUnit.class))).thenReturn("private-cluster-id");
            // Exercise the actual legacy Admin.close() delegation without blocking
            // for its Long.MAX_VALUE duration on a physical broker.
            doCallRealMethod().when(admin).close();
            doAnswer(invocation -> {
                released.set(true);
                return null;
            }).when(admin).close(any(Duration.class));
            indicator = new KafkaHealthIndicator(SECRET, props -> {
                createCount.incrementAndGet();
                properties = props;
                CreateHook hook = createHook;
                if (hook != null) {
                    try {
                        hook.run();
                    } catch (RuntimeException runtime) {
                        throw runtime;
                    } catch (Exception checked) {
                        throw new IllegalStateException(checked);
                    }
                }
                long delay = creationDelayMillis;
                if (delay > 0) {
                    try {
                        Thread.sleep(delay);
                    } catch (InterruptedException interrupted) {
                        Thread.currentThread().interrupt();
                        throw new IllegalStateException(interrupted);
                    }
                }
                RuntimeException failure = creationFailure;
                if (failure != null) {
                    throw failure;
                }
                return admin;
            });
        }

        @Override
        public void close() {
            indicator.destroy();
        }
    }

    @FunctionalInterface
    private interface CreateHook {
        void run() throws Exception;
    }
}
