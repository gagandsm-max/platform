package com.platform.core.sensorcontrol;

import com.platform.core.ingestion.authority.SensorAuthorityResolver;
import org.springframework.context.annotation.Bean;
import org.springframework.context.annotation.Configuration;
import org.springframework.context.annotation.Primary;

/**
 * Registers the production PostgreSQL-backed SensorAuthorityResolver.
 * FailClosedSensorAuthorityResolver remains {@code @ConditionalOnMissingBean} fallback only
 * when no SensorAuthorityResolver is present. Bean method name must differ from the fail-closed
 * factory method to avoid BeanDefinitionOverrideException.
 */
@Configuration
public class SensorControlConfiguration {

    @Bean
    @Primary
    public SensorAuthorityResolver postgresSensorAuthorityResolver(
            SensorControlRepository repository,
            SensorAuthorityMetrics metrics
    ) {
        return new PostgresSensorAuthorityResolver(repository, metrics);
    }
}
