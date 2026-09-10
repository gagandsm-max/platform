package com.platform.core.sensorcontrol;

import org.junit.jupiter.api.Test;

import java.nio.charset.StandardCharsets;
import java.security.MessageDigest;
import java.security.cert.X509Certificate;
import java.util.HexFormat;

import static org.junit.jupiter.api.Assertions.assertEquals;
import static org.junit.jupiter.api.Assertions.assertThrows;
import static org.mockito.Mockito.mock;
import static org.mockito.Mockito.when;

class CertificateFingerprintsTest {

    @Test
    void sha256DerProducesLowercaseHex64() throws Exception {
        byte[] der = "platform-sensor-cert-der".getBytes(StandardCharsets.UTF_8);
        X509Certificate cert = mock(X509Certificate.class);
        when(cert.getEncoded()).thenReturn(der);

        String expected = HexFormat.of().formatHex(MessageDigest.getInstance("SHA-256").digest(der));
        assertEquals(64, expected.length());
        assertEquals(expected, CertificateFingerprints.sha256DerHex(cert));
        assertEquals(expected, expected.toLowerCase());
    }

    @Test
    void nullCertificateRejected() {
        assertThrows(NullPointerException.class, () -> CertificateFingerprints.sha256DerHex(null));
    }
}
