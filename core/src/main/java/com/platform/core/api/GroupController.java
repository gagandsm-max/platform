package com.platform.core.api;

import com.platform.core.api.dto.GroupDtos.CreateGroupRequest;
import com.platform.core.api.dto.GroupDtos.GroupResponse;
import com.platform.core.api.dto.GroupDtos.UpdateGroupRequest;
import com.platform.core.domain.inventory.EntityGroup;
import com.platform.core.domain.inventory.EntityGroupService;
import org.springframework.http.HttpStatus;
import org.springframework.http.ResponseEntity;
import org.springframework.web.bind.annotation.GetMapping;
import org.springframework.web.bind.annotation.PathVariable;
import org.springframework.web.bind.annotation.PostMapping;
import org.springframework.web.bind.annotation.PutMapping;
import org.springframework.web.bind.annotation.RequestBody;
import org.springframework.web.bind.annotation.RequestMapping;
import org.springframework.web.bind.annotation.RequestParam;
import org.springframework.web.bind.annotation.RestController;

import java.util.UUID;

@RestController
@RequestMapping("/api/v1/tenants/{tenantId}/groups")
public class GroupController {

    private final EntityGroupService groups;

    public GroupController(EntityGroupService groups) {
        this.groups = groups;
    }

    @PostMapping
    public ResponseEntity<GroupResponse> create(
            @PathVariable UUID tenantId,
            @RequestBody CreateGroupRequest request
    ) {
        EntityGroup created = groups.create(
                tenantId,
                request.parentGroupId(),
                request.name(),
                request.description()
        );
        return ResponseEntity.status(HttpStatus.CREATED).body(toResponse(created));
    }

    @GetMapping("/{groupId}")
    public GroupResponse get(@PathVariable UUID tenantId, @PathVariable UUID groupId) {
        return toResponse(groups.get(tenantId, groupId));
    }

    @GetMapping
    public PageResult<GroupResponse> list(
            @PathVariable UUID tenantId,
            @RequestParam(defaultValue = "1") int page,
            @RequestParam(required = false) Integer limit
    ) {
        PageResult<EntityGroup> result = groups.list(tenantId, page, PageResult.normalizeLimit(limit));
        return new PageResult<>(
                result.items().stream().map(GroupController::toResponse).toList(),
                result.page(),
                result.limit(),
                result.total(),
                result.hasMore()
        );
    }

    @PutMapping("/{groupId}")
    public GroupResponse update(
            @PathVariable UUID tenantId,
            @PathVariable UUID groupId,
            @RequestBody UpdateGroupRequest request
    ) {
        return toResponse(groups.update(
                tenantId,
                groupId,
                request.getParentGroupId(),
                request.isParentGroupIdPresent(),
                request.getName(),
                request.getDescription()
        ));
    }

    private static GroupResponse toResponse(EntityGroup group) {
        return new GroupResponse(
                group.groupId(),
                group.tenantId(),
                group.parentGroupId(),
                group.name(),
                group.description(),
                group.createdAt(),
                group.updatedAt()
        );
    }
}
