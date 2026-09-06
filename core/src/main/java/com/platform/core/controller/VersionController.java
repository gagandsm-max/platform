package com.platform.core.controller;

import com.platform.core.health.ClickHouseHealthIndicator;
import org.springframework.beans.factory.annotation.Value;
import org.springframework.boot.actuate.health.Health;
import org.springframework.boot.actuate.health.HealthComponent;
import org.springframework.boot.actuate.health.HealthEndpoint;
import org.springframework.boot.actuate.health.Status;
import org.springframework.http.ResponseEntity;
import org.springframework.jdbc.core.JdbcTemplate;
import org.springframework.web.bind.annotation.GetMapping;
import org.springframework.web.bind.annotation.RequestMapping;
import org.springframework.web.bind.annotation.RestController;

import java.time.Instant;
import java.util.LinkedHashMap;
import java.util.Map;

@RestController
@RequestMapping("/api/v1")
public class VersionController {

    @Value("${platform.version:0.1.0-dev}")
    private String version = "0.1.0-dev";

    @Value("${platform.node-id:core-node-01}")
    private String nodeId = "core-node-01";

    @Value("${platform.cluster-mode:STANDALONE}")
    private String clusterMode = "STANDALONE";

    private final HealthEndpoint healthEndpoint;
    private final JdbcTemplate jdbcTemplate;

    public VersionController() {
        this(null, null);
    }

    @org.springframework.beans.factory.annotation.Autowired
    public VersionController(HealthEndpoint healthEndpoint, JdbcTemplate jdbcTemplate) {
        this.healthEndpoint = healthEndpoint;
        this.jdbcTemplate = jdbcTemplate;
    }

    @GetMapping("/version")
    public ResponseEntity<Map<String, Object>> getVersion() {
        Map<String, Object> response = new LinkedHashMap<>();
        response.put("product", "Platform");
        response.put("version", version);
        response.put("node_id", nodeId);
        response.put("cluster_mode", clusterMode);
        response.put("status", "UP");
        response.put("timestamp", Instant.now().toString());
        return ResponseEntity.ok(response);
    }

    @GetMapping("/health/connectivity")
    public ResponseEntity<Map<String, Object>> getConnectivityHealth() {
        Map<String, Object> result = new LinkedHashMap<>();
        result.put("timestamp", Instant.now().toString());
        result.put("node_id", nodeId);

        Map<String, Object> services = new LinkedHashMap<>();

        // 1. PostgreSQL probe
        if (jdbcTemplate != null) {
            try {
                Integer pgCheck = jdbcTemplate.queryForObject("SELECT 1", Integer.class);
                Map<String, Object> pgStatus = Map.of("status", pgCheck != null && pgCheck == 1 ? "UP" : "DOWN");
                services.put("postgres", pgStatus);
                services.put("postgresql", pgStatus);
            } catch (Exception e) {
                Map<String, Object> pgStatus = Map.of("status", "DOWN", "error", e.getMessage());
                services.put("postgres", pgStatus);
                services.put("postgresql", pgStatus);
            }
        }

        // 2. Actuator health components (ClickHouse, Valkey/Redis, etc.)
        boolean allHealthy = true;
        if (healthEndpoint != null) {
            HealthComponent overall = healthEndpoint.health();
            result.put("overall_status", overall.getStatus().getCode());
            if (!Status.UP.equals(overall.getStatus())) {
                allHealthy = false;
            }

            if (overall instanceof org.springframework.boot.actuate.health.CompositeHealth composite) {
                composite.getComponents().forEach((name, component) -> {
                    String status = component.getStatus().getCode();
                    Map<String, Object> details = (component instanceof Health h) ? h.getDetails() : Map.of();
                    Map<String, Object> compMap = Map.of("status", status, "details", details);

                    if ("clickHouse".equalsIgnoreCase(name)) {
                        services.put("clickhouse", compMap);
                        services.put("clickHouse", compMap);
                    } else if ("redis".equalsIgnoreCase(name) || "valkey".equalsIgnoreCase(name)) {
                        services.put("valkey", compMap);
                        services.put("redis", compMap);
                    } else if (!"db".equalsIgnoreCase(name)) {
                        services.put(name, compMap);
                    }
                });
            }
        }

        result.put("services", services);
        result.put("components", services);
        result.put("status", allHealthy ? "UP" : "DEGRADED");

        return allHealthy ? ResponseEntity.ok(result) : ResponseEntity.status(503).body(result);
    }
}
