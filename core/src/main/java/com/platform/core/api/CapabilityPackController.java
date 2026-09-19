package com.platform.core.api;

import com.platform.core.api.dto.CapabilityPackDtos.ActivateRequest;
import com.platform.core.api.dto.CapabilityPackDtos.ActiveVersionResponse;
import com.platform.core.api.dto.CapabilityPackDtos.ActiveVersionSummary;
import com.platform.core.api.dto.CapabilityPackDtos.AssignRequest;
import com.platform.core.api.dto.CapabilityPackDtos.PackVersionListItem;
import com.platform.core.api.dto.CapabilityPackDtos.PackVersionResponse;
import com.platform.core.api.dto.CapabilityPackDtos.SensorAssignmentItem;
import com.platform.core.api.dto.CapabilityPackDtos.SensorAssignmentsResponse;
import com.platform.core.domain.DomainException;
import com.platform.core.domain.capabilitypack.CapabilityPackActivation;
import com.platform.core.domain.capabilitypack.CapabilityPackCatalogService;
import com.platform.core.domain.capabilitypack.CapabilityPackLoader;
import com.platform.core.domain.capabilitypack.CapabilityPackRegistrationResult;
import com.platform.core.domain.capabilitypack.SensorCapabilityPackAssignment;
import com.platform.core.domain.capabilitypack.SensorCapabilityPackAssignmentService;
import com.platform.core.domain.capabilitypack.StoredCapabilityPack;
import org.springframework.http.HttpStatus;
import org.springframework.http.MediaType;
import org.springframework.http.ResponseEntity;
import org.springframework.web.bind.annotation.DeleteMapping;
import org.springframework.web.bind.annotation.GetMapping;
import org.springframework.web.bind.annotation.PathVariable;
import org.springframework.web.bind.annotation.PutMapping;
import org.springframework.web.bind.annotation.RequestBody;
import org.springframework.web.bind.annotation.RequestMapping;
import org.springframework.web.bind.annotation.RequestParam;
import org.springframework.web.bind.annotation.RestController;

import java.util.Optional;
import java.util.UUID;

@RestController
@RequestMapping("/api/v1/tenants/{tenantId}/capability-packs")
public class CapabilityPackController {

    private final CapabilityPackCatalogService catalog;
    private final SensorCapabilityPackAssignmentService assignmentService;

    public CapabilityPackController(
            CapabilityPackCatalogService catalog,
            SensorCapabilityPackAssignmentService assignmentService
    ) {
        this.catalog = catalog;
        this.assignmentService = assignmentService;
    }

    @PutMapping(
            path = "/{packId}/versions/{packVersion}",
            consumes = MediaType.APPLICATION_JSON_VALUE
    )
    public ResponseEntity<PackVersionResponse> register(
            @PathVariable UUID tenantId,
            @PathVariable String packId,
            @PathVariable String packVersion,
            @RequestBody(required = false) byte[] rawBody
    ) {
        if (rawBody != null && rawBody.length > CapabilityPackLoader.MAX_MANIFEST_BYTES) {
            throw new DomainException(
                    "MANIFEST_TOO_LARGE",
                    "Manifest exceeds max size " + CapabilityPackLoader.MAX_MANIFEST_BYTES + " bytes."
            );
        }
        CapabilityPackRegistrationResult result = catalog.register(tenantId, packId, packVersion, rawBody);
        PackVersionResponse body = toVersionResponse(result.pack(),
                catalog.findActive(tenantId, result.pack().packId()).orElse(null));
        return ResponseEntity.status(result.created() ? HttpStatus.CREATED : HttpStatus.OK).body(body);
    }

    @GetMapping
    public PageResult<PackVersionListItem> list(
            @PathVariable UUID tenantId,
            @RequestParam(defaultValue = "1") int page,
            @RequestParam(required = false) Integer limit
    ) {
        PageResult<StoredCapabilityPack> result = catalog.list(tenantId, page, PageResult.normalizeLimit(limit));
        return new PageResult<>(
                result.items().stream().map(CapabilityPackController::toListItem).toList(),
                result.page(),
                result.limit(),
                result.total(),
                result.hasMore()
        );
    }

    @GetMapping("/{packId}/versions/{packVersion}")
    public PackVersionResponse get(
            @PathVariable UUID tenantId,
            @PathVariable String packId,
            @PathVariable String packVersion
    ) {
        StoredCapabilityPack pack = catalog.get(tenantId, packId, packVersion);
        Optional<CapabilityPackActivation> active = catalog.findActive(tenantId, packId);
        CapabilityPackActivation matching = active
                .filter(a -> a.packVersion().equals(packVersion))
                .orElse(null);
        return toVersionResponse(pack, matching);
    }

    @GetMapping("/{packId}/active-version")
    public ActiveVersionResponse getActive(
            @PathVariable UUID tenantId,
            @PathVariable String packId
    ) {
        CapabilityPackActivation active = catalog.requireActive(tenantId, packId);
        return new ActiveVersionResponse(
                active.packId(),
                active.packVersion(),
                active.generation(),
                active.activatedAt(),
                active.activatedBy()
        );
    }

    @PutMapping(
            path = "/{packId}/active-version",
            consumes = MediaType.APPLICATION_JSON_VALUE
    )
    public ActiveVersionResponse activate(
            @PathVariable UUID tenantId,
            @PathVariable String packId,
            @RequestBody ActivateRequest request
    ) {
        if (request == null || request.packVersion() == null || request.packVersion().isBlank()) {
            throw new DomainException("VALIDATION_FAILED", "packVersion is required");
        }
        if (request.expectedGeneration() == null) {
            throw new DomainException("VALIDATION_FAILED", "expectedGeneration is required");
        }
        CapabilityPackActivation active = catalog.activate(
                tenantId, packId, request.packVersion(), request.expectedGeneration());
        return new ActiveVersionResponse(
                active.packId(),
                active.packVersion(),
                active.generation(),
                active.activatedAt(),
                active.activatedBy()
        );
    }

    @DeleteMapping("/{packId}/active-version")
    public ResponseEntity<Void> deactivate(
            @PathVariable UUID tenantId,
            @PathVariable String packId,
            @RequestParam long expectedGeneration
    ) {
        catalog.deactivate(tenantId, packId, expectedGeneration);
        return ResponseEntity.noContent().build();
    }

    @GetMapping("/assignments/sensors/{sensorId}")
    public SensorAssignmentsResponse getSensorAssignments(
            @PathVariable UUID tenantId,
            @PathVariable String sensorId
    ) {
        return toAssignmentsResponse(sensorId, assignmentService.getAssignments(tenantId, sensorId));
    }

    @PutMapping(
            path = "/assignments/sensors/{sensorId}/{packId}",
            consumes = MediaType.APPLICATION_JSON_VALUE
    )
    public SensorAssignmentsResponse assignToSensor(
            @PathVariable UUID tenantId,
            @PathVariable String sensorId,
            @PathVariable String packId,
            @RequestBody AssignRequest request
    ) {
        if (request == null || request.packVersion() == null || request.packVersion().isBlank()) {
            throw new DomainException("VALIDATION_FAILED", "packVersion is required");
        }
        if (request.expectedGeneration() == null) {
            throw new DomainException("VALIDATION_FAILED", "expectedGeneration is required");
        }
        return toAssignmentsResponse(
                sensorId,
                assignmentService.assign(
                        tenantId, sensorId, packId, request.packVersion(), request.expectedGeneration()));
    }

    @DeleteMapping("/assignments/sensors/{sensorId}/{packId}")
    public SensorAssignmentsResponse unassignFromSensor(
            @PathVariable UUID tenantId,
            @PathVariable String sensorId,
            @PathVariable String packId,
            @RequestParam(required = false) Long expectedGeneration
    ) {
        if (expectedGeneration == null) {
            throw new DomainException("VALIDATION_FAILED", "expectedGeneration is required");
        }
        return toAssignmentsResponse(
                sensorId,
                assignmentService.unassign(tenantId, sensorId, packId, expectedGeneration));
    }

    private static SensorAssignmentsResponse toAssignmentsResponse(
            String sensorId,
            SensorCapabilityPackAssignmentService.AssignmentView view
    ) {
        return new SensorAssignmentsResponse(
                sensorId,
                view.desiredGeneration(),
                view.desiredChecksum(),
                view.appliedGeneration(),
                view.appliedChecksum(),
                view.appliedAt(),
                view.assignments().stream().map(CapabilityPackController::toAssignmentItem).toList()
        );
    }

    private static SensorAssignmentItem toAssignmentItem(SensorCapabilityPackAssignment a) {
        return new SensorAssignmentItem(a.packId(), a.packVersion(), a.assignedBy(), a.assignedAt());
    }

    private static PackVersionListItem toListItem(StoredCapabilityPack pack) {
        return new PackVersionListItem(
                pack.packId(),
                pack.packVersion(),
                pack.schemaVersion(),
                pack.packClass(),
                pack.name(),
                pack.publisherType(),
                pack.publisherName(),
                pack.certification(),
                pack.manifestSha256(),
                pack.createdBy(),
                pack.createdAt()
        );
    }

    private static PackVersionResponse toVersionResponse(
            StoredCapabilityPack pack,
            CapabilityPackActivation activeOrNull
    ) {
        ActiveVersionSummary summary = null;
        if (activeOrNull != null) {
            summary = new ActiveVersionSummary(
                    activeOrNull.packVersion(),
                    activeOrNull.generation(),
                    activeOrNull.activatedAt(),
                    activeOrNull.activatedBy()
            );
        }
        return new PackVersionResponse(
                pack.packId(),
                pack.packVersion(),
                pack.schemaVersion(),
                pack.packClass(),
                pack.name(),
                pack.publisherType(),
                pack.publisherName(),
                pack.certification(),
                pack.manifestSha256(),
                pack.createdBy(),
                pack.createdAt(),
                summary
        );
    }
}
