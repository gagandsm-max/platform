package com.platform.core.api.dto;

import com.fasterxml.jackson.annotation.JsonIgnore;
import com.fasterxml.jackson.annotation.JsonProperty;

import java.time.Instant;
import java.util.UUID;

public final class SiteDtos {
    private SiteDtos() {}

    public record CreateSiteRequest(
            UUID parentSiteId,
            String name,
            String description,
            String timezone
    ) {}

    public static final class UpdateSiteRequest {
        private UUID parentSiteId;
        private boolean parentSiteIdPresent;
        private String name;
        private String description;
        private String timezone;

        @JsonProperty("parentSiteId")
        public void setParentSiteId(UUID parentSiteId) {
            this.parentSiteId = parentSiteId;
            this.parentSiteIdPresent = true;
        }

        public UUID getParentSiteId() {
            return parentSiteId;
        }

        @JsonIgnore
        public boolean isParentSiteIdPresent() {
            return parentSiteIdPresent;
        }

        public String getName() {
            return name;
        }

        public void setName(String name) {
            this.name = name;
        }

        public String getDescription() {
            return description;
        }

        public void setDescription(String description) {
            this.description = description;
        }

        public String getTimezone() {
            return timezone;
        }

        public void setTimezone(String timezone) {
            this.timezone = timezone;
        }
    }

    public record SiteResponse(
            UUID siteId,
            UUID tenantId,
            UUID parentSiteId,
            String name,
            String description,
            String timezone,
            Instant createdAt,
            Instant updatedAt
    ) {}
}
