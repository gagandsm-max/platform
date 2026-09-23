package com.platform.core.sensorcontrol;

import com.platform.core.security.http.SecurityAuditEmitter;
import org.junit.jupiter.api.Test;
import org.springframework.context.annotation.AnnotationConfigApplicationContext;

import static org.junit.jupiter.api.Assertions.assertEquals;
import static org.junit.jupiter.api.Assertions.assertNotNull;
import static org.mockito.Mockito.mock;

/**
 * Regression for CI #483: Spring must select the three-argument production constructor
 * when a package-private SecureRandom test constructor also exists.
 */
class SensorEnrollmentAuthorityServiceSpringWiringTest {

    @Test
    void springSelectsThreeArgumentProductionConstructor() {
        assertEquals(2, SensorEnrollmentAuthorityService.class.getDeclaredConstructors().length);

        AnnotationConfigApplicationContext context = new AnnotationConfigApplicationContext();
        context.getDefaultListableBeanFactory().registerSingleton(
                "sensorEnrollmentAuthorityRepository", mock(SensorEnrollmentAuthorityRepository.class));
        context.getDefaultListableBeanFactory().registerSingleton(
                "sensorControlRepository", mock(SensorControlRepository.class));
        context.getDefaultListableBeanFactory().registerSingleton(
                "securityAuditEmitter", mock(SecurityAuditEmitter.class));
        context.register(SensorEnrollmentAuthorityService.class);
        context.refresh();

        assertNotNull(context.getBean(SensorEnrollmentAuthorityService.class));
        context.close();
    }
}
