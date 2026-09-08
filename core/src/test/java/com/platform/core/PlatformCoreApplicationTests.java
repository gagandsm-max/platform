package com.platform.core;

import com.platform.core.controller.SensorIngestionController;
import com.platform.core.controller.VersionController;
import com.platform.core.domain.discovery.DiscoveryService;
import org.junit.jupiter.api.Test;
import org.springframework.http.HttpStatus;
import org.springframework.http.ResponseEntity;
import org.springframework.mock.web.MockHttpServletRequest;

import javax.security.auth.x500.X500Principal;
import java.math.BigInteger;
import java.security.cert.X509Certificate;
import java.util.Map;

import static org.junit.jupiter.api.Assertions.assertEquals;
import static org.junit.jupiter.api.Assertions.assertNotNull;
import static org.mockito.Mockito.mock;
import static org.mockito.Mockito.when;

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

    @Test
    void testSensorIngestionRequiresClientCertificate() {
        SensorIngestionController controller = new SensorIngestionController(mock(DiscoveryService.class));
        MockHttpServletRequest request = new MockHttpServletRequest();

        // No client certificate attached to request
        ResponseEntity<Map<String, Object>> response = controller.sensorHeartbeat(Map.of(), request);
        assertNotNull(response);
        assertEquals(HttpStatus.UNAUTHORIZED, response.getStatusCode());
        assertEquals("ERR_AUTH_CLIENT_CERT_REQUIRED", response.getBody().get("code"));
    }

    @Test
    void testSensorIngestionAcceptsValidClientCertificate() {
        SensorIngestionController controller = new SensorIngestionController(mock(DiscoveryService.class));
        MockHttpServletRequest request = new MockHttpServletRequest();

        X509Certificate mockCert = mock(X509Certificate.class);
        when(mockCert.getSubjectX500Principal()).thenReturn(new X500Principal("CN=sensor-net-01.sensor.platform.local,OU=Sensors,O=Platform"));
        when(mockCert.getSerialNumber()).thenReturn(BigInteger.valueOf(12345));

        request.setAttribute("jakarta.servlet.request.X509Certificate", new X509Certificate[]{mockCert});

        ResponseEntity<Map<String, Object>> response = controller.sensorHeartbeat(Map.of(), request);
        assertNotNull(response);
        assertEquals(HttpStatus.OK, response.getStatusCode());
        assertEquals("sensor-net-01.sensor.platform.local", response.getBody().get("sensor_identity"));
        assertEquals(true, response.getBody().get("authenticated"));
    }
}
