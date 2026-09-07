package com.platform.core.api.dto;

import com.fasterxml.jackson.annotation.JsonIgnore;
import com.fasterxml.jackson.annotation.JsonProperty;

import java.time.Instant;
import java.util.UUID;

public final class GroupDtos {
    private GroupDtos() {}

    public record CreateGroupRequest(
            UUID parentGroupId,
            String name,
            String description
    ) {}

    public static final class UpdateGroupRequest {
        private UUID parentGroupId;
        private boolean parentGroupIdPresent;
        private String name;
        private String description;

        @JsonProperty("parentGroupId")
        public void setParentGroupId(UUID parentGroupId) {
            this.parentGroupId = parentGroupId;
            this.parentGroupIdPresent = true;
        }

        public UUID getParentGroupId() {
            return parentGroupId;
        }

        @JsonIgnore
        public boolean isParentGroupIdPresent() {
            return parentGroupIdPresent;
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
    }

    public record GroupResponse(
            UUID groupId,
            UUID tenantId,
            UUID parentGroupId,
            String name,
            String description,
            Instant createdAt,
            Instant updatedAt
    ) {}
}
