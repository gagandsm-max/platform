package com.platform.core.sensorcontrol;

import com.platform.core.domain.DomainException;
import com.platform.core.domain.discovery.DiscoveryRequest;
import com.platform.core.domain.discovery.DiscoveryService;
import com.platform.core.domain.discovery.DiscoveryStatus;
import com.platform.core.security.audit.AuditActions;
import com.platform.core.security.audit.AuditCategory;
import com.platform.core.security.audit.AuditOutcome;
import com.platform.core.security.http.SecurityAuditEmitter;
import org.slf4j.Logger;
import org.slf4j.LoggerFactory;
import org.springframework.stereotype.Service;
import org.springframework.transaction.annotation.Transactional;

import java.util.Comparator;
import java.util.List;
import java.util.Map;
import java.util.Objects;
import java.util.UUID;

/**
 * Deterministic discovery placement from enrolled Sensor Control principals.
 * Does not use heartbeat/liveness. Site policy: when the request has a site_id,
 * the sensor.site_id must equal that site; when the request site is null, any sensor site is eligible.
 */
@Service
public class DiscoveryPlacementService {

    private static final Logger log = LoggerFactory.getLogger(DiscoveryPlacementService.class);

    private final SensorControlRepository sensorRepository;
    private final DiscoveryService discoveryService;
    private final SecurityAuditEmitter auditEmitter;
    private final DiscoveryPlacementMetrics metrics;

    public DiscoveryPlacementService(
            SensorControlRepository sensorRepository,
            DiscoveryService discoveryService,
            SecurityAuditEmitter auditEmitter,
            DiscoveryPlacementMetrics metrics
    ) {
        this.sensorRepository = Objects.requireNonNull(sensorRepository, "sensorRepository");
        this.discoveryService = Objects.requireNonNull(discoveryService, "discoveryService");
        this.auditEmitter = Objects.requireNonNull(auditEmitter, "auditEmitter");
        this.metrics = Objects.requireNonNull(metrics, "metrics");
    }

    /**
     * Place a PENDING discovery request onto the lexicographically first eligible ACTIVE sensor.
     *
     * @return the assigned discovery request
     * @throws SensorControlException if the request is not placeable or no eligible sensor exists (request left PENDING)
     */
    @Transactional("transactionManager")
    public DiscoveryRequest placePendingRequest(UUID tenantId, UUID requestId) {
        Objects.requireNonNull(tenantId, "tenantId");
        Objects.requireNonNull(requestId, "requestId");
        long startedNs = System.nanoTime();

        try {
            DiscoveryRequest request = discoveryService.getRequestForUpdate(tenantId, requestId);
            if (request.status() != DiscoveryStatus.PENDING || request.assignedSensorId() != null) {
                auditEmitter.emitControlEvent(
                        AuditCategory.SECURITY_ADMINISTRATION,
                        AuditActions.DISCOVERY_PLACEMENT_REJECT,
                        AuditOutcome.FAILURE,
                        tenantId,
                        "DISCOVERY_NOT_PLACEABLE",
                        Map.of("request_id", requestId.toString())
                );
                metrics.recordFailure(DiscoveryPlacementMetrics.REASON_NOT_PLACEABLE, elapsedMs(startedNs));
                throw new SensorControlException(
                        SensorControlException.ERR_DISCOVERY_REQUEST_NOT_PLACEABLE,
                        "Discovery request must be PENDING with no assigned sensor"
                );
            }

            List<SensorPrincipal> eligible = sensorRepository.listActivePrincipalsByTenant(tenantId).stream()
                    .filter(s -> SensorCapability.roleAllows(s.role(), SensorCapability.DISCOVERY))
                    .filter(s -> siteCompatible(request.siteId(), s.siteId()))
                    .sorted(Comparator.comparing(SensorPrincipal::sensorId))
                    .toList();

            if (eligible.isEmpty()) {
                log.info("Discovery placement rejected: no eligible sensor for requestId={} tenantId={}",
                        requestId, tenantId);
                auditEmitter.emitControlEvent(
                        AuditCategory.SECURITY_ADMINISTRATION,
                        AuditActions.DISCOVERY_PLACEMENT_REJECT,
                        AuditOutcome.FAILURE,
                        tenantId,
                        "NO_ELIGIBLE_SENSOR",
                        Map.of("request_id", requestId.toString())
                );
                metrics.recordFailure(DiscoveryPlacementMetrics.REASON_NO_ELIGIBLE_SENSOR, elapsedMs(startedNs));
                throw new SensorControlException(
                        SensorControlException.ERR_NO_ELIGIBLE_SENSOR,
                        "No ACTIVE DISCOVERY-capable sensor eligible for placement"
                );
            }

            SensorPrincipal chosen = eligible.getFirst();
            DiscoveryRequest assigned = discoveryService.assignSensorFirstPlacementFromControlPlane(
                    tenantId, requestId, chosen.sensorId());
            auditEmitter.emitControlEvent(
                    AuditCategory.SECURITY_ADMINISTRATION,
                    AuditActions.DISCOVERY_PLACEMENT_ASSIGN,
                    AuditOutcome.SUCCESS,
                    tenantId,
                    "DISCOVERY_SENSOR_ASSIGNED",
                    Map.of(
                            "request_id", requestId.toString(),
                            "sensor_id", chosen.sensorId()
                    )
            );
            metrics.recordSuccess(DiscoveryPlacementMetrics.REASON_ASSIGNED, elapsedMs(startedNs));
            return assigned;
        } catch (SensorControlException e) {
            throw e;
        } catch (DomainException e) {
            if ("DISCOVERY_PLACEMENT_CONFLICT".equals(e.getCode())) {
                auditEmitter.emitControlEvent(
                        AuditCategory.SECURITY_ADMINISTRATION,
                        AuditActions.DISCOVERY_PLACEMENT_REJECT,
                        AuditOutcome.FAILURE,
                        tenantId,
                        "DISCOVERY_PLACEMENT_CONFLICT",
                        Map.of("request_id", requestId.toString())
                );
                metrics.recordFailure(DiscoveryPlacementMetrics.REASON_PLACEMENT_CONFLICT, elapsedMs(startedNs));
                throw new SensorControlException(
                        SensorControlException.ERR_DISCOVERY_PLACEMENT_CONFLICT,
                        "Discovery request placement conflict or stale concurrent attempt"
                );
            }
            metrics.recordFailure(DiscoveryPlacementMetrics.REASON_PLACEMENT_ERROR, elapsedMs(startedNs));
            throw e;
        } catch (RuntimeException e) {
            metrics.recordFailure(DiscoveryPlacementMetrics.REASON_PLACEMENT_ERROR, elapsedMs(startedNs));
            throw e;
        }
    }

    private static long elapsedMs(long startedNs) {
        return Math.max(0L, (System.nanoTime() - startedNs) / 1_000_000L);
    }

    /**
     * Site compatibility:
     * <ul>
     *   <li>If request.siteId is null → any sensor site (including null) is eligible.</li>
     *   <li>If request.siteId is non-null → sensor.siteId must equal request.siteId
     *       (tenant-wide sensors with null site are not eligible for site-scoped requests).</li>
     * </ul>
     */
    static boolean siteCompatible(UUID requestSiteId, UUID sensorSiteId) {
        if (requestSiteId == null) {
            return true;
        }
        return requestSiteId.equals(sensorSiteId);
    }
}
