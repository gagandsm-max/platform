package com.platform.core.sensorcontrol.pollgrant;

import java.time.Instant;
import java.util.UUID;

/**
 * Immutable V015 row snapshot. Identifiers and plan references in this row do not
 * authenticate a caller or establish execution/capture-time authority.
 * Nullable terminal fields retain their database meaning, including absent actor/reason.
 */
public record SensorPollGrant(
        UUID grantId, UUID tenantId, String sensorId, UUID entityId, UUID endpointId,
        long grantGeneration, long assignmentGeneration, Protocol protocol,
        AddressType issuedAddressType, String issuedAddress,
        String profilePlanId, String profilePlanVersion,
        Instant issuedAt, Instant windowStart, Instant windowEnd, String issuedBy,
        Status status, Instant revokedAt, String revokedBy,
        RevocationClass revocationClass, String revocationReason, Instant updatedAt) {

    public enum Protocol { ICMP, SNMP }
    public enum AddressType { IPV4, IPV6, FQDN }
    public enum Status { ISSUED, REVOKED, SUPERSEDED }
    public enum RevocationClass { SECURITY, ROUTINE, SUPERSEDED }
}
