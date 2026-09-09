package com.platform.core.controller;

import org.slf4j.Logger;
import org.slf4j.LoggerFactory;
import org.springframework.beans.factory.annotation.Value;
import org.springframework.boot.actuate.health.HealthEndpoint;
import org.springframework.boot.actuate.health.HealthComponent;
import org.springframework.boot.actuate.health.Status;
import org.springframework.http.ResponseEntity;
import org.springframework.jdbc.core.JdbcTemplate;
import org.springframework.web.bind.annotation.GetMapping;
import org.springframework.web.bind.annotation.RequestMapping;
import org.springframework.web.bind.annotation.RestController;

import java.time.Instant;
import java.util.LinkedHashMap;
import java.util.Map;
import java.util.Set;

/**
 * Public version probe plus privileged connectivity status.
 * Connectivity responses are intentionally bounded: status codes only — never exception text,
 * connection URLs, hostnames, credentials, filesystem paths, stack traces, or Actuator detail maps.
 */
@RestController
@RequestMapping("/api/v1")
public class VersionController {

    private static final Logger log = LoggerFactory.getLogger(VersionController.class);

    private static final Set<String> BOUNDED_STATUSES = Set.of("UP", "DOWN", "UNKNOWN", "OUT_OF_SERVICE");

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

    /**
     * Bounded datastore connectivity snapshot. Authorization is enforced by the HTTP security
     * chain (default-deny until a canonical Platform permission exists). Response bodies never
     * embed backend exception material or Actuator component detail maps.
     */
    @GetMapping("/health/connectivity")
    public ResponseEntity<Map<String, Object>> getConnectivityHealth() {
        Map<String, Object> result = new LinkedHashMap<>();
        result.put("timestamp", Instant.now().toString());
        result.put("node_id", nodeId);

        Map<String, Object> services = new LinkedHashMap<>();
        boolean allHealthy = true;

        Map<String, Object> postgres = probePostgres();
        services.put("postgres", postgres);
        services.put("postgresql", postgres);
        if (!isUp(postgres)) {
            allHealthy = false;
        }

        if (healthEndpoint != null) {
            try {
                HealthComponent overall = healthEndpoint.health();
                String overallStatus = boundedStatus(overall.getStatus());
                result.put("overall_status", overallStatus);
                if (!Status.UP.getCode().equals(overallStatus)) {
                    allHealthy = false;
                }

                if (overall instanceof org.springframework.boot.actuate.health.CompositeHealth composite) {
                    for (Map.Entry<String, HealthComponent> entry : composite.getComponents().entrySet()) {
                        String name = entry.getKey();
                        String status = boundedStatus(entry.getValue().getStatus());
                        Map<String, Object> statusOnly = Map.of("status", status);
                        if (!Status.UP.getCode().equals(status)) {
                            allHealthy = false;
                        }

                        if ("clickHouse".equalsIgnoreCase(name)) {
                            services.put("clickhouse", statusOnly);
                            services.put("clickHouse", statusOnly);
                        } else if ("redis".equalsIgnoreCase(name) || "valkey".equalsIgnoreCase(name)) {
                            services.put("valkey", statusOnly);
                            services.put("redis", statusOnly);
                        } else if ("kafka".equalsIgnoreCase(name)) {
                            services.put("kafka", statusOnly);
                        } else if (!"db".equalsIgnoreCase(name) && !"diskSpace".equalsIgnoreCase(name)
                                && !"ping".equalsIgnoreCase(name) && !"livenessState".equalsIgnoreCase(name)
                                && !"readinessState".equalsIgnoreCase(name)) {
                            // Preserve unknown named components as status-only; never copy Health details.
                            services.put(name, statusOnly);
                        }
                    }
                }
            } catch (Exception ex) {
                log.warn("Connectivity health aggregation failed: {}", ex.getClass().getSimpleName());
                result.put("overall_status", "DOWN");
                allHealthy = false;
            }
        } else {
            result.put("overall_status", "UNKNOWN");
            allHealthy = false;
        }

        result.put("services", services);
        result.put("status", allHealthy ? "UP" : "DEGRADED");

        return allHealthy ? ResponseEntity.ok(result) : ResponseEntity.status(503).body(result);
    }

    private Map<String, Object> probePostgres() {
        if (jdbcTemplate == null) {
            return Map.of("status", "UNKNOWN");
        }
        try {
            Integer pgCheck = jdbcTemplate.queryForObject("SELECT 1", Integer.class);
            return Map.of("status", pgCheck != null && pgCheck == 1 ? "UP" : "DOWN");
        } catch (Exception ex) {
            // Never return Exception#getMessage(), JDBC URLs, hostnames, or SQLState text to clients.
            log.warn("PostgreSQL connectivity probe failed: {}", ex.getClass().getSimpleName());
            return Map.of("status", "DOWN");
        }
    }

    private static boolean isUp(Map<String, Object> component) {
        return Status.UP.getCode().equals(String.valueOf(component.get("status")));
    }

    private static String boundedStatus(Status status) {
        if (status == null) {
            return "UNKNOWN";
        }
        String code = status.getCode();
        if (code != null && BOUNDED_STATUSES.contains(code)) {
            return code;
        }
        return "UNKNOWN";
    }
}
