package com.platform.core.domain.capabilitypack;

import com.platform.core.api.PageResult;
import com.platform.core.domain.DomainException;
import com.platform.core.id.UuidV7;
import com.platform.core.logging.CorrelationIdFilter;
import com.platform.core.persistence.CapabilityPackRepository;
import com.platform.core.security.http.PlatformSecurityContext;
import org.slf4j.MDC;
import org.springframework.stereotype.Service;
import org.springframework.transaction.annotation.Transactional;

import java.time.Instant;
import java.util.ArrayList;
import java.util.List;
import java.util.Objects;
import java.util.Optional;
import java.util.UUID;

/**
 * Tenant-scoped Capability Pack catalog: immutable registration and activation snapshot control.
 *
 * <p>Activation/deactivation always locks {@code platform.tenant} FOR UPDATE, reloads the active
 * snapshot, validates the candidate via {@link CapabilityPackRegistry#validateSnapshot}, then mutates.
 */
@Service
public class CapabilityPackCatalogService {

    public static final String ERR_NOT_FOUND = "CAPABILITY_PACK_NOT_FOUND";
    public static final String ERR_IDENTITY_CONFLICT = "CAPABILITY_PACK_IDENTITY_CONFLICT";
    public static final String ERR_PATH_IDENTITY_MISMATCH = "CAPABILITY_PACK_PATH_IDENTITY_MISMATCH";
    public static final String ERR_GENERATION_CONFLICT = "CAPABILITY_PACK_GENERATION_CONFLICT";
    public static final String ERR_DEPENDENCY_IN_USE = "CAPABILITY_PACK_DEPENDENCY_IN_USE";
    public static final String ERR_SNAPSHOT_INVALID = "CAPABILITY_PACK_SNAPSHOT_INVALID";

    private final CapabilityPackRepository repository;
    private final CapabilityPackLoader loader;

    public CapabilityPackCatalogService(CapabilityPackRepository repository) {
        this(repository, new CapabilityPackLoader());
    }

    CapabilityPackCatalogService(CapabilityPackRepository repository, CapabilityPackLoader loader) {
        this.repository = Objects.requireNonNull(repository, "repository");
        this.loader = Objects.requireNonNull(loader, "loader");
    }

    @Transactional
    public CapabilityPackRegistrationResult register(
            UUID tenantId,
            String packId,
            String packVersion,
            byte[] manifestBytes
    ) {
        requireTenant(tenantId);
        Objects.requireNonNull(packId, "packId");
        Objects.requireNonNull(packVersion, "packVersion");
        if (manifestBytes == null || manifestBytes.length == 0) {
            throw new DomainException("VALIDATION_FAILED", "Manifest body is required");
        }
        if (manifestBytes.length > CapabilityPackLoader.MAX_MANIFEST_BYTES) {
            throw new CapabilityPackValidationException(
                    "MANIFEST_TOO_LARGE",
                    "Manifest exceeds max size " + CapabilityPackLoader.MAX_MANIFEST_BYTES + " bytes."
            );
        }

        CapabilityPackDescriptor descriptor = loader.load(manifestBytes);
        if (!packId.equals(descriptor.packId()) || !packVersion.equals(descriptor.packVersion())) {
            throw new DomainException(
                    ERR_PATH_IDENTITY_MISMATCH,
                    "Path packId/packVersion must exactly match the manifest identity"
            );
        }

        String actor = PlatformSecurityContext.requirePrincipal().subjectId();
        Instant now = Instant.now();
        StoredCapabilityPack pack = new StoredCapabilityPack(
                tenantId,
                descriptor.packId(),
                descriptor.packVersion(),
                descriptor.schemaVersion(),
                descriptor.packClass().name(),
                descriptor.name(),
                descriptor.publisher().type(),
                descriptor.publisher().name(),
                descriptor.certification(),
                descriptor.manifestSha256(),
                manifestBytes,
                actor,
                now
        );
        int inserted = repository.insertPackIfAbsent(pack);
        if (inserted == 1) {
            return new CapabilityPackRegistrationResult(pack, true);
        }
        StoredCapabilityPack existing = repository.findPack(tenantId, packId, packVersion)
                .orElseThrow(() -> new DomainException(
                        "DATABASE_UNAVAILABLE",
                        "Capability pack registration could not be completed"));
        if (existing.manifestSha256().equals(descriptor.manifestSha256())) {
            return new CapabilityPackRegistrationResult(existing, false);
        }
        throw new DomainException(
                ERR_IDENTITY_CONFLICT,
                "A different manifest is already registered for this pack identity"
        );
    }

    public PageResult<StoredCapabilityPack> list(UUID tenantId, int page, int limit) {
        requireTenant(tenantId);
        int p = PageResult.normalizePage(page);
        int l = PageResult.normalizeLimit(limit);
        long total = repository.countPacks(tenantId);
        return new PageResult<>(
                repository.listPacks(tenantId, l, PageResult.offset(p, l)),
                p,
                l,
                total,
                (long) p * l < total
        );
    }

    public StoredCapabilityPack get(UUID tenantId, String packId, String packVersion) {
        requireTenant(tenantId);
        return repository.findPack(tenantId, packId, packVersion)
                .orElseThrow(() -> new DomainException(ERR_NOT_FOUND, "Capability pack version not found"));
    }

    public Optional<CapabilityPackActivation> findActive(UUID tenantId, String packId) {
        requireTenant(tenantId);
        return repository.findActiveActivation(tenantId, packId);
    }

    public CapabilityPackActivation requireActive(UUID tenantId, String packId) {
        return findActive(tenantId, packId)
                .orElseThrow(() -> new DomainException(ERR_NOT_FOUND, "No active capability pack version"));
    }

    @Transactional
    public CapabilityPackActivation activate(UUID tenantId, String packId, String packVersion, long expectedGeneration) {
        lockTenant(tenantId);
        Objects.requireNonNull(packId, "packId");
        Objects.requireNonNull(packVersion, "packVersion");
        if (expectedGeneration < 0) {
            throw new DomainException("VALIDATION_FAILED", "expectedGeneration must be >= 0");
        }

        StoredCapabilityPack target = repository.findPack(tenantId, packId, packVersion)
                .orElseThrow(() -> new DomainException(ERR_NOT_FOUND, "Capability pack version not found"));

        Optional<CapabilityPackActivation> currentActive = repository.findActiveActivation(tenantId, packId);
        long currentGeneration = currentActive.map(CapabilityPackActivation::generation).orElse(0L);
        if (expectedGeneration != currentGeneration) {
            throw new DomainException(ERR_GENERATION_CONFLICT, "Capability pack activation generation conflict");
        }
        if (currentActive.isPresent()
                && currentActive.get().packVersion().equals(packVersion)
                && currentActive.get().generation() == expectedGeneration) {
            return currentActive.get();
        }

        List<CapabilityPackDescriptor> candidate = buildCandidateSnapshot(tenantId, packId, target);
        validateCandidateSnapshot(candidate, false);

        String actor = PlatformSecurityContext.requirePrincipal().subjectId();
        Instant now = Instant.now();
        if (currentActive.isPresent()) {
            int revoked = repository.revokeActivation(
                    tenantId, currentActive.get().activationId(), actor, now);
            if (revoked != 1) {
                throw new DomainException(ERR_GENERATION_CONFLICT, "Capability pack activation generation conflict");
            }
        }

        long nextGeneration = repository.maxActivationGeneration(tenantId, packId) + 1L;
        CapabilityPackActivation created = new CapabilityPackActivation(
                UuidV7.generate(),
                tenantId,
                packId,
                packVersion,
                nextGeneration,
                CapabilityPackActivation.STATUS_ACTIVE,
                actor,
                now,
                null,
                null,
                correlationIdOrNull()
        );
        repository.insertActivation(created);
        return created;
    }

    @Transactional
    public CapabilityPackActivation deactivate(UUID tenantId, String packId, long expectedGeneration) {
        lockTenant(tenantId);
        Objects.requireNonNull(packId, "packId");
        if (expectedGeneration < 1) {
            throw new DomainException("VALIDATION_FAILED", "expectedGeneration must be >= 1");
        }

        CapabilityPackActivation current = repository.findActiveActivation(tenantId, packId)
                .orElseThrow(() -> new DomainException(ERR_NOT_FOUND, "No active capability pack version"));
        if (current.generation() != expectedGeneration) {
            throw new DomainException(ERR_GENERATION_CONFLICT, "Capability pack activation generation conflict");
        }

        List<CapabilityPackDescriptor> candidate = buildCandidateSnapshot(tenantId, packId, null);
        validateCandidateSnapshot(candidate, true);

        String actor = PlatformSecurityContext.requirePrincipal().subjectId();
        Instant now = Instant.now();
        int revoked = repository.revokeActivation(tenantId, current.activationId(), actor, now);
        if (revoked != 1) {
            throw new DomainException(ERR_GENERATION_CONFLICT, "Capability pack activation generation conflict");
        }
        return new CapabilityPackActivation(
                current.activationId(),
                current.tenantId(),
                current.packId(),
                current.packVersion(),
                current.generation(),
                CapabilityPackActivation.STATUS_REVOKED,
                current.activatedBy(),
                current.activatedAt(),
                actor,
                now,
                current.correlationId()
        );
    }

    private List<CapabilityPackDescriptor> buildCandidateSnapshot(
            UUID tenantId,
            String targetPackId,
            StoredCapabilityPack replacementOrNull
    ) {
        List<CapabilityPackActivation> actives = repository.listActiveActivations(tenantId);
        List<CapabilityPackDescriptor> descriptors = new ArrayList<>();
        for (CapabilityPackActivation active : actives) {
            if (active.packId().equals(targetPackId)) {
                continue;
            }
            StoredCapabilityPack stored = repository.findPack(tenantId, active.packId(), active.packVersion())
                    .orElseThrow(() -> new DomainException(
                            ERR_SNAPSHOT_INVALID,
                            "Active pack artifact is missing from registry"));
            descriptors.add(loader.load(stored.manifestBytes()));
        }
        if (replacementOrNull != null) {
            descriptors.add(loader.load(replacementOrNull.manifestBytes()));
        }
        return descriptors;
    }

    private static void validateCandidateSnapshot(List<CapabilityPackDescriptor> candidate, boolean deactivating) {
        try {
            CapabilityPackRegistry.validateSnapshot(candidate);
        } catch (CapabilityPackValidationException ex) {
            if (deactivating && isDependencyFailure(ex)) {
                throw new DomainException(ERR_DEPENDENCY_IN_USE, "Active packs depend on this pack");
            }
            throw new DomainException(ERR_SNAPSHOT_INVALID, sanitizeSnapshotMessage(ex));
        }
    }

    private static boolean isDependencyFailure(CapabilityPackValidationException ex) {
        return ex.getIssues().stream().anyMatch(issue ->
                "DEPENDENCY_NOT_FOUND".equals(issue.code())
                        || "DEPENDENCY_VERSION_MISMATCH".equals(issue.code()));
    }

    private static String sanitizeSnapshotMessage(CapabilityPackValidationException ex) {
        String code = ex.getCode() == null ? ERR_SNAPSHOT_INVALID : ex.getCode();
        return "Capability pack active snapshot is invalid (" + code + ")";
    }

    private void requireTenant(UUID tenantId) {
        Objects.requireNonNull(tenantId, "tenantId");
        if (!repository.tenantExists(tenantId)) {
            throw new DomainException("TENANT_NOT_FOUND", "Tenant not found");
        }
    }

    private void lockTenant(UUID tenantId) {
        Objects.requireNonNull(tenantId, "tenantId");
        if (!repository.lockTenantForPackMutation(tenantId)) {
            throw new DomainException("TENANT_NOT_FOUND", "Tenant not found");
        }
    }

    private static String correlationIdOrNull() {
        String value = MDC.get(CorrelationIdFilter.MDC_CORRELATION_ID_KEY);
        if (value == null || value.isBlank()) {
            return null;
        }
        return value.length() > 128 ? value.substring(0, 128) : value;
    }
}
