package com.platform.core.domain.capabilitypack;

import java.util.ArrayList;
import java.util.HashMap;
import java.util.List;
import java.util.Map;
import java.util.Optional;
import java.util.concurrent.atomic.AtomicReference;

/**
 * Concurrent read-safe Capability Pack registry with atomic snapshot replacement.
 *
 * <p>{@link #replaceAll(List)} validates the entire candidate snapshot first. On any failure the
 * previous snapshot is retained (no partial update, no last-wins).
 *
 * <p>An active snapshot admits at most one version per {@code packId}. Dependencies and cycle
 * detection use exact {@code packId@packVersion} identity keys.
 */
public final class CapabilityPackRegistry {

    /**
     * Defensive upper bound on packs in one registry snapshot. Same order of magnitude as
     * {@code MAX_DISCOVERED_MANIFESTS} in {@code device-packs/tools/validate.mjs}.
     */
    public static final int MAX_PACK_COUNT = 256;

    private final AtomicReference<Snapshot> snapshot = new AtomicReference<>(Snapshot.empty());

    public List<CapabilityPackDescriptor> list() {
        return snapshot.get().packs();
    }

    public Optional<CapabilityPackDescriptor> find(String packId, String packVersion) {
        if (packId == null || packVersion == null) {
            return Optional.empty();
        }
        return Optional.ofNullable(snapshot.get().byIdentity().get(packId + "@" + packVersion));
    }

    public int size() {
        return snapshot.get().packs().size();
    }

    public CapabilityPackMatchResult match(CapabilityPackIdentity identity) {
        return CapabilityPackMatcher.match(snapshot.get().packs(), identity);
    }

    /**
     * Atomically replaces the registry contents after validating the full snapshot.
     *
     * @throws CapabilityPackValidationException when the snapshot is rejected; prior state kept
     */
    public void replaceAll(List<CapabilityPackDescriptor> packs) {
        Snapshot next = validateSnapshot(packs);
        snapshot.set(next);
    }

    static Snapshot validateSnapshot(List<CapabilityPackDescriptor> packs) {
        if (packs == null) {
            throw new CapabilityPackValidationException("INVALID_REGISTRY_SNAPSHOT", "Pack list is required.");
        }
        if (packs.size() > MAX_PACK_COUNT) {
            throw new CapabilityPackValidationException(
                    "REGISTRY_TOO_LARGE",
                    "Registry snapshot exceeds max pack count " + MAX_PACK_COUNT + ".",
                    List.of(new CapabilityPackValidationException.Issue(
                            "REGISTRY_TOO_LARGE", "$",
                            "Registry snapshot exceeds max pack count " + MAX_PACK_COUNT + "."))
            );
        }

        List<CapabilityPackValidationException.Issue> issues = new ArrayList<>();
        Map<String, CapabilityPackDescriptor> byIdentity = new HashMap<>();
        Map<String, CapabilityPackDescriptor> byPackId = new HashMap<>();

        for (int i = 0; i < packs.size(); i++) {
            CapabilityPackDescriptor pack = packs.get(i);
            if (pack == null) {
                issues.add(new CapabilityPackValidationException.Issue(
                        "INVALID_REGISTRY_ENTRY", "$[" + i + "]", "Pack descriptor is null."));
                continue;
            }
            if (pack.packClass() != CapabilityPackClass.DEVICE_NETWORK) {
                issues.add(new CapabilityPackValidationException.Issue(
                        "UNSUPPORTED_PACK_PROFILE",
                        "$[" + i + "].packClass",
                        "EMC1.1 registry admits DEVICE_NETWORK (schemaVersion 1.0.0) only; got "
                                + pack.packClass() + "."));
            }
            if (!CapabilityPackLoader.CONTRACT_SCHEMA_VERSION.equals(pack.schemaVersion())) {
                issues.add(new CapabilityPackValidationException.Issue(
                        "UNSUPPORTED_SCHEMA_VERSION",
                        "$[" + i + "].schemaVersion",
                        "Expected " + CapabilityPackLoader.CONTRACT_SCHEMA_VERSION + "."));
            }

            String identity = pack.identityKey();
            if (byIdentity.containsKey(identity)) {
                issues.add(new CapabilityPackValidationException.Issue(
                        "DUPLICATE_PACK_IDENTITY",
                        "$[" + i + "]",
                        "Duplicate packId+packVersion: " + identity + "."));
            } else {
                byIdentity.put(identity, pack);
            }

            if (byPackId.containsKey(pack.packId())) {
                issues.add(new CapabilityPackValidationException.Issue(
                        "DUPLICATE_PACK_ID",
                        "$[" + i + "]",
                        "Active snapshot admits only one version per packId: " + pack.packId() + "."));
            } else {
                byPackId.put(pack.packId(), pack);
            }
        }

        for (int i = 0; i < packs.size(); i++) {
            CapabilityPackDescriptor pack = packs.get(i);
            if (pack == null) {
                continue;
            }
            List<PackDependency> deps = pack.dependencies();
            for (int d = 0; d < deps.size(); d++) {
                PackDependency dep = deps.get(d);
                if (dep.packId().equals(pack.packId())) {
                    issues.add(new CapabilityPackValidationException.Issue(
                            "SELF_DEPENDENCY",
                            "$[" + i + "].dependencies[" + d + "]",
                            "A pack cannot depend on itself."));
                    continue;
                }
                if (!byIdentity.containsKey(dep.identityKey())) {
                    issues.add(new CapabilityPackValidationException.Issue(
                            "DEPENDENCY_NOT_FOUND",
                            "$[" + i + "].dependencies[" + d + "]",
                            "Dependency " + dep.identityKey() + " is not present."));
                }
            }
        }

        issues.addAll(detectDependencyCycles(byIdentity));

        if (!issues.isEmpty()) {
            CapabilityPackValidationException.Issue first = issues.get(0);
            throw new CapabilityPackValidationException(first.code(), first.message(), issues);
        }

        List<CapabilityPackDescriptor> immutable = List.copyOf(packs);
        Map<String, CapabilityPackDescriptor> identityCopy = Map.copyOf(byIdentity);
        return new Snapshot(immutable, identityCopy);
    }

    private static List<CapabilityPackValidationException.Issue> detectDependencyCycles(
            Map<String, CapabilityPackDescriptor> byIdentity) {
        List<CapabilityPackValidationException.Issue> issues = new ArrayList<>();
        Map<String, Integer> state = new HashMap<>();
        List<String> stack = new ArrayList<>();

        for (String identityKey : byIdentity.keySet()) {
            visit(identityKey, byIdentity, state, stack, issues);
        }
        return issues;
    }

    /**
     * DFS cycle detection over exact pack identities ({@code packId@packVersion}).
     */
    private static void visit(
            String identityKey,
            Map<String, CapabilityPackDescriptor> byIdentity,
            Map<String, Integer> state,
            List<String> stack,
            List<CapabilityPackValidationException.Issue> issues
    ) {
        int current = state.getOrDefault(identityKey, 0);
        if (current == 2) {
            return;
        }
        if (current == 1) {
            int start = stack.indexOf(identityKey);
            List<String> cycle = new ArrayList<>(stack.subList(Math.max(start, 0), stack.size()));
            cycle.add(identityKey);
            issues.add(new CapabilityPackValidationException.Issue(
                    "DEPENDENCY_CYCLE",
                    "$.dependencies",
                    "Dependency cycle: " + String.join(" -> ", cycle) + "."));
            return;
        }
        state.put(identityKey, 1);
        stack.add(identityKey);
        CapabilityPackDescriptor entry = byIdentity.get(identityKey);
        if (entry != null) {
            for (PackDependency dep : entry.dependencies()) {
                String depIdentity = dep.identityKey();
                if (byIdentity.containsKey(depIdentity)) {
                    visit(depIdentity, byIdentity, state, stack, issues);
                }
            }
        }
        stack.remove(stack.size() - 1);
        state.put(identityKey, 2);
    }

    record Snapshot(List<CapabilityPackDescriptor> packs, Map<String, CapabilityPackDescriptor> byIdentity) {
        static Snapshot empty() {
            return new Snapshot(List.of(), Map.of());
        }
    }
}
