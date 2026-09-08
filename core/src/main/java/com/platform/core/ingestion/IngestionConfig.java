package com.platform.core.ingestion;

import org.apache.kafka.clients.admin.NewTopic;
import org.springframework.beans.factory.annotation.Value;
import org.springframework.context.annotation.Bean;
import org.springframework.context.annotation.Configuration;
import org.springframework.kafka.config.TopicBuilder;

@Configuration
public class IngestionConfig {

    @Bean
    public NewTopic telemetryNumericTopic(
            @Value("${platform.ingestion.kafka.topic:telemetry.numeric.v1}") String topicName,
            @Value("${platform.ingestion.kafka.partitions:3}") int partitions,
            @Value("${platform.ingestion.kafka.replicas:1}") int replicas
    ) {
        return TopicBuilder.name(topicName)
                .partitions(partitions)
                .replicas(replicas)
                .build();
    }
}
