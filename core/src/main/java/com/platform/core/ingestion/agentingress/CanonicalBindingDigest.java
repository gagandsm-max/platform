package com.platform.core.ingestion.agentingress;

import java.io.ByteArrayOutputStream;
import java.io.DataOutputStream;
import java.nio.charset.StandardCharsets;
import java.security.MessageDigest;
import java.util.HexFormat;
import java.util.List;
import java.util.UUID;

/** Versioned, length-prefixed UTF-8 checkpoint serialization. */
final class CanonicalBindingDigest {
    private CanonicalBindingDigest() {}

    static String combine(UUID tenantId, String sensorId, UUID reservationId,
            String observationSha256, List<PendingCanonicalBindingV1> bindings) {
        if (tenantId == null || sensorId == null || sensorId.isBlank() || reservationId == null
                || observationSha256 == null || !observationSha256.matches("[0-9a-f]{64}")
                || bindings == null || bindings.isEmpty()) {
            throw new IllegalArgumentException("Incomplete canonical checkpoint scope");
        }
        try {
            ByteArrayOutputStream bytes = new ByteArrayOutputStream();
            DataOutputStream out = new DataOutputStream(bytes);
            field(out, "PendingCanonicalBindingV1+observations:1");
            field(out, observationSha256);
            field(out, tenantId.toString());
            field(out, sensorId);
            field(out, reservationId.toString());
            out.writeInt(bindings.size());
            for (PendingCanonicalBindingV1 binding : bindings) {
                if (binding == null || binding.agentId() == null || binding.agentId().isBlank()
                        || binding.hostId() == null || binding.hostId().isBlank()
                        || binding.entityIdentifierId() == null || binding.entityId() == null
                        || binding.sensorIngressSequence() < 1 || binding.assignmentGeneration() < 1
                        || binding.bindingVersion() < 1) {
                    throw new IllegalArgumentException("Incomplete canonical binding");
                }
                field(out, tenantId.toString());
                field(out, sensorId);
                field(out, reservationId.toString());
                out.writeLong(binding.sensorIngressSequence());
                field(out, binding.agentId());
                out.writeLong(binding.assignmentGeneration());
                field(out, binding.hostId());
                field(out, binding.entityIdentifierId().toString());
                field(out, binding.entityId().toString());
                out.writeLong(binding.bindingVersion());
            }
            out.flush();
            return HexFormat.of().formatHex(MessageDigest.getInstance("SHA-256").digest(bytes.toByteArray()));
        } catch (java.io.IOException | java.security.NoSuchAlgorithmException e) {
            throw new IllegalStateException("Cannot digest canonical checkpoint", e);
        }
    }

    private static void field(DataOutputStream out, String value) throws java.io.IOException {
        byte[] encoded = value.getBytes(StandardCharsets.UTF_8);
        out.writeInt(encoded.length);
        out.write(encoded);
    }
}
