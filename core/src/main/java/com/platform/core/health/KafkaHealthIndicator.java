package com.platform.core.health;

import com.platform.core.logging.CoreSupportabilityEventCodes;
import com.platform.core.logging.SupportabilityMdcKeys;
import org.apache.kafka.clients.admin.AdminClient;
import org.apache.kafka.clients.admin.AdminClientConfig;
import org.apache.kafka.clients.admin.DescribeClusterResult;
import org.apache.kafka.common.errors.InterruptException;
import org.slf4j.Logger;
import org.slf4j.LoggerFactory;
import org.slf4j.MDC;
import org.springframework.beans.factory.DisposableBean;
import org.springframework.beans.factory.annotation.Value;
import org.springframework.boot.actuate.health.Health;
import org.springframework.boot.actuate.health.HealthIndicator;
import org.springframework.stereotype.Component;

import java.time.Duration;
import java.util.Properties;
import java.util.concurrent.ArrayBlockingQueue;
import java.util.concurrent.CompletableFuture;
import java.util.concurrent.ExecutionException;
import java.util.concurrent.RejectedExecutionException;
import java.util.concurrent.ThreadPoolExecutor;
import java.util.concurrent.TimeUnit;
import java.util.concurrent.TimeoutException;
import java.util.concurrent.atomic.AtomicBoolean;
import java.util.concurrent.atomic.AtomicInteger;
import java.util.function.Function;

/**
 * Kafka dependency health with a lifecycle-owned single-slot probe worker.
 *
 * <p>Synchronous {@code AdminClient.create} / bootstrap DNS is not reliably preemptible by
 * {@code request.timeout.ms}, future deadlines or {@code Thread.interrupt()}. The Actuator HTTP
 * thread therefore never constructs the client itself: it only waits a finite budget for the
 * bounded probe worker. Stale/abandoned work cannot publish UP; the worker closes any late client.
 *
 * <p>This does not claim a universal three-second HTTP guarantee for every JVM/OS resolver.
 */
@Component("kafka")
public class KafkaHealthIndicator implements HealthIndicator, DisposableBean {

    private static final Logger log = LoggerFactory.getLogger(KafkaHealthIndicator.class);
    // Leave 500ms below Test19's unchanged three-second request allowance.
    private static final long PROBE_BUDGET_MILLIS = 2500;
    private static final long CLOSE_CAP_MILLIS = 100;
    private static final long WORK_BUDGET_NANOS = TimeUnit.MILLISECONDS.toNanos(PROBE_BUDGET_MILLIS - CLOSE_CAP_MILLIS);
    private static final String PROBE_THREAD_NAME = "core-kafka-health-probe";

    @Value("${spring.kafka.bootstrap-servers:kafka:9092}")
    private String bootstrapServers;

    private final Function<Properties, AdminClient> adminClientFactory;
    private final ThreadPoolExecutor probeExecutor;
    private final Object probeLock = new Object();
    private final AtomicInteger probeThreadsLive = new AtomicInteger();
    private volatile boolean stopped;
    private ProbeSession inFlight;

    public KafkaHealthIndicator() {
        this(AdminClient::create);
    }

    // Package-visible for focused tests (factory must be safe to call on the probe worker thread).
    KafkaHealthIndicator(Function<Properties, AdminClient> adminClientFactory) {
        this.adminClientFactory = adminClientFactory;
        // One worker + one handoff slot. Coalesced in-flight probes prevent a second
        // concurrent create; the handoff slot only absorbs the idle-transition race.
        this.probeExecutor = new ThreadPoolExecutor(
                1,
                1,
                0L,
                TimeUnit.MILLISECONDS,
                new ArrayBlockingQueue<>(1),
                runnable -> {
                    Thread thread = new Thread(runnable, PROBE_THREAD_NAME);
                    thread.setDaemon(true);
                    return thread;
                },
                new ThreadPoolExecutor.AbortPolicy());
        this.probeExecutor.prestartCoreThread();
    }

    KafkaHealthIndicator(String bootstrapServers, Function<Properties, AdminClient> adminClientFactory) {
        this(adminClientFactory);
        this.bootstrapServers = bootstrapServers;
    }

    @Override
    public Health health() {
        long started = System.nanoTime();
        if (stopped) {
            return incomplete("STOPPED");
        }
        if (Thread.currentThread().isInterrupted()) {
            return failed();
        }

        ProbeSession session;
        synchronized (probeLock) {
            if (stopped) {
                return incomplete("STOPPED");
            }
            if (inFlight != null && !inFlight.future.isDone()) {
                session = inFlight;
            } else {
                session = new ProbeSession();
                try {
                    probeExecutor.execute(() -> runProbe(session));
                    inFlight = session;
                } catch (RejectedExecutionException rejected) {
                    // Worker may still be exiting after clearing inFlight, or stuck after
                    // abandon. Prefer an existing in-flight session; otherwise fail closed.
                    if (inFlight != null && !inFlight.future.isDone()) {
                        session = inFlight;
                    } else {
                        return incomplete("BUSY");
                    }
                }
            }
        }

        try {
            ProbeResult result = session.future.get(remainingWorkNanos(started), TimeUnit.NANOSECONDS);
            if (result.healthy()
                    && !Thread.currentThread().isInterrupted()
                    && System.nanoTime() - started < TimeUnit.MILLISECONDS.toNanos(PROBE_BUDGET_MILLIS)) {
                return Health.up()
                        .withDetail("dependency", "kafka")
                        .withDetail("nodeCount", result.nodeCount())
                        .build();
            }
            if (result.incomplete()) {
                return incomplete(result.reason());
            }
            return failed();
        } catch (TimeoutException timeout) {
            session.abandoned.set(true);
            return incomplete("DEADLINE");
        } catch (InterruptedException interrupted) {
            session.abandoned.set(true);
            Thread.currentThread().interrupt();
            return failed();
        } catch (ExecutionException execution) {
            session.abandoned.set(true);
            return failed();
        }
    }

    @Override
    public void destroy() {
        stopped = true;
        ProbeSession session;
        synchronized (probeLock) {
            session = inFlight;
            if (session != null) {
                session.abandoned.set(true);
            }
        }
        probeExecutor.shutdown();
        try {
            if (!probeExecutor.awaitTermination(PROBE_BUDGET_MILLIS + CLOSE_CAP_MILLIS, TimeUnit.MILLISECONDS)) {
                probeExecutor.shutdownNow();
                probeExecutor.awaitTermination(CLOSE_CAP_MILLIS, TimeUnit.MILLISECONDS);
            }
        } catch (InterruptedException interrupted) {
            probeExecutor.shutdownNow();
            Thread.currentThread().interrupt();
        }
        if (session != null) {
            session.future.complete(ProbeResult.incomplete("STOPPED"));
        }
    }

    // Package-visible observability for focused resource-bound tests.
    int probePoolSize() {
        return probeExecutor.getPoolSize();
    }

    int probeThreadsLive() {
        return probeThreadsLive.get();
    }

    boolean hasInFlightProbe() {
        synchronized (probeLock) {
            return inFlight != null && !inFlight.future.isDone();
        }
    }

    private void runProbe(ProbeSession session) {
        probeThreadsLive.incrementAndGet();
        long started = System.nanoTime();
        AdminClient adminClient = null;
        boolean healthy = false;
        boolean interrupted = false;
        int nodeCount = 0;
        try {
            if (session.abandoned.get() || stopped || Thread.currentThread().isInterrupted()) {
                return;
            }
            Properties props = new Properties();
            props.put(AdminClientConfig.BOOTSTRAP_SERVERS_CONFIG, bootstrapServers);
            props.put(AdminClientConfig.REQUEST_TIMEOUT_MS_CONFIG, "1000");
            props.put(AdminClientConfig.DEFAULT_API_TIMEOUT_MS_CONFIG, Long.toString(PROBE_BUDGET_MILLIS - CLOSE_CAP_MILLIS));

            remainingWorkNanos(started);
            // Includes synchronous bootstrap/DNS work. This runs only on the single probe worker;
            // the Actuator HTTP thread is not held by the constructor.
            adminClient = adminClientFactory.apply(props);
            if (session.abandoned.get() || stopped || Thread.currentThread().isInterrupted()) {
                return;
            }
            remainingWorkNanos(started);
            DescribeClusterResult cluster = adminClient.describeCluster();
            nodeCount = cluster.nodes().get(remainingWorkNanos(started), TimeUnit.NANOSECONDS).size();
            remainingWorkNanos(started); // Do not advance after a late first result.
            // Prove connectivity without exposing cluster identity in health details.
            cluster.clusterId().get(remainingWorkNanos(started), TimeUnit.NANOSECONDS);
            remainingWorkNanos(started); // A late successful result is not healthy.
            if (session.abandoned.get() || stopped || Thread.currentThread().isInterrupted()) {
                return;
            }
            healthy = true;
        } catch (Exception failure) {
            interrupted = failure instanceof InterruptedException || failure instanceof InterruptException;
        } finally {
            if (adminClient != null) {
                try {
                    adminClient.close(closeAllowance(started));
                } catch (Exception failure) {
                    healthy = false;
                    interrupted |= failure instanceof InterruptedException || failure instanceof InterruptException;
                }
            }
            if (interrupted) {
                Thread.currentThread().interrupt();
            }
            ProbeResult result;
            if (healthy
                    && !session.abandoned.get()
                    && !stopped
                    && !Thread.currentThread().isInterrupted()
                    && System.nanoTime() - started < TimeUnit.MILLISECONDS.toNanos(PROBE_BUDGET_MILLIS)) {
                result = ProbeResult.up(nodeCount);
            } else if (session.abandoned.get() || stopped) {
                result = ProbeResult.incomplete(stopped ? "STOPPED" : "DEADLINE");
            } else {
                result = ProbeResult.failed();
            }
            session.future.complete(result);
            synchronized (probeLock) {
                if (inFlight == session) {
                    inFlight = null;
                }
            }
            probeThreadsLive.decrementAndGet();
        }
    }

    private Health failed() {
        MDC.put(SupportabilityMdcKeys.EVENT_CODE, CoreSupportabilityEventCodes.KAFKA_HEALTH_PROBE_FAILED);
        try {
            log.warn("Kafka health probe failed");
        } finally {
            MDC.remove(SupportabilityMdcKeys.EVENT_CODE);
        }
        return Health.down()
                .withDetail("dependency", "kafka")
                .withDetail("reasonCode", CoreSupportabilityEventCodes.KAFKA_HEALTH_PROBE_FAILED)
                .build();
    }

    private Health incomplete(String probeState) {
        MDC.put(SupportabilityMdcKeys.EVENT_CODE, CoreSupportabilityEventCodes.KAFKA_HEALTH_PROBE_FAILED);
        try {
            log.warn("Kafka health probe incomplete");
        } finally {
            MDC.remove(SupportabilityMdcKeys.EVENT_CODE);
        }
        // UNKNOWN = incomplete evidence (deadline/backpressure/shutdown). Not UP.
        // Confirmed broker failure remains DOWN. No universal HTTP-code claim is made here:
        // Spring's default StatusHttpMapper leaves UNKNOWN as non-503 unless separately mapped.
        return Health.unknown()
                .withDetail("dependency", "kafka")
                .withDetail("reasonCode", CoreSupportabilityEventCodes.KAFKA_HEALTH_PROBE_FAILED)
                .withDetail("probeState", probeState)
                .build();
    }

    private static long remainingWorkNanos(long started) throws InterruptedException, TimeoutException {
        if (Thread.currentThread().isInterrupted()) {
            throw new InterruptedException();
        }
        long remaining = WORK_BUDGET_NANOS - (System.nanoTime() - started);
        if (remaining <= 0) {
            throw new TimeoutException();
        }
        return remaining;
    }

    private static Duration closeAllowance(long started) {
        long remaining = TimeUnit.MILLISECONDS.toNanos(PROBE_BUDGET_MILLIS) - (System.nanoTime() - started);
        // Kafka 3.7.1 truncates to milliseconds before join: zero/sub-ms means
        // join(0), an unbounded wait. If creation/work already overran, allow
        // one whole millisecond for release rather than pretend it is preemptible.
        long millis = Math.max(1, Math.min(CLOSE_CAP_MILLIS, TimeUnit.NANOSECONDS.toMillis(remaining)));
        return Duration.ofMillis(millis);
    }

    private static final class ProbeSession {
        private final CompletableFuture<ProbeResult> future = new CompletableFuture<>();
        private final AtomicBoolean abandoned = new AtomicBoolean();
    }

    private record ProbeResult(boolean healthy, boolean incomplete, int nodeCount, String reason) {
        static ProbeResult up(int nodeCount) {
            return new ProbeResult(true, false, nodeCount, null);
        }

        static ProbeResult failed() {
            return new ProbeResult(false, false, 0, null);
        }

        static ProbeResult incomplete(String reason) {
            return new ProbeResult(false, true, 0, reason);
        }
    }
}
