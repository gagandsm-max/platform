package com.platform.core.controller;

import com.platform.core.api.dto.RecurringScheduleDtos.AppliedStateRequest;
import com.platform.core.api.dto.RecurringScheduleDtos.DesiredDefinitionItem;
import com.platform.core.api.dto.RecurringScheduleDtos.SensorDesiredStateResponse;
import com.platform.core.api.dto.RecurringScheduleDtos.SnmpLimitsDto;
import com.platform.core.api.dto.RecurringScheduleDtos.SnmpParamsDto;
import com.platform.core.domain.DomainException;
import com.platform.core.domain.recurring.DesiredRecurringDefinition;
import com.platform.core.domain.recurring.SensorRecurringAssignmentService;
import com.platform.core.domain.recurring.SensorRecurringDesiredState;
import com.platform.core.domain.recurring.SnmpParams;
import com.platform.core.ingestion.IngestionException;
import com.platform.core.ingestion.authority.SensorAuthorityResolver;
import com.platform.core.ingestion.authority.SensorScope;
import jakarta.servlet.http.HttpServletRequest;
import org.slf4j.Logger;
import org.slf4j.LoggerFactory;
import org.springframework.http.HttpStatus;
import org.springframework.http.MediaType;
import org.springframework.http.ResponseEntity;
import org.springframework.web.bind.annotation.GetMapping;
import org.springframework.web.bind.annotation.PutMapping;
import org.springframework.web.bind.annotation.RequestBody;
import org.springframework.web.bind.annotation.RequestMapping;
import org.springframework.web.bind.annotation.RestController;

import java.security.cert.X509Certificate;
import java.util.LinkedHashMap;
import java.util.Map;
import java.util.Objects;

/**
 * Sensor machine recurring schedule pull/ACK API (Slice A).
 * Identity is exclusively from mTLS → {@link SensorAuthorityResolver}; client-claimed ids ignored.
 */
@RestController
@RequestMapping("/api/v1/sensor/recurring-schedules")
public class SensorRecurringScheduleController {

    private static final Logger log = LoggerFactory.getLogger(SensorRecurringScheduleController.class);

    private final SensorAuthorityResolver authorityResolver;
    private final SensorRecurringAssignmentService assignmentService;

    public SensorRecurringScheduleController(
            SensorAuthorityResolver authorityResolver,
            SensorRecurringAssignmentService assignmentService
    ) {
        this.authorityResolver = Objects.requireNonNull(authorityResolver, "authorityResolver");
        this.assignmentService = Objects.requireNonNull(assignmentService, "assignmentService");
    }

    @GetMapping("/desired-state")
    public ResponseEntity<?> desiredState(HttpServletRequest request) {
        try {
            SensorScope scope = resolveScope(request);
            if (scope == null) {
                return clientCertRequired();
            }
            SensorRecurringDesiredState desired =
                    assignmentService.getDesiredState(scope.tenantId(), scope.sensorId());
            return ResponseEntity.ok(new SensorDesiredStateResponse(
                    desired.generation(),
                    desired.checksum(),
                    desired.appliedGeneration(),
                    desired.appliedChecksum(),
                    desired.definitions().stream().map(SensorRecurringScheduleController::toItem).toList()
            ));
        } catch (IngestionException e) {
            return ingestionError(e);
        }
    }

    @PutMapping(path = "/applied-state", consumes = MediaType.APPLICATION_JSON_VALUE)
    public ResponseEntity<?> appliedState(
            @RequestBody AppliedStateRequest body,
            HttpServletRequest request
    ) {
        try {
            SensorScope scope = resolveScope(request);
            if (scope == null) {
                return clientCertRequired();
            }
            if (body == null || body.generation() == null) {
                throw new DomainException("VALIDATION_FAILED", "generation is required");
            }
            try {
                assignmentService.ackAppliedState(
                        scope.tenantId(), scope.sensorId(), body.generation(), body.checksum());
                Map<String, Object> resp = new LinkedHashMap<>();
                resp.put("status", "ACCEPTED");
                resp.put("generation", body.generation());
                resp.put("checksum", body.checksum());
                return ResponseEntity.ok(resp);
            } catch (DomainException ex) {
                if (SensorRecurringAssignmentService.ERR_STATE_STALE.equals(ex.getCode())
                        || SensorRecurringAssignmentService.ERR_CHECKSUM_MISMATCH.equals(ex.getCode())) {
                    return ResponseEntity.status(HttpStatus.CONFLICT).body(Map.of(
                            "status", "ERROR",
                            "code", ex.getCode(),
                            "message", ex.getMessage()
                    ));
                }
                throw ex;
            }
        } catch (IngestionException e) {
            return ingestionError(e);
        }
    }

    private SensorScope resolveScope(HttpServletRequest request) {
        X509Certificate clientCert = extractClientCertificate(request);
        if (clientCert == null) {
            log.warn("Rejected Sensor recurring-schedule API without client certificate from remote={}",
                    request.getRemoteAddr());
            return null;
        }
        try {
            SensorScope scope = authorityResolver.resolve(clientCert);
            if (scope == null || scope.sensorId() == null || scope.sensorId().isBlank()) {
                throw new IngestionException(
                        HttpStatus.FORBIDDEN,
                        "ERR_SENSOR_AUTHORITY_UNAVAILABLE",
                        "Authoritative Sensor Scope is unavailable"
                );
            }
            return scope;
        } catch (IngestionException e) {
            throw e;
        }
    }

    private static DesiredDefinitionItem toItem(DesiredRecurringDefinition e) {
        return new DesiredDefinitionItem(
                e.assignmentId(),
                e.tenantId(),
                e.entityId(),
                e.monitorDefinitionId(),
                e.schedulingClass(),
                e.executionClass(),
                e.cadenceMs(),
                e.freshnessMs(),
                e.phaseVersion(),
                e.executionTimeoutMs(),
                e.credentialProfileRef(),
                e.protocol(),
                toSnmpDto(e.snmp())
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

    private static X509Certificate extractClientCertificate(HttpServletRequest request) {
        Object certAttr = request.getAttribute("jakarta.servlet.request.X509Certificate");
        if (certAttr instanceof X509Certificate[] certs && certs.length > 0) {
            return certs[0];
        }
        return null;
    }

    private static ResponseEntity<Map<String, Object>> clientCertRequired() {
        return ResponseEntity.status(HttpStatus.UNAUTHORIZED).body(Map.of(
                "status", "ERROR",
                "code", "ERR_AUTH_CLIENT_CERT_REQUIRED",
                "message", "mTLS client certificate authentication required for Sensor recurring schedule transport"
        ));
    }

    private static ResponseEntity<Map<String, Object>> ingestionError(IngestionException e) {
        return ResponseEntity.status(e.getStatus()).body(Map.of(
                "status", "ERROR",
                "code", e.getCode(),
                "message", e.getMessage()
        ));
    }
}
