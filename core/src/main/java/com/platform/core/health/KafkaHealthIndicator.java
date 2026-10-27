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
import org.springframework.beans.factory.annotation.Value;
import org.springframework.boot.actuate.health.Health;
import org.springframework.boot.actuate.health.HealthIndicator;
import org.springframework.stereotype.Component;

import java.time.Duration;
import java.util.Properties;
import java.util.concurrent.TimeUnit;
import java.util.concurrent.TimeoutException;

@Component("kafka")
public class KafkaHealthIndicator implements HealthIndicator {

    private static final Logger log = LoggerFactory.getLogger(KafkaHealthIndicator.class);
    // Leave 500ms below Test19's unchanged three-second request allowance.
    private static final long PROBE_BUDGET_MILLIS = 2500;
    private static final long CLOSE_CAP_MILLIS = 100;
    private static final long WORK_BUDGET_NANOS = TimeUnit.MILLISECONDS.toNanos(PROBE_BUDGET_MILLIS - CLOSE_CAP_MILLIS);

    @Value("${spring.kafka.bootstrap-servers:kafka:9092}")
    private String bootstrapServers;

    @Override
    public Health health() {
        long started = System.nanoTime();
        AdminClient adminClient = null;
        boolean healthy = false;
        boolean interrupted = false;
        int nodeCount = 0;
        try {
            Properties props = new Properties();
            props.put(AdminClientConfig.BOOTSTRAP_SERVERS_CONFIG, bootstrapServers);
            props.put(AdminClientConfig.REQUEST_TIMEOUT_MS_CONFIG, "1000");
            props.put(AdminClientConfig.DEFAULT_API_TIMEOUT_MS_CONFIG, Long.toString(PROBE_BUDGET_MILLIS - CLOSE_CAP_MILLIS));

            remainingWorkNanos(started);
            // Includes synchronous bootstrap/DNS work: charged to the budget,
            // but this call cannot be preempted without a broader lifecycle change.
            adminClient = AdminClient.create(props);
            remainingWorkNanos(started);
            DescribeClusterResult cluster = adminClient.describeCluster();
            nodeCount = cluster.nodes().get(remainingWorkNanos(started), TimeUnit.NANOSECONDS).size();
            remainingWorkNanos(started); // Do not advance after a late first result.
            // Prove connectivity without exposing cluster identity in health details.
            cluster.clusterId().get(remainingWorkNanos(started), TimeUnit.NANOSECONDS);
            remainingWorkNanos(started); // A late successful result is not healthy.
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
            // Restore after cleanup, so joining the I/O thread is still attempted.
            if (interrupted) Thread.currentThread().interrupt();
        }
        if (healthy && !Thread.currentThread().isInterrupted()
                && System.nanoTime() - started < TimeUnit.MILLISECONDS.toNanos(PROBE_BUDGET_MILLIS)) {
            return Health.up()
                    .withDetail("dependency", "kafka")
                    .withDetail("nodeCount", nodeCount)
                    .build();
        }
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

    private static long remainingWorkNanos(long started) throws InterruptedException, TimeoutException {
        if (Thread.currentThread().isInterrupted()) throw new InterruptedException();
        long remaining = WORK_BUDGET_NANOS - (System.nanoTime() - started);
        if (remaining <= 0) throw new TimeoutException();
        return remaining;
    }

    private static Duration closeAllowance(long started) {
        long remaining = TimeUnit.MILLISECONDS.toNanos(PROBE_BUDGET_MILLIS) - (System.nanoTime() - started);
        // Kafka3.7.1 truncates to milliseconds before join: zero/sub-ms means
        // join(0), an unbounded wait. If creation/work already overran, allow
        // one whole millisecond for release rather than pretend it is preemptible.
        long millis = Math.max(1, Math.min(CLOSE_CAP_MILLIS, TimeUnit.NANOSECONDS.toMillis(remaining)));
        return Duration.ofMillis(millis);
    }
}
