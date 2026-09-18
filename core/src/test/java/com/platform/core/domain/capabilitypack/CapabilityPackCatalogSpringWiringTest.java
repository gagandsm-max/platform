package com.platform.core.domain.capabilitypack;

import com.platform.core.api.CapabilityPackController;
import com.platform.core.persistence.CapabilityPackRepository;
import org.junit.jupiter.api.Test;
import org.springframework.context.annotation.AnnotationConfigApplicationContext;
import org.springframework.jdbc.core.namedparam.NamedParameterJdbcTemplate;

import static org.junit.jupiter.api.Assertions.assertEquals;
import static org.junit.jupiter.api.Assertions.assertNotNull;
import static org.mockito.Mockito.mock;

/**
 * Focused Spring wiring proof: CatalogService has a single injection constructor
 * and wires with Controller without ambiguous-constructor failure.
 */
class CapabilityPackCatalogSpringWiringTest {

    @Test
    void catalogServiceAndControllerWireWithoutAmbiguousConstructor() {
        assertEquals(
                1,
                CapabilityPackCatalogService.class.getDeclaredConstructors().length,
                "CapabilityPackCatalogService must expose exactly one constructor for Spring injection"
        );

        AnnotationConfigApplicationContext context = new AnnotationConfigApplicationContext();
        NamedParameterJdbcTemplate jdbc = mock(NamedParameterJdbcTemplate.class);
        context.getDefaultListableBeanFactory().registerSingleton("namedParameterJdbcTemplate", jdbc);
        context.register(CapabilityPackRepository.class);
        context.register(CapabilityPackCatalogService.class);
        context.register(CapabilityPackController.class);
        context.refresh();

        assertNotNull(context.getBean(CapabilityPackCatalogService.class));
        assertNotNull(context.getBean(CapabilityPackController.class));
        context.close();
    }
}
