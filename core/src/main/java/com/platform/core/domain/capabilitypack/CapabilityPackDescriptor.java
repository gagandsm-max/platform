package com.platform.core.domain.capabilitypack;

import java.util.List;
import java.util.Objects;
import java.util.Optional;

/**
 * Immutable normalized Capability Pack descriptor.
 *
 * <p>Device Pack {@code schemaVersion} {@code 1.0.0} maps only to
 * {@link CapabilityPackClass#DEVICE_NETWORK}. This foundation does not declare a
 * Platform version range; see {@link #platformVersionRangePresent()}.
 */
public final class CapabilityPackDescriptor {

    private final String packId;
    private final String packVersion;
    private final CapabilityPackClass packClass;
    private final String schemaVersion;
    private final String name;
    private final PackPublisher publisher;
    private final String certification;
    private final PackMetadata metadata;
    private final List<MatchRule> matchRules;
    private final List<PackCapability> capabilities;
    private final List<PackProtocol> protocols;
    private final List<PackReference> references;
    private final List<NormalizationMapping> normalization;
    private final List<PackDependency> dependencies;
    private final List<PackProvenance> provenance;
    private final String manifestSha256;

    CapabilityPackDescriptor(
            String packId,
            String packVersion,
            CapabilityPackClass packClass,
            String schemaVersion,
            String name,
            PackPublisher publisher,
            String certification,
            PackMetadata metadata,
            List<MatchRule> matchRules,
            List<PackCapability> capabilities,
            List<PackProtocol> protocols,
            List<PackReference> references,
            List<NormalizationMapping> normalization,
            List<PackDependency> dependencies,
            List<PackProvenance> provenance,
            String manifestSha256
    ) {
        this.packId = Objects.requireNonNull(packId, "packId");
        this.packVersion = Objects.requireNonNull(packVersion, "packVersion");
        this.packClass = Objects.requireNonNull(packClass, "packClass");
        this.schemaVersion = Objects.requireNonNull(schemaVersion, "schemaVersion");
        this.name = Objects.requireNonNull(name, "name");
        this.publisher = Objects.requireNonNull(publisher, "publisher");
        this.certification = Objects.requireNonNull(certification, "certification");
        this.metadata = metadata == null ? PackMetadata.empty() : metadata;
        this.matchRules = List.copyOf(Objects.requireNonNull(matchRules, "matchRules"));
        this.capabilities = List.copyOf(Objects.requireNonNull(capabilities, "capabilities"));
        this.protocols = List.copyOf(Objects.requireNonNull(protocols, "protocols"));
        this.references = List.copyOf(Objects.requireNonNull(references, "references"));
        this.normalization = List.copyOf(Objects.requireNonNull(normalization, "normalization"));
        this.dependencies = List.copyOf(Objects.requireNonNull(dependencies, "dependencies"));
        this.provenance = List.copyOf(Objects.requireNonNull(provenance, "provenance"));
        this.manifestSha256 = Objects.requireNonNull(manifestSha256, "manifestSha256");
    }

    public String packId() {
        return packId;
    }

    public String packVersion() {
        return packVersion;
    }

    public CapabilityPackClass packClass() {
        return packClass;
    }

    public String schemaVersion() {
        return schemaVersion;
    }

    public String name() {
        return name;
    }

    public PackPublisher publisher() {
        return publisher;
    }

    public String certification() {
        return certification;
    }

    public PackMetadata metadata() {
        return metadata;
    }

    public List<MatchRule> matchRules() {
        return matchRules;
    }

    public List<PackCapability> capabilities() {
        return capabilities;
    }

    public List<PackProtocol> protocols() {
        return protocols;
    }

    public List<PackReference> references() {
        return references;
    }

    public List<NormalizationMapping> normalization() {
        return normalization;
    }

    public List<PackDependency> dependencies() {
        return dependencies;
    }

    public List<PackProvenance> provenance() {
        return provenance;
    }

    public String manifestSha256() {
        return manifestSha256;
    }

    /**
     * Device Pack schemaVersion 1.0.0 does not declare a Platform version range.
     * EMC1.1 deliberately records absence rather than inventing a range field.
     */
    public boolean platformVersionRangePresent() {
        return false;
    }

    /** Always empty for schemaVersion 1.0.0 Device Pack manifests. */
    public Optional<String> platformVersionRange() {
        return Optional.empty();
    }

    public String identityKey() {
        return packId + "@" + packVersion;
    }
}
