package com.platform.core.ingestion.authority;

import org.springframework.boot.autoconfigure.condition.ConditionalOnMissingBean;
import org.springframework.context.annotation.Bean;
import org.springframework.context.annotation.Configuration;

@Configuration
public class SensorAuthorityConfiguration {

    @Bean
    @ConditionalOnMissingBean(SensorAuthorityResolver.class)
    public SensorAuthorityResolver sensorAuthorityResolver() {
        return new FailClosedSensorAuthorityResolver();
    }
}
