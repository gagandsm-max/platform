package com.platform.core;

import com.platform.core.controller.VersionController;
import org.junit.jupiter.api.Test;
import org.springframework.http.ResponseEntity;

import java.util.Map;

import static org.junit.jupiter.api.Assertions.assertEquals;
import static org.junit.jupiter.api.Assertions.assertNotNull;

class PlatformCoreApplicationTests {

    @Test
    void testVersionEndpointUnit() {
        VersionController controller = new VersionController();
        ResponseEntity<Map<String, Object>> response = controller.getVersion();
        assertNotNull(response);
        assertEquals(200, response.getStatusCode().value());

        Map<String, Object> body = response.getBody();
        assertNotNull(body);
        assertEquals("Platform", body.get("product"));
        assertNotNull(body.get("version"));
    }
}
