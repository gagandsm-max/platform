package com.platform.core;

import com.platform.core.controller.SensorIngestionController;
import com.platform.core.controller.VersionController;
import com.platform.core.domain.discovery.DiscoveryService;
import com.platform.core.ingestion.IngestionException;
import com.platform.core.ingestion.SensorIngestionService;
import com.platform.core.ingestion.authority.SensorAuthorityResolver;
import com.platform.core.ingestion.authority.SensorScope;
import org.junit.jupiter.api.Test;
import org.springframework.http.HttpStatus;
import org.springframework.http.ResponseEntity;
import org.springframework.mock.web.MockHttpServletRequest;

import javax.security.auth.x500.X500Principal;
import java.math.BigInteger;
import java.security.cert.X509Certificate;
import java.util.Map;
import java.util.Set;
import java.util.UUID;

import static org.junit.jupiter.api.Assertions.assertEquals;
import static org.junit.jupiter.api.Assertions.assertNotNull;
import static org.mockito.ArgumentMatchers.any;
import static org.mockito.Mockito.mock;
import static org.mockito.Mockito.when;

class PlatformCoreApplicationTests {

    private static final UUID TENANT = UUID.fromString("018f0e0a-0000-7000-8000-000000000001");

    private static SensorIngestionController newController(SensorAuthorityResolver resolver) {
        return new SensorIngestionController(
                mock(SensorIngestionService.class),
                new com.fasterxml.jackson.databind.ObjectMapper(),
                mock(DiscoveryService.class),
                resolver
        );
    }

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
        SensorIngestionController controller = newController(mock(SensorAuthorityResolver.class));
        MockHttpServletRequest request = new MockHttpServletRequest();

        ResponseEntity<Map<String, Object>> response = controller.sensorHeartbeat(Map.of(), request);
        assertNotNull(response);
        assertEquals(HttpStatus.UNAUTHORIZED, response.getStatusCode());
        assertEquals("ERR_AUTH_CLIENT_CERT_REQUIRED", response.getBody().get("code"));
    }

    @Test
    void testSensorHeartbeatUsesAuthoritativeSensorIdentityNotCn() {
        SensorAuthorityResolver resolver = mock(SensorAuthorityResolver.class);
        when(resolver.resolve(any())).thenReturn(new SensorScope(
                "sensor-net-01", TENANT, null, Set.of(), true));
        SensorIngestionController controller = newController(resolver);
        MockHttpServletRequest request = new MockHttpServletRequest();

        X509Certificate mockCert = mock(X509Certificate.class);
        when(mockCert.getSubjectX500Principal()).thenReturn(new X500Principal("CN=misleading-cn.example,OU=Sensors,O=Platform"));
        when(mockCert.getSerialNumber()).thenReturn(BigInteger.valueOf(12345));

        request.setAttribute("jakarta.servlet.request.X509Certificate", new X509Certificate[]{mockCert});

        ResponseEntity<Map<String, Object>> response = controller.sensorHeartbeat(Map.of(), request);
        assertNotNull(response);
        assertEquals(HttpStatus.OK, response.getStatusCode());
        assertEquals("sensor-net-01", response.getBody().get("sensor_identity"));
        assertEquals(true, response.getBody().get("authenticated"));
    }

    @Test
    void testSensorHeartbeatFailsClosedWhenAuthorityUnavailable() {
        SensorAuthorityResolver resolver = mock(SensorAuthorityResolver.class);
        when(resolver.resolve(any())).thenThrow(new IngestionException(
                HttpStatus.FORBIDDEN, "ERR_SENSOR_AUTHORITY_UNAVAILABLE", "unavailable"));
        SensorIngestionController controller = newController(resolver);
        MockHttpServletRequest request = new MockHttpServletRequest();

        X509Certificate mockCert = mock(X509Certificate.class);
        when(mockCert.getSubjectX500Principal()).thenReturn(new X500Principal("CN=sensor-net-01,O=Platform"));
        request.setAttribute("jakarta.servlet.request.X509Certificate", new X509Certificate[]{mockCert});

        ResponseEntity<Map<String, Object>> response = controller.sensorHeartbeat(Map.of(), request);
        assertEquals(HttpStatus.FORBIDDEN, response.getStatusCode());
        assertEquals("ERR_SENSOR_AUTHORITY_UNAVAILABLE", response.getBody().get("code"));
    }
}
