package com.platform.core.health;

import com.platform.core.logging.CoreSupportabilityEventCodes;
import com.platform.core.logging.SupportabilityMdcKeys;
import org.slf4j.Logger;
import org.slf4j.LoggerFactory;
import org.slf4j.MDC;
import org.springframework.beans.factory.annotation.Value;
import org.springframework.boot.actuate.health.Health;
import org.springframework.boot.actuate.health.HealthIndicator;
import org.springframework.stereotype.Component;

import java.net.URI;
import java.net.http.HttpClient;
import java.net.http.HttpRequest;
import java.net.http.HttpResponse;
import java.time.Duration;

@Component("clickHouse")
public class ClickHouseHealthIndicator implements HealthIndicator {

    private static final Logger log = LoggerFactory.getLogger(ClickHouseHealthIndicator.class);

    @Value("${platform.clickhouse.http-url:http://clickhouse:8123}")
    private String clickhouseHttpUrl;

    private final HttpClient httpClient;

    public ClickHouseHealthIndicator() {
        this.httpClient = HttpClient.newBuilder()
                .connectTimeout(Duration.ofSeconds(3))
                .build();
    }

    // Package-visible for tests.
    ClickHouseHealthIndicator(HttpClient httpClient, String clickhouseHttpUrl) {
        this.httpClient = httpClient;
        this.clickhouseHttpUrl = clickhouseHttpUrl;
    }

    @Override
    public Health health() {
        try {
            URI pingUri = URI.create(clickhouseHttpUrl + "/ping");
            HttpRequest request = HttpRequest.newBuilder()
                    .uri(pingUri)
                    .timeout(Duration.ofSeconds(3))
                    .GET()
                    .build();

            HttpResponse<String> response = httpClient.send(request, HttpResponse.BodyHandlers.ofString());

            if (response.statusCode() == 200 && "Ok.".equals(response.body().trim())) {
                return Health.up()
                        .withDetail("dependency", "clickHouse")
                        .build();
            }
            return down();
        } catch (Exception e) {
            return down();
        }
    }

    private Health down() {
        MDC.put(SupportabilityMdcKeys.EVENT_CODE, CoreSupportabilityEventCodes.CLICKHOUSE_HEALTH_PROBE_FAILED);
        try {
            log.warn("ClickHouse health probe failed");
        } finally {
            MDC.remove(SupportabilityMdcKeys.EVENT_CODE);
        }
        return Health.down()
                .withDetail("dependency", "clickHouse")
                .withDetail("reasonCode", CoreSupportabilityEventCodes.CLICKHOUSE_HEALTH_PROBE_FAILED)
                .build();
    }
}
