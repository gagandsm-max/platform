package com.platform.core.sensorcontrol;

import com.platform.core.ingestion.authority.SensorAuthorityResolver;
import org.springframework.context.annotation.Bean;
import org.springframework.context.annotation.Configuration;

/**
 * Registers the production PostgreSQL-backed SensorAuthorityResolver.
 * FailClosedSensorAuthorityResolver remains {@code @ConditionalOnMissingBean} fallback only.
 */
@Configuration
public class SensorControlConfiguration {

    @Bean
    public SensorAuthorityResolver sensorAuthorityResolver(
            SensorControlRepository repository,
            SensorAuthorityMetrics metrics
    ) {
        return new PostgresSensorAuthorityResolver(repository, metrics);
    }
}
