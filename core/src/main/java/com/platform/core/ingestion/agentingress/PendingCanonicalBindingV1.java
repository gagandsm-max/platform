package com.platform.core.ingestion.agentingress;

import java.util.UUID;

/** Exact per-ingress-record authority persisted under one pending reservation. */
public record PendingCanonicalBindingV1(
        long sensorIngressSequence,
        String agentId,
        long assignmentGeneration,
        String hostId,
        UUID entityIdentifierId,
        UUID entityId,
        long bindingVersion
) {}
