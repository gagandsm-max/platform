package com.platform.core.domain.capabilitypack;

import java.nio.ByteBuffer;
import java.nio.charset.StandardCharsets;
import java.security.MessageDigest;
import java.security.NoSuchAlgorithmException;
import java.util.ArrayList;
import java.util.Comparator;
import java.util.HexFormat;
import java.util.List;
import java.util.Objects;

/**
 * Deterministic desired-state checksum over Sensor Capability Pack assignments.
 *
 * <p>Assignments are sorted by {@code pack_id} ascending. Each assignment contributes
 * length-prefixed UTF-8 fields {@code packId}, {@code packVersion}, {@code manifestSha256}
 * (4-byte big-endian length + bytes). No timestamps, actors, UUIDs, or JSON formatting.
 */
public final class SensorPackDesiredChecksum {

    private SensorPackDesiredChecksum() {
    }

    public record AssignmentDigest(String packId, String packVersion, String manifestSha256) {
        public AssignmentDigest {
            Objects.requireNonNull(packId, "packId");
            Objects.requireNonNull(packVersion, "packVersion");
            Objects.requireNonNull(manifestSha256, "manifestSha256");
        }
    }

    /**
     * @return lowercase 64-hex SHA-256, or {@code null} when the assignment set is empty
     */
    public static String compute(List<AssignmentDigest> assignments) {
        Objects.requireNonNull(assignments, "assignments");
        if (assignments.isEmpty()) {
            return null;
        }
        List<AssignmentDigest> sorted = new ArrayList<>(assignments);
        sorted.sort(Comparator.comparing(AssignmentDigest::packId));
        try {
            MessageDigest digest = MessageDigest.getInstance("SHA-256");
            for (AssignmentDigest row : sorted) {
                writeLengthPrefixed(digest, row.packId());
                writeLengthPrefixed(digest, row.packVersion());
                writeLengthPrefixed(digest, row.manifestSha256());
            }
            return HexFormat.of().formatHex(digest.digest());
        } catch (NoSuchAlgorithmException ex) {
            throw new IllegalStateException("SHA-256 unavailable", ex);
        }
    }

    private static void writeLengthPrefixed(MessageDigest digest, String value) {
        byte[] utf8 = value.getBytes(StandardCharsets.UTF_8);
        if (utf8.length > Integer.MAX_VALUE) {
            throw new IllegalArgumentException("field too large");
        }
        ByteBuffer len = ByteBuffer.allocate(4);
        len.putInt(utf8.length);
        digest.update(len.array());
        digest.update(utf8);
    }
}
