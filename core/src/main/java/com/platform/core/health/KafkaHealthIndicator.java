package com.platform.core.health;

import com.platform.core.logging.CoreSupportabilityEventCodes;
import com.platform.core.logging.SupportabilityMdcKeys;
import org.apache.kafka.clients.admin.AdminClient;
import org.apache.kafka.clients.admin.AdminClientConfig;
import org.apache.kafka.clients.admin.DescribeClusterResult;
import org.slf4j.Logger;
import org.slf4j.LoggerFactory;
import org.slf4j.MDC;
import org.springframework.beans.factory.annotation.Value;
import org.springframework.boot.actuate.health.Health;
import org.springframework.boot.actuate.health.HealthIndicator;
import org.springframework.stereotype.Component;

import java.util.Properties;
import java.util.concurrent.TimeUnit;

@Component("kafka")
public class KafkaHealthIndicator implements HealthIndicator {

    private static final Logger log = LoggerFactory.getLogger(KafkaHealthIndicator.class);

    @Value("${spring.kafka.bootstrap-servers:kafka:9092}")
    private String bootstrapServers;

    @Override
    public Health health() {
        Properties props = new Properties();
        props.put(AdminClientConfig.BOOTSTRAP_SERVERS_CONFIG, bootstrapServers);
        props.put(AdminClientConfig.REQUEST_TIMEOUT_MS_CONFIG, "3000");
        props.put(AdminClientConfig.DEFAULT_API_TIMEOUT_MS_CONFIG, "3000");

        try (AdminClient adminClient = AdminClient.create(props)) {
            DescribeClusterResult cluster = adminClient.describeCluster();
            int nodeCount = cluster.nodes().get(3, TimeUnit.SECONDS).size();
            // clusterId fetched to prove connectivity without exposing it in health details.
            cluster.clusterId().get(3, TimeUnit.SECONDS);

            return Health.up()
                    .withDetail("dependency", "kafka")
                    .withDetail("nodeCount", nodeCount)
                    .build();
        } catch (Exception e) {
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
    }
}
