package com.platform.core.api;

import com.platform.core.api.dto.EntityDtos.AssignGroupRequest;
import com.platform.core.api.dto.EntityDtos.AssignTagRequest;
import com.platform.core.api.dto.EntityDtos.CreateEntityRequest;
import com.platform.core.api.dto.EntityDtos.CreateIdentifierRequest;
import com.platform.core.api.dto.EntityDtos.EntityResponse;
import com.platform.core.api.dto.EntityDtos.GroupMembershipResponse;
import com.platform.core.api.dto.EntityDtos.IdentifierResponse;
import com.platform.core.api.dto.EntityDtos.TagAssignmentResponse;
import com.platform.core.api.dto.EntityDtos.UpdateEntityRequest;
import com.platform.core.domain.DomainException;
import com.platform.core.domain.inventory.Entity;
import com.platform.core.domain.inventory.EntityIdentifier;
import com.platform.core.domain.inventory.EntityService;
import com.platform.core.domain.inventory.EntityTagAssignment;
import com.platform.core.domain.inventory.GroupMembership;
import com.platform.core.domain.inventory.LifecycleState;
import org.springframework.http.HttpStatus;
import org.springframework.http.ResponseEntity;
import org.springframework.web.bind.annotation.DeleteMapping;
import org.springframework.web.bind.annotation.GetMapping;
import org.springframework.web.bind.annotation.PathVariable;
import org.springframework.web.bind.annotation.PostMapping;
import org.springframework.web.bind.annotation.PutMapping;
import org.springframework.web.bind.annotation.RequestBody;
import org.springframework.web.bind.annotation.RequestMapping;
import org.springframework.web.bind.annotation.RequestParam;
import org.springframework.web.bind.annotation.RestController;

import java.util.List;
import java.util.UUID;

@RestController
@RequestMapping("/api/v1/tenants/{tenantId}/entities")
public class EntityController {

    private final EntityService entities;

    public EntityController(EntityService entities) {
        this.entities = entities;
    }

    @PostMapping
    public ResponseEntity<EntityResponse> create(
            @PathVariable UUID tenantId,
            @RequestBody CreateEntityRequest request
    ) {
        List<EntityService.IdentifierInput> initial = null;
        if (request.identifiers() != null) {
            initial = request.identifiers().stream().map(EntityController::toInput).toList();
        }
        Entity created = entities.create(
                tenantId,
                request.entityType(),
                request.displayName(),
                request.description(),
                request.discoveredName(),
                request.lifecycleState() == null ? null : LifecycleState.from(request.lifecycleState()),
                request.siteId(),
                request.criticality(),
                request.ownerTeamId(),
                request.devicePackId(),
                request.devicePackVersion(),
                request.createdBy(),
                initial
        );
        return ResponseEntity.status(HttpStatus.CREATED).body(toResponse(created));
    }

    @GetMapping("/{entityId}")
    public EntityResponse get(@PathVariable UUID tenantId, @PathVariable UUID entityId) {
        return toResponse(entities.get(tenantId, entityId));
    }

    @GetMapping
    public PageResult<EntityResponse> list(
            @PathVariable UUID tenantId,
            @RequestParam(defaultValue = "1") int page,
            @RequestParam(required = false) Integer limit
    ) {
        PageResult<Entity> result = entities.list(tenantId, page, PageResult.normalizeLimit(limit));
        return new PageResult<>(
                result.items().stream().map(EntityController::toResponse).toList(),
                result.page(),
                result.limit(),
                result.total(),
                result.hasMore()
        );
    }

    @PutMapping("/{entityId}")
    public EntityResponse update(
            @PathVariable UUID tenantId,
            @PathVariable UUID entityId,
            @RequestBody UpdateEntityRequest request
    ) {
        return toResponse(entities.update(
                tenantId,
                entityId,
                request.getEntityType(),
                request.getDisplayName(),
                request.getDescription(),
                request.getDiscoveredName(),
                request.getLifecycleState() == null ? null : LifecycleState.from(request.getLifecycleState()),
                request.getSiteId(),
                request.isSiteIdPresent(),
                request.getCriticality(),
                request.getOwnerTeamId(),
                request.isOwnerTeamIdPresent(),
                request.getDevicePackId(),
                request.getDevicePackVersion(),
                request.getLastSeenAt()
        ));
    }

    @PostMapping("/{entityId}/identifiers")
    public ResponseEntity<IdentifierResponse> addIdentifier(
            @PathVariable UUID tenantId,
            @PathVariable UUID entityId,
            @RequestBody CreateIdentifierRequest request
    ) {
        EntityIdentifier created = entities.addIdentifier(tenantId, entityId, toInput(request));
        return ResponseEntity.status(HttpStatus.CREATED).body(toIdentifierResponse(created));
    }

    @GetMapping("/{entityId}/identifiers")
    public PageResult<IdentifierResponse> listIdentifiers(
            @PathVariable UUID tenantId,
            @PathVariable UUID entityId,
            @RequestParam(defaultValue = "1") int page,
            @RequestParam(required = false) Integer limit
    ) {
        PageResult<EntityIdentifier> result = entities.listIdentifiers(tenantId, entityId, page, PageResult.normalizeLimit(limit));
        return new PageResult<>(
                result.items().stream().map(EntityController::toIdentifierResponse).toList(),
                result.page(),
                result.limit(),
                result.total(),
                result.hasMore()
        );
    }

    @DeleteMapping("/{entityId}/identifiers/{identifierId}")
    public ResponseEntity<Void> removeIdentifier(
            @PathVariable UUID tenantId,
            @PathVariable UUID entityId,
            @PathVariable UUID identifierId
    ) {
        entities.removeIdentifier(tenantId, entityId, identifierId);
        return ResponseEntity.ok().build();
    }

    @GetMapping("/identifiers/resolve")
    public IdentifierResponse resolveIdentifier(
            @PathVariable UUID tenantId,
            @RequestParam String namespace,
            @RequestParam String identifierType,
            @RequestParam String identifierValue
    ) {
        return toIdentifierResponse(entities.resolveIdentifier(tenantId, namespace, identifierType, identifierValue));
    }

    @PostMapping("/{entityId}/groups")
    public ResponseEntity<GroupMembershipResponse> assignGroup(
            @PathVariable UUID tenantId,
            @PathVariable UUID entityId,
            @RequestBody AssignGroupRequest request
    ) {
        if (request.groupId() == null) {
            throw new DomainException("VALIDATION_FAILED", "groupId is required");
        }
        GroupMembership membership = entities.assignGroup(tenantId, entityId, request.groupId());
        return ResponseEntity.status(HttpStatus.CREATED).body(new GroupMembershipResponse(
                membership.tenantId(), membership.entityId(), membership.groupId(), membership.createdAt()
        ));
    }

    @GetMapping("/{entityId}/groups")
    public PageResult<GroupMembershipResponse> listGroups(
            @PathVariable UUID tenantId,
            @PathVariable UUID entityId,
            @RequestParam(defaultValue = "1") int page,
            @RequestParam(required = false) Integer limit
    ) {
        PageResult<GroupMembership> result = entities.listGroups(tenantId, entityId, page, PageResult.normalizeLimit(limit));
        return new PageResult<>(
                result.items().stream()
                        .map(m -> new GroupMembershipResponse(m.tenantId(), m.entityId(), m.groupId(), m.createdAt()))
                        .toList(),
                result.page(),
                result.limit(),
                result.total(),
                result.hasMore()
        );
    }

    @DeleteMapping("/{entityId}/groups/{groupId}")
    public ResponseEntity<Void> unassignGroup(
            @PathVariable UUID tenantId,
            @PathVariable UUID entityId,
            @PathVariable UUID groupId
    ) {
        entities.unassignGroup(tenantId, entityId, groupId);
        return ResponseEntity.ok().build();
    }

    @PostMapping("/{entityId}/tags")
    public ResponseEntity<TagAssignmentResponse> assignTag(
            @PathVariable UUID tenantId,
            @PathVariable UUID entityId,
            @RequestBody AssignTagRequest request
    ) {
        if (request.tagId() == null) {
            throw new DomainException("VALIDATION_FAILED", "tagId is required");
        }
        EntityTagAssignment assignment = entities.assignTag(tenantId, entityId, request.tagId());
        return ResponseEntity.status(HttpStatus.CREATED).body(new TagAssignmentResponse(
                assignment.tenantId(), assignment.entityId(), assignment.tagId(), assignment.createdAt()
        ));
    }

    @GetMapping("/{entityId}/tags")
    public PageResult<TagAssignmentResponse> listTags(
            @PathVariable UUID tenantId,
            @PathVariable UUID entityId,
            @RequestParam(defaultValue = "1") int page,
            @RequestParam(required = false) Integer limit
    ) {
        PageResult<EntityTagAssignment> result = entities.listTags(tenantId, entityId, page, PageResult.normalizeLimit(limit));
        return new PageResult<>(
                result.items().stream()
                        .map(a -> new TagAssignmentResponse(a.tenantId(), a.entityId(), a.tagId(), a.createdAt()))
                        .toList(),
                result.page(),
                result.limit(),
                result.total(),
                result.hasMore()
        );
    }

    @DeleteMapping("/{entityId}/tags/{tagId}")
    public ResponseEntity<Void> unassignTag(
            @PathVariable UUID tenantId,
            @PathVariable UUID entityId,
            @PathVariable UUID tagId
    ) {
        entities.unassignTag(tenantId, entityId, tagId);
        return ResponseEntity.ok().build();
    }

    private static EntityService.IdentifierInput toInput(CreateIdentifierRequest request) {
        return new EntityService.IdentifierInput(
                request.identifierType(),
                request.namespace(),
                request.identifierValue(),
                request.confidence(),
                request.firstSeenAt(),
                request.lastSeenAt()
        );
    }

    private static EntityResponse toResponse(Entity entity) {
        return new EntityResponse(
                entity.entityId(),
                entity.tenantId(),
                entity.entityType(),
                entity.displayName(),
                entity.description(),
                entity.discoveredName(),
                entity.lifecycleState().name(),
                entity.siteId(),
                entity.criticality(),
                entity.ownerTeamId(),
                entity.devicePackId(),
                entity.devicePackVersion(),
                entity.firstSeenAt(),
                entity.lastSeenAt(),
                entity.createdBy(),
                entity.createdAt(),
                entity.updatedAt()
        );
    }

    private static IdentifierResponse toIdentifierResponse(EntityIdentifier identifier) {
        return new IdentifierResponse(
                identifier.identifierId(),
                identifier.tenantId(),
                identifier.entityId(),
                identifier.identifierType(),
                identifier.namespace(),
                identifier.identifierValue(),
                identifier.confidence(),
                identifier.firstSeenAt(),
                identifier.lastSeenAt()
        );
    }
}
