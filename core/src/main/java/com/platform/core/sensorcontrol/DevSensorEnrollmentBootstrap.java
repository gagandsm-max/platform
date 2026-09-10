package com.platform.core.sensorcontrol;

import org.slf4j.Logger;
import org.slf4j.LoggerFactory;
import org.springframework.beans.factory.annotation.Value;
import org.springframework.boot.ApplicationArguments;
import org.springframework.boot.ApplicationRunner;
import org.springframework.boot.autoconfigure.condition.ConditionalOnProperty;
import org.springframework.core.annotation.Order;
import org.springframework.stereotype.Component;

import java.io.InputStream;
import java.nio.file.Files;
import java.nio.file.Path;
import java.security.cert.CertificateFactory;
import java.security.cert.X509Certificate;
import java.util.List;
import java.util.UUID;

/**
 * Development-only bootstrap that enrolls docker-compose Sensor identities and binds
 * mounted client certificates so local mTLS transport can exercise the production
 * fingerprint authority path. Never enabled in production profiles.
 *
 * <p>Not a public enrollment API — internal ApplicationRunner gated by property.
 */
@Component
@Order(100)
@ConditionalOnProperty(name = "platform.sensor-control.dev-bootstrap-enrollment", havingValue = "true")
public class DevSensorEnrollmentBootstrap implements ApplicationRunner {

    private static final Logger log = LoggerFactory.getLogger(DevSensorEnrollmentBootstrap.class);
    private static final UUID DEFAULT_TENANT = UUID.fromString("018f0e0a-0000-7000-8000-000000000001");

    private final SensorControlService sensorControlService;
    private final SensorControlRepository repository;
    private final Path certDir;

    public DevSensorEnrollmentBootstrap(
            SensorControlService sensorControlService,
            SensorControlRepository repository,
            @Value("${platform.sensor-control.dev-cert-dir:/certs}") String certDir
    ) {
        this.sensorControlService = sensorControlService;
        this.repository = repository;
        this.certDir = Path.of(certDir);
    }

    @Override
    public void run(ApplicationArguments args) {
        List<DevSensor> sensors = List.of(
                new DevSensor("sensor-net-01", SensorRole.NETWORK, "sensor-net.crt"),
                new DevSensor("sensor-infra-01", SensorRole.INFRASTRUCTURE, "sensor-infra.crt")
        );
        for (DevSensor sensor : sensors) {
            try {
                ensureEnrolledAndBound(sensor);
            } catch (Exception e) {
                log.warn("Dev Sensor enrollment bootstrap skipped for {}: {}",
                        sensor.sensorId(), e.getMessage());
            }
        }
    }

    private void ensureEnrolledAndBound(DevSensor sensor) throws Exception {
        Path certPath = certDir.resolve(sensor.certFile());
        if (!Files.isRegularFile(certPath)) {
            log.warn("Dev Sensor enrollment bootstrap: cert missing at {}", certPath);
            return;
        }
        X509Certificate certificate = loadCertificate(certPath);
        String fingerprint = CertificateFingerprints.sha256DerHex(certificate);

        if (repository.findPrincipalById(sensor.sensorId()).isEmpty()) {
            sensorControlService.enrollSensor(
                    sensor.sensorId(), DEFAULT_TENANT, null, sensor.role(), false);
            log.info("Dev Sensor enrollment bootstrap: enrolled sensorId={} role={}",
                    sensor.sensorId(), sensor.role());
        }

        var existing = repository.findCredentialByFingerprint(fingerprint);
        if (existing.isPresent()) {
            if (!existing.get().sensorId().equals(sensor.sensorId())) {
                throw new SensorControlException(
                        SensorControlException.ERR_SENSOR_CREDENTIAL_DUPLICATE,
                        "Dev bootstrap fingerprint already bound to another Sensor");
            }
            return;
        }

        SensorPrincipal principal = repository.findPrincipalById(sensor.sensorId()).orElseThrow();
        var active = repository.listActiveCredentialsForSensor(principal.tenantId(), principal.sensorId());
        if (active.isEmpty()) {
            sensorControlService.bindCertificate(sensor.sensorId(), certificate);
            log.info("Dev Sensor enrollment bootstrap: bound certificate sensorId={} fingerprint={}",
                    sensor.sensorId(), fingerprint);
        } else {
            sensorControlService.rotateCertificate(sensor.sensorId(), certificate);
            log.info("Dev Sensor enrollment bootstrap: rotated certificate sensorId={} fingerprint={}",
                    sensor.sensorId(), fingerprint);
        }
    }

    private static X509Certificate loadCertificate(Path path) throws Exception {
        CertificateFactory factory = CertificateFactory.getInstance("X.509");
        try (InputStream in = Files.newInputStream(path)) {
            return (X509Certificate) factory.generateCertificate(in);
        }
    }

    private record DevSensor(String sensorId, SensorRole role, String certFile) {}
}
