package com.platform.core.domain.capabilitypack;

import org.junit.jupiter.api.Test;

import java.util.ArrayList;
import java.util.List;

import static org.junit.jupiter.api.Assertions.assertEquals;
import static org.junit.jupiter.api.Assertions.assertThrows;
import static org.junit.jupiter.api.Assertions.assertTrue;

class CapabilityPackRegistryTest {

    @Test
    void replaceAllAtomicallyCommitsValidSnapshot() {
        CapabilityPackRegistry registry = new CapabilityPackRegistry();
        CapabilityPackDescriptor a = CapabilityPackTestSupport.loadMinimal("fixture.fictional.reg-a", "1.0.0", "A");
        CapabilityPackDescriptor b = CapabilityPackTestSupport.loadMinimal("fixture.fictional.reg-b", "1.0.0", "B");

        registry.replaceAll(List.of(a));
        assertEquals(1, registry.size());

        registry.replaceAll(List.of(a, b));
        assertEquals(2, registry.size());
        assertTrue(registry.find("fixture.fictional.reg-b", "1.0.0").isPresent());
    }

    @Test
    void replaceAllKeepsPreviousSnapshotOnFailure() {
        CapabilityPackRegistry registry = new CapabilityPackRegistry();
        CapabilityPackDescriptor good = CapabilityPackTestSupport.loadMinimal("fixture.fictional.keep", "1.0.0", "Keep");
        registry.replaceAll(List.of(good));

        CapabilityPackDescriptor bad = CapabilityPackTestSupport.withDependency(
                "fixture.fictional.missing-dep", "1.0.0", "Missing", "fixture.fictional.absent", "1.0.0");

        CapabilityPackValidationException ex = assertThrows(
                CapabilityPackValidationException.class,
                () -> registry.replaceAll(List.of(good, bad)));
        assertEquals("DEPENDENCY_NOT_FOUND", ex.getCode());
        assertEquals(1, registry.size());
        assertEquals("fixture.fictional.keep", registry.list().get(0).packId());
    }

    @Test
    void rejectsDuplicatePackIdAndVersion() {
        CapabilityPackRegistry registry = new CapabilityPackRegistry();
        CapabilityPackDescriptor a = CapabilityPackTestSupport.loadMinimal("fixture.fictional.dup", "1.0.0", "A");
        CapabilityPackDescriptor b = CapabilityPackTestSupport.loadMinimal("fixture.fictional.dup", "1.0.0", "B");

        CapabilityPackValidationException ex = assertThrows(
                CapabilityPackValidationException.class,
                () -> registry.replaceAll(List.of(a, b)));
        assertEquals("DUPLICATE_PACK_IDENTITY", ex.getCode());
        assertEquals(0, registry.size());
    }

    @Test
    void rejectsSelfDependencyAndCycle() {
        CapabilityPackRegistry registry = new CapabilityPackRegistry();
        CapabilityPackDescriptor self = CapabilityPackTestSupport.withDependency(
                "fixture.fictional.self", "1.0.0", "Self", "fixture.fictional.self", "1.0.0");
        CapabilityPackValidationException selfEx = assertThrows(
                CapabilityPackValidationException.class,
                () -> registry.replaceAll(List.of(self)));
        assertEquals("SELF_DEPENDENCY", selfEx.getCode());

        CapabilityPackDescriptor a = CapabilityPackTestSupport.withDependency(
                "fixture.fictional.cycle-a", "1.0.0", "A", "fixture.fictional.cycle-b", "1.0.0");
        CapabilityPackDescriptor b = CapabilityPackTestSupport.withDependency(
                "fixture.fictional.cycle-b", "1.0.0", "B", "fixture.fictional.cycle-a", "1.0.0");
        CapabilityPackValidationException cycleEx = assertThrows(
                CapabilityPackValidationException.class,
                () -> registry.replaceAll(List.of(a, b)));
        assertTrue(cycleEx.getIssues().stream().anyMatch(i -> "DEPENDENCY_CYCLE".equals(i.code())
                || "SELF_DEPENDENCY".equals(i.code())
                || "DEPENDENCY_NOT_FOUND".equals(i.code())
                || "DEPENDENCY_CYCLE".equals(cycleEx.getCode())));
        assertTrue(
                "DEPENDENCY_CYCLE".equals(cycleEx.getCode())
                        || cycleEx.getIssues().stream().anyMatch(i -> "DEPENDENCY_CYCLE".equals(i.code())),
                () -> "Expected cycle, got " + cycleEx.getCode() + " issues=" + cycleEx.getIssues());
    }

    @Test
    void rejectsUnsupportedPackProfile() {
        CapabilityPackRegistry registry = new CapabilityPackRegistry();
        CapabilityPackDescriptor unsupported = CapabilityPackTestSupport.unsupportedProfileDescriptor();
        CapabilityPackValidationException ex = assertThrows(
                CapabilityPackValidationException.class,
                () -> registry.replaceAll(List.of(unsupported)));
        assertEquals("UNSUPPORTED_PACK_PROFILE", ex.getCode());
    }

    @Test
    void rejectsSnapshotAboveMaxPackCount() {
        CapabilityPackRegistry registry = new CapabilityPackRegistry();
        CapabilityPackDescriptor template = CapabilityPackTestSupport.loadMinimal(
                "fixture.fictional.template", "1.0.0", "Template");
        List<CapabilityPackDescriptor> packs = new ArrayList<>(CapabilityPackRegistry.MAX_PACK_COUNT + 1);
        for (int i = 0; i < CapabilityPackRegistry.MAX_PACK_COUNT + 1; i++) {
            packs.add(new CapabilityPackDescriptor(
                    "fixture.fictional.pack" + i,
                    template.packVersion(),
                    template.packClass(),
                    template.schemaVersion(),
                    template.name(),
                    template.publisher(),
                    template.certification(),
                    template.metadata(),
                    template.matchRules(),
                    template.capabilities(),
                    template.protocols(),
                    template.references(),
                    template.normalization(),
                    template.dependencies(),
                    template.provenance(),
                    template.manifestSha256()
            ));
        }
        CapabilityPackValidationException ex = assertThrows(
                CapabilityPackValidationException.class,
                () -> registry.replaceAll(packs));
        assertEquals("REGISTRY_TOO_LARGE", ex.getCode());
        assertEquals(0, registry.size());
    }

    @Test
    void acceptsResolvedDependencyGraph() {
        CapabilityPackRegistry registry = new CapabilityPackRegistry();
        CapabilityPackDescriptor base = CapabilityPackTestSupport.loadMinimal(
                "fixture.fictional.base", "1.0.0", "Base");
        CapabilityPackDescriptor dependent = CapabilityPackTestSupport.withDependency(
                "fixture.fictional.child", "1.0.0", "Child", "fixture.fictional.base", "1.0.0");
        registry.replaceAll(List.of(base, dependent));
        assertEquals(2, registry.size());
    }
}
