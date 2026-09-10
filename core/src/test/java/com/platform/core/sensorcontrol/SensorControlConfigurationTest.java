package com.platform.core.sensorcontrol;

import com.platform.core.ingestion.authority.FailClosedSensorAuthorityResolver;
import com.platform.core.ingestion.authority.SensorAuthorityConfiguration;
import com.platform.core.ingestion.authority.SensorAuthorityResolver;
import org.junit.jupiter.api.Test;
import org.springframework.context.annotation.AnnotationConfigApplicationContext;
import org.springframework.jdbc.core.namedparam.NamedParameterJdbcTemplate;

import javax.sql.DataSource;

import static org.junit.jupiter.api.Assertions.assertInstanceOf;
import static org.junit.jupiter.api.Assertions.assertNotNull;
import static org.mockito.Mockito.mock;

class SensorControlConfigurationTest {

    @Test
    void postgresResolverIsProductionBeanAndFailClosedIsFallbackOnly() {
        AnnotationConfigApplicationContext context = new AnnotationConfigApplicationContext();
        DataSource dataSource = mock(DataSource.class);
        NamedParameterJdbcTemplate jdbc = mock(NamedParameterJdbcTemplate.class);
        context.getDefaultListableBeanFactory().registerSingleton("dataSource", dataSource);
        context.getDefaultListableBeanFactory().registerSingleton("namedParameterJdbcTemplate", jdbc);
        context.register(SensorControlRepository.class);
        context.register(SensorAuthorityMetrics.class);
        context.register(SensorControlConfiguration.class);
        context.register(SensorAuthorityConfiguration.class);
        context.refresh();

        SensorAuthorityResolver resolver = context.getBean(SensorAuthorityResolver.class);
        assertNotNull(resolver);
        assertInstanceOf(PostgresSensorAuthorityResolver.class, resolver);

        context.close();
    }

    @Test
    void failClosedRemainsWhenSensorControlConfigurationAbsent() {
        AnnotationConfigApplicationContext context = new AnnotationConfigApplicationContext();
        context.register(SensorAuthorityConfiguration.class);
        context.refresh();

        SensorAuthorityResolver resolver = context.getBean(SensorAuthorityResolver.class);
        assertInstanceOf(FailClosedSensorAuthorityResolver.class, resolver);
        context.close();
    }
}
