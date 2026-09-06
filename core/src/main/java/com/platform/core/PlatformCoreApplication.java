package com.platform.core;

import org.slf4j.Logger;
import org.slf4j.LoggerFactory;
import org.slf4j.MDC;
import org.springframework.beans.factory.annotation.Value;
import org.springframework.boot.CommandLineRunner;
import org.springframework.boot.SpringApplication;
import org.springframework.boot.autoconfigure.SpringBootApplication;
import org.springframework.context.annotation.Bean;

import java.util.UUID;

@SpringBootApplication
public class PlatformCoreApplication {

    private static final Logger log = LoggerFactory.getLogger(PlatformCoreApplication.class);

    @Value("${platform.version:0.1.0-dev}")
    private String version;

    @Value("${platform.node-id:core-node-01}")
    private String nodeId;

    @Value("${platform.cluster-mode:STANDALONE}")
    private String clusterMode;

    public static void main(String[] args) {
        SpringApplication.run(PlatformCoreApplication.class, args);
    }

    @Bean
    public CommandLineRunner startupLogger() {
        return args -> {
            String correlationId = UUID.randomUUID().toString();
            MDC.put("correlation_id", correlationId);
            MDC.put("component", "platform-core");
            MDC.put("node_id", nodeId);
            MDC.put("event_code", "PLT-CORE-0001");
            MDC.put("version", version);

            log.info("Platform Core application initialized successfully. Version: {}, Node: {}, Mode: {}",
                    version, nodeId, clusterMode);

            MDC.clear();
        };
    }
}
