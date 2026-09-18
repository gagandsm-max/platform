package com.platform.core.domain.capabilitypack;

import org.junit.jupiter.api.Test;

import java.util.ArrayList;
import java.util.List;
import java.util.concurrent.CountDownLatch;
import java.util.concurrent.CyclicBarrier;
import java.util.concurrent.ExecutorService;
import java.util.concurrent.Executors;
import java.util.concurrent.TimeUnit;
import java.util.concurrent.atomic.AtomicInteger;
import java.util.concurrent.atomic.AtomicReference;

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
    void rejectsSecondVersionOfSamePackId() {
        CapabilityPackRegistry registry = new CapabilityPackRegistry();
        CapabilityPackDescriptor v1 = CapabilityPackTestSupport.loadMinimal("fixture.fictional.foo", "1.0.0", "Foo");
        CapabilityPackDescriptor v2 = CapabilityPackTestSupport.loadMinimal("fixture.fictional.foo", "2.0.0", "Foo");

        CapabilityPackValidationException ex = assertThrows(
                CapabilityPackValidationException.class,
                () -> registry.replaceAll(List.of(v1, v2)));
        assertEquals("DUPLICATE_PACK_ID", ex.getCode());
        assertEquals(0, registry.size());
    }

    @Test
    void acceptsExactDependencyVersionAndRejectsWrongVersion() {
        CapabilityPackRegistry registry = new CapabilityPackRegistry();
        CapabilityPackDescriptor base = CapabilityPackTestSupport.loadMinimal(
                "fixture.fictional.base", "1.0.0", "Base");
        CapabilityPackDescriptor dependent = CapabilityPackTestSupport.withDependency(
                "fixture.fictional.child", "1.0.0", "Child", "fixture.fictional.base", "1.0.0");
        registry.replaceAll(List.of(base, dependent));
        assertEquals(2, registry.size());

        CapabilityPackDescriptor wrongVersion = CapabilityPackTestSupport.withDependency(
                "fixture.fictional.child-bad", "1.0.0", "ChildBad", "fixture.fictional.base", "9.9.9");
        CapabilityPackValidationException ex = assertThrows(
                CapabilityPackValidationException.class,
                () -> registry.replaceAll(List.of(base, wrongVersion)));
        assertEquals("DEPENDENCY_NOT_FOUND", ex.getCode());
        assertEquals(2, registry.size());
    }

    @Test
    void rejectsThreeNodeDependencyCycleByExactIdentity() {
        CapabilityPackRegistry registry = new CapabilityPackRegistry();
        CapabilityPackDescriptor a = CapabilityPackTestSupport.withDependency(
                "fixture.fictional.cycle-a", "1.0.0", "A", "fixture.fictional.cycle-b", "1.0.0");
        CapabilityPackDescriptor b = CapabilityPackTestSupport.withDependency(
                "fixture.fictional.cycle-b", "1.0.0", "B", "fixture.fictional.cycle-c", "1.0.0");
        CapabilityPackDescriptor c = CapabilityPackTestSupport.withDependency(
                "fixture.fictional.cycle-c", "1.0.0", "C", "fixture.fictional.cycle-a", "1.0.0");

        CapabilityPackValidationException ex = assertThrows(
                CapabilityPackValidationException.class,
                () -> registry.replaceAll(List.of(a, b, c)));
        assertTrue(
                "DEPENDENCY_CYCLE".equals(ex.getCode())
                        || ex.getIssues().stream().anyMatch(i -> "DEPENDENCY_CYCLE".equals(i.code())),
                () -> "Expected cycle, got " + ex.getCode() + " issues=" + ex.getIssues());
        assertTrue(ex.getMessage().contains("@"));
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

    @Test
    void concurrentReadersSeeCompleteOldOrCompleteNewSnapshot() throws Exception {
        CapabilityPackRegistry registry = new CapabilityPackRegistry();
        CapabilityPackDescriptor oldA = CapabilityPackTestSupport.loadMinimal("fixture.fictional.old-a", "1.0.0", "OldA");
        CapabilityPackDescriptor oldB = CapabilityPackTestSupport.loadMinimal("fixture.fictional.old-b", "1.0.0", "OldB");
        CapabilityPackDescriptor newA = CapabilityPackTestSupport.loadMinimal("fixture.fictional.new-a", "1.0.0", "NewA");
        CapabilityPackDescriptor newB = CapabilityPackTestSupport.loadMinimal("fixture.fictional.new-b", "1.0.0", "NewB");
        CapabilityPackDescriptor newC = CapabilityPackTestSupport.loadMinimal("fixture.fictional.new-c", "1.0.0", "NewC");

        registry.replaceAll(List.of(oldA, oldB));

        int readers = 8;
        CyclicBarrier start = new CyclicBarrier(readers + 1);
        CountDownLatch writerDone = new CountDownLatch(1);
        CountDownLatch readersDone = new CountDownLatch(readers);
        AtomicInteger violations = new AtomicInteger();
        AtomicReference<String> sampleViolation = new AtomicReference<>();

        ExecutorService pool = Executors.newFixedThreadPool(readers + 1);
        try {
            for (int i = 0; i < readers; i++) {
                pool.execute(() -> {
                    try {
                        start.await(5, TimeUnit.SECONDS);
                        for (int probe = 0; probe < 2000; probe++) {
                            List<CapabilityPackDescriptor> view = registry.list();
                            int size = view.size();
                            boolean oldComplete = size == 2
                                    && view.stream().anyMatch(p -> p.packId().equals("fixture.fictional.old-a"))
                                    && view.stream().anyMatch(p -> p.packId().equals("fixture.fictional.old-b"));
                            boolean newComplete = size == 3
                                    && view.stream().anyMatch(p -> p.packId().equals("fixture.fictional.new-a"))
                                    && view.stream().anyMatch(p -> p.packId().equals("fixture.fictional.new-b"))
                                    && view.stream().anyMatch(p -> p.packId().equals("fixture.fictional.new-c"));
                            if (!oldComplete && !newComplete) {
                                violations.incrementAndGet();
                                sampleViolation.compareAndSet(null, "partial size=" + size + " ids="
                                        + view.stream().map(CapabilityPackDescriptor::packId).toList());
                                break;
                            }
                            if (writerDone.getCount() == 0 && newComplete) {
                                break;
                            }
                        }
                    } catch (Exception ex) {
                        violations.incrementAndGet();
                        sampleViolation.compareAndSet(null, ex.toString());
                    } finally {
                        readersDone.countDown();
                    }
                });
            }

            pool.execute(() -> {
                try {
                    start.await(5, TimeUnit.SECONDS);
                    registry.replaceAll(List.of(newA, newB, newC));
                } catch (Exception ex) {
                    violations.incrementAndGet();
                    sampleViolation.compareAndSet(null, ex.toString());
                } finally {
                    writerDone.countDown();
                }
            });

            assertTrue(writerDone.await(10, TimeUnit.SECONDS));
            assertTrue(readersDone.await(10, TimeUnit.SECONDS));
        } finally {
            pool.shutdownNow();
        }

        assertEquals(0, violations.get(), () -> "Concurrent snapshot violation: " + sampleViolation.get());
        assertEquals(3, registry.size());
        assertTrue(registry.find("fixture.fictional.new-c", "1.0.0").isPresent());
    }
}
