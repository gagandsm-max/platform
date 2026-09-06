package com.platform.core.health;

import org.slf4j.Logger;
import org.slf4j.LoggerFactory;
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

            if (response.statusCode() == 200 && response.body().trim().equals("Ok.")) {
                return Health.up()
                        .withDetail("endpoint", clickhouseHttpUrl)
                        .withDetail("response", "Ok.")
                        .build();
            } else {
                return Health.down()
                        .withDetail("endpoint", clickhouseHttpUrl)
                        .withDetail("statusCode", response.statusCode())
                        .withDetail("response", response.body())
                        .build();
            }
        } catch (Exception e) {
            log.warn("ClickHouse health probe failed against {}: {}", clickhouseHttpUrl, e.getMessage());
            return Health.down()
                    .withDetail("endpoint", clickhouseHttpUrl)
                    .withDetail("error", e.getClass().getSimpleName() + ": " + e.getMessage())
                    .build();
        }
    }
}
