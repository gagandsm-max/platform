package com.platform.core.ingestion.agentingress;

import com.platform.core.ingestion.replay.WatermarkState;

import java.util.UUID;

public interface AgentIngressWatermarkStore {

    WatermarkState getWatermarkForUpdate(UUID tenantId, String sensorId);

    void updateWatermark(
            UUID tenantId,
            String sensorId,
            long ackedThrough,
            long firstSeq,
            long lastSeq,
            String checksum,
            String batchId
    );
}
