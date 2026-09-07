package com.platform.core.api.dto;

import com.fasterxml.jackson.annotation.JsonIgnore;
import com.fasterxml.jackson.annotation.JsonProperty;

import java.math.BigDecimal;
import java.time.Instant;
import java.util.List;
import java.util.UUID;

public final class EntityDtos {
    private EntityDtos() {}

    public record CreateEntityRequest(
            String entityType,
            String displayName,
            String description,
            String discoveredName,
            String lifecycleState,
            UUID siteId,
            String criticality,
            UUID ownerTeamId,
            String devicePackId,
            String devicePackVersion,
            String createdBy,
            List<CreateIdentifierRequest> identifiers
    ) {}

    public static final class UpdateEntityRequest {
        private String entityType;
        private String displayName;
        private String description;
        private String discoveredName;
        private String lifecycleState;
        private UUID siteId;
        private boolean siteIdPresent;
        private String criticality;
        private UUID ownerTeamId;
        private boolean ownerTeamIdPresent;
        private String devicePackId;
        private String devicePackVersion;
        private Instant lastSeenAt;

        public String getEntityType() {
            return entityType;
        }

        public void setEntityType(String entityType) {
            this.entityType = entityType;
        }

        public String getDisplayName() {
            return displayName;
        }

        public void setDisplayName(String displayName) {
            this.displayName = displayName;
        }

        public String getDescription() {
            return description;
        }

        public void setDescription(String description) {
            this.description = description;
        }

        public String getDiscoveredName() {
            return discoveredName;
        }

        public void setDiscoveredName(String discoveredName) {
            this.discoveredName = discoveredName;
        }

        public String getLifecycleState() {
            return lifecycleState;
        }

        public void setLifecycleState(String lifecycleState) {
            this.lifecycleState = lifecycleState;
        }

        @JsonProperty("siteId")
        public void setSiteId(UUID siteId) {
            this.siteId = siteId;
            this.siteIdPresent = true;
        }

        public UUID getSiteId() {
            return siteId;
        }

        @JsonIgnore
        public boolean isSiteIdPresent() {
            return siteIdPresent;
        }

        public String getCriticality() {
            return criticality;
        }

        public void setCriticality(String criticality) {
            this.criticality = criticality;
        }

        @JsonProperty("ownerTeamId")
        public void setOwnerTeamId(UUID ownerTeamId) {
            this.ownerTeamId = ownerTeamId;
            this.ownerTeamIdPresent = true;
        }

        public UUID getOwnerTeamId() {
            return ownerTeamId;
        }

        @JsonIgnore
        public boolean isOwnerTeamIdPresent() {
            return ownerTeamIdPresent;
        }

        public String getDevicePackId() {
            return devicePackId;
        }

        public void setDevicePackId(String devicePackId) {
            this.devicePackId = devicePackId;
        }

        public String getDevicePackVersion() {
            return devicePackVersion;
        }

        public void setDevicePackVersion(String devicePackVersion) {
            this.devicePackVersion = devicePackVersion;
        }

        public Instant getLastSeenAt() {
            return lastSeenAt;
        }

        public void setLastSeenAt(Instant lastSeenAt) {
            this.lastSeenAt = lastSeenAt;
        }
    }

    public record EntityResponse(
            UUID entityId,
            UUID tenantId,
            String entityType,
            String displayName,
            String description,
            String discoveredName,
            String lifecycleState,
            UUID siteId,
            String criticality,
            UUID ownerTeamId,
            String devicePackId,
            String devicePackVersion,
            Instant firstSeenAt,
            Instant lastSeenAt,
            String createdBy,
            Instant createdAt,
            Instant updatedAt
    ) {}

    public record CreateIdentifierRequest(
            String identifierType,
            String namespace,
            String identifierValue,
            BigDecimal confidence,
            Instant firstSeenAt,
            Instant lastSeenAt
    ) {}

    public record IdentifierResponse(
            UUID identifierId,
            UUID tenantId,
            UUID entityId,
            String identifierType,
            String namespace,
            String identifierValue,
            BigDecimal confidence,
            Instant firstSeenAt,
            Instant lastSeenAt
    ) {}

    public record AssignGroupRequest(UUID groupId) {}

    public record AssignTagRequest(UUID tagId) {}

    public record GroupMembershipResponse(
            UUID tenantId,
            UUID entityId,
            UUID groupId,
            Instant createdAt
    ) {}

    public record TagAssignmentResponse(
            UUID tenantId,
            UUID entityId,
            UUID tagId,
            Instant createdAt
    ) {}
}
