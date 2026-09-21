package com.platform.core.api;

import com.platform.core.api.dto.EntityManagementEndpointDtos.CreateManagementEndpointRequest;
import com.platform.core.api.dto.EntityManagementEndpointDtos.ManagementEndpointResponse;
import com.platform.core.api.dto.EntityManagementEndpointDtos.UpdateManagementEndpointRequest;
import com.platform.core.domain.inventory.EntityManagementEndpoint;
import com.platform.core.domain.inventory.EntityManagementEndpointService;
import com.platform.core.domain.inventory.ManagementEndpointAddressType;
import com.platform.core.domain.inventory.ManagementEndpointSource;
import com.platform.core.domain.inventory.ManagementEndpointState;
import org.springframework.http.HttpStatus;
import org.springframework.http.ResponseEntity;
import org.springframework.web.bind.annotation.GetMapping;
import org.springframework.web.bind.annotation.PathVariable;
import org.springframework.web.bind.annotation.PostMapping;
import org.springframework.web.bind.annotation.PutMapping;
import org.springframework.web.bind.annotation.RequestBody;
import org.springframework.web.bind.annotation.RequestMapping;
import org.springframework.web.bind.annotation.RestController;

import java.util.List;
import java.util.UUID;

@RestController
@RequestMapping("/api/v1/tenants/{tenantId}/entities/{entityId}/management-endpoints")
public class EntityManagementEndpointController {

    private final EntityManagementEndpointService endpoints;

    public EntityManagementEndpointController(EntityManagementEndpointService endpoints) {
        this.endpoints = endpoints;
    }

    @GetMapping
    public List<ManagementEndpointResponse> list(
            @PathVariable UUID tenantId,
            @PathVariable UUID entityId
    ) {
        return endpoints.list(tenantId, entityId).stream().map(EntityManagementEndpointController::toResponse).toList();
    }

    @GetMapping("/primary")
    public ResponseEntity<ManagementEndpointResponse> primary(
            @PathVariable UUID tenantId,
            @PathVariable UUID entityId
    ) {
        return endpoints.resolvePrimaryManagementEndpoint(tenantId, entityId)
                .map(ep -> ResponseEntity.ok(toResponse(ep)))
                .orElseGet(() -> ResponseEntity.notFound().build());
    }

    @PostMapping
    public ResponseEntity<ManagementEndpointResponse> create(
            @PathVariable UUID tenantId,
            @PathVariable UUID entityId,
            @RequestBody CreateManagementEndpointRequest request
    ) {
        if (request == null) {
            throw new com.platform.core.domain.DomainException("VALIDATION_FAILED", "request body is required");
        }
        ManagementEndpointAddressType type = ManagementEndpointAddressType.from(request.addressType());
        boolean primary = request.primary() != null && request.primary();
        ManagementEndpointSource source = ManagementEndpointSource.from(request.source());
        EntityManagementEndpoint created;
        if (source == ManagementEndpointSource.MANUAL) {
            created = endpoints.createManual(tenantId, entityId, type, request.address(), primary);
        } else {
            // Explicit approved-source path only — still requires entity.update authorization.
            created = endpoints.createFromApprovedSource(
                    tenantId, entityId, type, request.address(), source, primary);
        }
        return ResponseEntity.status(HttpStatus.CREATED).body(toResponse(created));
    }

    @PutMapping("/{endpointId}")
    public ManagementEndpointResponse update(
            @PathVariable UUID tenantId,
            @PathVariable UUID entityId,
            @PathVariable UUID endpointId,
            @RequestBody UpdateManagementEndpointRequest request
    ) {
        if (request == null) {
            throw new com.platform.core.domain.DomainException("VALIDATION_FAILED", "request body is required");
        }
        ManagementEndpointState state = request.state() == null ? null : ManagementEndpointState.from(request.state());
        return toResponse(endpoints.updateState(tenantId, entityId, endpointId, state, request.primary()));
    }

    private static ManagementEndpointResponse toResponse(EntityManagementEndpoint ep) {
        return new ManagementEndpointResponse(
                ep.endpointId(),
                ep.tenantId(),
                ep.entityId(),
                ep.addressType().name(),
                ep.address(),
                ep.state().name(),
                ep.source().name(),
                ep.primary(),
                ep.createdBy(),
                ep.createdAt(),
                ep.updatedBy(),
                ep.updatedAt()
        );
    }
}
