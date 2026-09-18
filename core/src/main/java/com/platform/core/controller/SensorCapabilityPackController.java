package com.platform.core.controller;

import com.platform.core.api.dto.CapabilityPackDtos.AppliedStateRequest;
import com.platform.core.api.dto.CapabilityPackDtos.DesiredPackItem;
import com.platform.core.api.dto.CapabilityPackDtos.SensorDesiredStateResponse;
import com.platform.core.domain.DomainException;
import com.platform.core.domain.capabilitypack.DesiredPackEntry;
import com.platform.core.domain.capabilitypack.SensorCapabilityPackAssignmentService;
import com.platform.core.domain.capabilitypack.SensorPackDesiredState;
import com.platform.core.domain.capabilitypack.StoredCapabilityPack;
import com.platform.core.ingestion.IngestionException;
import com.platform.core.ingestion.authority.SensorAuthorityResolver;
import com.platform.core.ingestion.authority.SensorScope;
import jakarta.servlet.http.HttpServletRequest;
import org.slf4j.Logger;
import org.slf4j.LoggerFactory;
import org.springframework.http.HttpHeaders;
import org.springframework.http.HttpStatus;
import org.springframework.http.MediaType;
import org.springframework.http.ResponseEntity;
import org.springframework.web.bind.annotation.GetMapping;
import org.springframework.web.bind.annotation.PathVariable;
import org.springframework.web.bind.annotation.PutMapping;
import org.springframework.web.bind.annotation.RequestBody;
import org.springframework.web.bind.annotation.RequestMapping;
import org.springframework.web.bind.annotation.RestController;

import java.security.cert.X509Certificate;
import java.util.LinkedHashMap;
import java.util.Map;
import java.util.Objects;

/**
 * Sensor machine Capability Pack pull/ACK API (EMC1.3).
 * Identity is exclusively from mTLS → {@link SensorAuthorityResolver}; client-claimed ids ignored.
 */
@RestController
@RequestMapping("/api/v1/sensor/capability-packs")
public class SensorCapabilityPackController {

    public static final String HDR_PACK_SHA256 = "X-Platform-Pack-SHA256";
    public static final String HDR_PACK_ID = "X-Platform-Pack-ID";
    public static final String HDR_PACK_VERSION = "X-Platform-Pack-Version";

    private static final Logger log = LoggerFactory.getLogger(SensorCapabilityPackController.class);

    private final SensorAuthorityResolver authorityResolver;
    private final SensorCapabilityPackAssignmentService assignmentService;

    public SensorCapabilityPackController(
            SensorAuthorityResolver authorityResolver,
            SensorCapabilityPackAssignmentService assignmentService
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
            SensorPackDesiredState desired = assignmentService.getDesiredState(scope.tenantId(), scope.sensorId());
            return ResponseEntity.ok(new SensorDesiredStateResponse(
                    desired.generation(),
                    desired.checksum(),
                    desired.packs().stream().map(SensorCapabilityPackController::toItem).toList()
            ));
        } catch (IngestionException e) {
            return ingestionError(e);
        }
    }

    @GetMapping("/artifacts/{packId}/versions/{packVersion}")
    public ResponseEntity<?> artifact(
            @PathVariable String packId,
            @PathVariable String packVersion,
            HttpServletRequest request
    ) {
        try {
            SensorScope scope = resolveScope(request);
            if (scope == null) {
                return clientCertRequired();
            }
            try {
                StoredCapabilityPack pack = assignmentService.requireAssignedArtifact(
                        scope.tenantId(), scope.sensorId(), packId, packVersion);
                HttpHeaders headers = new HttpHeaders();
                headers.set(HDR_PACK_SHA256, pack.manifestSha256());
                headers.set(HDR_PACK_ID, pack.packId());
                headers.set(HDR_PACK_VERSION, pack.packVersion());
                headers.setContentType(MediaType.APPLICATION_JSON);
                return new ResponseEntity<>(pack.manifestBytes(), headers, HttpStatus.OK);
            } catch (DomainException ex) {
                return ResponseEntity.status(HttpStatus.NOT_FOUND).body(Map.of(
                        "status", "ERROR",
                        "code", "ERR_NOT_FOUND",
                        "message", "Pack artifact not available"
                ));
            }
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
                if (SensorCapabilityPackAssignmentService.ERR_PACK_STATE_STALE.equals(ex.getCode())
                        || SensorCapabilityPackAssignmentService.ERR_PACK_CHECKSUM_MISMATCH.equals(ex.getCode())) {
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
            log.warn("Rejected Sensor pack API without client certificate from remote={}",
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

    private static DesiredPackItem toItem(DesiredPackEntry e) {
        return new DesiredPackItem(e.packId(), e.packVersion(), e.manifestSha256(), e.artifactBytes());
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
                "message", "mTLS client certificate authentication required for Sensor pack transport"
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
