package com.platform.core.sensorcontrol;

import java.security.MessageDigest;
import java.security.NoSuchAlgorithmException;
import java.security.cert.CertificateEncodingException;
import java.security.cert.X509Certificate;
import java.util.HexFormat;
import java.util.Objects;

/**
 * Certificate fingerprint helpers. Authority uses SHA-256 of the certificate DER encoding only.
 * CN/SAN values are never used for authority.
 */
public final class CertificateFingerprints {

    private static final HexFormat HEX = HexFormat.of();

    private CertificateFingerprints() {
    }

    /**
     * SHA-256 of the X.509 certificate DER encoding, lowercase hex (64 chars).
     */
    public static String sha256DerHex(X509Certificate certificate) {
        Objects.requireNonNull(certificate, "certificate");
        try {
            return sha256Hex(certificate.getEncoded());
        } catch (CertificateEncodingException e) {
            throw new IllegalArgumentException("Unable to encode client certificate DER", e);
        }
    }

    /**
     * SHA-256 of raw DER bytes, lowercase hex (64 chars).
     */
    public static String sha256Hex(byte[] derBytes) {
        Objects.requireNonNull(derBytes, "derBytes");
        try {
            MessageDigest digest = MessageDigest.getInstance("SHA-256");
            return HEX.formatHex(digest.digest(derBytes));
        } catch (NoSuchAlgorithmException e) {
            throw new IllegalStateException("SHA-256 not available", e);
        }
    }

    /**
     * SHA-256 of the subject public key info (SPKI) DER, lowercase hex, or null if unavailable.
     */
    public static String spkiSha256Hex(X509Certificate certificate) {
        Objects.requireNonNull(certificate, "certificate");
        try {
            return sha256Hex(certificate.getPublicKey().getEncoded());
        } catch (RuntimeException e) {
            return null;
        }
    }
}
