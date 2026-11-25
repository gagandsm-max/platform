package com.platform.core.sensorcontrol;

import com.platform.core.ingestion.authority.SensorAuthorityResolver;
import com.platform.core.sensorcontrol.pollgrant.SensorPollGrantAccess;
import com.platform.core.sensorcontrol.pollgrant.SensorPollGrantEvaluationService;
import com.platform.core.sensorcontrol.pollgrant.SensorPollGrantRepository;
import org.springframework.context.annotation.Bean;
import org.springframework.context.annotation.Configuration;
import org.springframework.context.annotation.Primary;
import org.springframework.jdbc.core.namedparam.NamedParameterJdbcTemplate;

/**
 * Registers the production PostgreSQL-backed SensorAuthorityResolver and the
 * internal Issue #229 G1b poll-grant evaluation seam.
 *
 * <p>FailClosedSensorAuthorityResolver remains {@code @ConditionalOnMissingBean} fallback only
 * when no SensorAuthorityResolver is present. Bean method name must differ from the fail-closed
 * factory method to avoid BeanDefinitionOverrideException.
 *
 * <p>Poll-grant beans expose no public endpoint, scheduler, delivery path or G2 admission.
 * {@link SensorPollGrantEvaluationService} owns the Spring read-transaction boundary;
 * {@link SensorPollGrantAccess} remains a plain accessor that requires that caller-held TX.
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

    @Bean
    public SensorPollGrantRepository sensorPollGrantRepository(NamedParameterJdbcTemplate jdbc) {
        return new SensorPollGrantRepository(jdbc);
    }

    @Bean
    public SensorPollGrantAccess sensorPollGrantAccess(
            SensorControlRepository sensors,
            SensorPollGrantRepository grants
    ) {
        return new SensorPollGrantAccess(sensors, grants);
    }

    @Bean
    public SensorPollGrantEvaluationService sensorPollGrantEvaluationService(
            SensorPollGrantAccess access
    ) {
        return new SensorPollGrantEvaluationService(access);
    }
}
