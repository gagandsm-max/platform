package com.platform.core.api;

import com.platform.core.api.dto.RecurringScheduleDtos.AssignmentItem;
import com.platform.core.api.dto.RecurringScheduleDtos.AssignmentRequestItem;
import com.platform.core.api.dto.RecurringScheduleDtos.AssignmentsResponse;
import com.platform.core.api.dto.RecurringScheduleDtos.ReplaceAssignmentsRequest;
import com.platform.core.api.dto.RecurringScheduleDtos.SnmpLimitsDto;
import com.platform.core.api.dto.RecurringScheduleDtos.SnmpParamsDto;
import com.platform.core.domain.DomainException;
import com.platform.core.domain.recurring.RecurringAssignment;
import com.platform.core.domain.recurring.RecurringScheduleBounds;
import com.platform.core.domain.recurring.SensorRecurringAssignmentService;
import com.platform.core.domain.recurring.SensorRecurringAssignmentService.AssignmentInput;
import com.platform.core.domain.recurring.SnmpParams;
import org.springframework.http.MediaType;
import org.springframework.web.bind.annotation.GetMapping;
import org.springframework.web.bind.annotation.PathVariable;
import org.springframework.web.bind.annotation.PutMapping;
import org.springframework.web.bind.annotation.RequestBody;
import org.springframework.web.bind.annotation.RequestMapping;
import org.springframework.web.bind.annotation.RestController;

import java.util.List;
import java.util.UUID;

@RestController
@RequestMapping("/api/v1/tenants/{tenantId}/sensors/{sensorId}/recurring-assignments")
public class RecurringScheduleController {

    private final SensorRecurringAssignmentService assignmentService;

    public RecurringScheduleController(SensorRecurringAssignmentService assignmentService) {
        this.assignmentService = assignmentService;
    }

    @GetMapping
    public AssignmentsResponse list(
            @PathVariable UUID tenantId,
            @PathVariable String sensorId
    ) {
        return toResponse(sensorId, assignmentService.getAssignments(tenantId, sensorId));
    }

    @PutMapping(consumes = MediaType.APPLICATION_JSON_VALUE)
    public AssignmentsResponse replace(
            @PathVariable UUID tenantId,
            @PathVariable String sensorId,
            @RequestBody ReplaceAssignmentsRequest request
    ) {
        if (request == null) {
            throw new DomainException("VALIDATION_FAILED", "request body is required");
        }
        if (request.expectedGeneration() == null) {
            throw new DomainException("VALIDATION_FAILED", "expectedGeneration is required");
        }
        if (request.assignments() == null) {
            throw new DomainException("VALIDATION_FAILED", "assignments is required");
        }
        List<AssignmentInput> inputs = request.assignments().stream()
                .map(RecurringScheduleController::toInput)
                .toList();
        return toResponse(
                sensorId,
                assignmentService.replaceAssignments(
                        tenantId, sensorId, request.expectedGeneration(), inputs));
    }

    private static AssignmentInput toInput(AssignmentRequestItem item) {
        if (item == null) {
            throw new DomainException("VALIDATION_FAILED", "assignment entry is required");
        }
        if (item.cadenceMs() == null || item.freshnessMs() == null
                || item.phaseVersion() == null || item.executionTimeoutMs() == null) {
            throw new DomainException(
                    "VALIDATION_FAILED",
                    "cadenceMs, freshnessMs, phaseVersion and executionTimeoutMs are required");
        }
        SnmpParams snmp = item.snmp() == null ? null : toSnmp(item.snmp());
        return new AssignmentInput(
                item.assignmentId(),
                item.entityId(),
                item.monitorDefinitionId(),
                item.schedulingClass(),
                item.executionClass(),
                item.cadenceMs(),
                item.freshnessMs(),
                item.phaseVersion(),
                item.executionTimeoutMs(),
                item.credentialProfileRef(),
                item.protocol(),
                snmp,
                item.protocolParamsJson()
        );
    }

    private static SnmpParams toSnmp(SnmpParamsDto dto) {
        if (dto.port() == null || dto.timeoutMs() == null || dto.limits() == null) {
            throw new DomainException("VALIDATION_FAILED", "snmp.port, timeoutMs and limits are required");
        }
        SnmpLimitsDto limits = dto.limits();
        if (limits.maxOids() == null || limits.maxWalkEntries() == null
                || limits.maxBulkRepetitions() == null || limits.maxResults() == null
                || limits.maxResultBytes() == null) {
            throw new DomainException("VALIDATION_FAILED", "snmp.limits fields are required");
        }
        return new SnmpParams(
                dto.target(),
                dto.port(),
                dto.version(),
                dto.operation(),
                dto.oids() == null ? List.of() : dto.oids(),
                dto.timeoutMs(),
                new SnmpParams.SnmpLimits(
                        limits.maxOids(),
                        limits.maxWalkEntries(),
                        limits.maxBulkRepetitions(),
                        limits.maxResults(),
                        limits.maxResultBytes()
                )
        );
    }

    private static AssignmentsResponse toResponse(
            String sensorId,
            SensorRecurringAssignmentService.AssignmentView view
    ) {
        return new AssignmentsResponse(
                sensorId,
                view.desiredGeneration(),
                view.desiredChecksum(),
                view.appliedGeneration(),
                view.appliedChecksum(),
                view.appliedAt(),
                view.assignments().stream().map(RecurringScheduleController::toItem).toList()
        );
    }

    private static AssignmentItem toItem(RecurringAssignment row) {
        SnmpParams snmp = RecurringScheduleBounds.parseAndValidateSnmpParams(row.protocolParamsJson());
        return new AssignmentItem(
                row.assignmentId(),
                row.entityId(),
                row.monitorDefinitionId(),
                row.schedulingClass(),
                row.executionClass(),
                row.cadenceMs(),
                row.freshnessMs(),
                row.phaseVersion(),
                row.executionTimeoutMs(),
                row.credentialProfileRef(),
                row.protocol(),
                toSnmpDto(snmp),
                row.assignedBy(),
                row.assignedAt()
        );
    }

    private static SnmpParamsDto toSnmpDto(SnmpParams snmp) {
        return new SnmpParamsDto(
                snmp.target(),
                snmp.port(),
                snmp.version(),
                snmp.operation(),
                snmp.oids(),
                snmp.timeoutMs(),
                new SnmpLimitsDto(
                        snmp.limits().maxOids(),
                        snmp.limits().maxWalkEntries(),
                        snmp.limits().maxBulkRepetitions(),
                        snmp.limits().maxResults(),
                        snmp.limits().maxResultBytes()
                )
        );
    }
}
