package com.platform.core.ingestion;

import org.apache.kafka.clients.consumer.ConsumerRecord;
import org.junit.jupiter.api.Test;
import org.junit.jupiter.params.ParameterizedTest;
import org.junit.jupiter.params.provider.EnumSource;
import org.springframework.kafka.event.ContainerStoppedEvent;
import org.springframework.kafka.event.ConsumerStartingEvent;
import org.springframework.kafka.event.ConsumerStoppedEvent;
import org.springframework.kafka.listener.ConcurrentMessageListenerContainer;
import org.springframework.kafka.listener.KafkaMessageListenerContainer;

import java.util.ArrayList;
import java.util.List;
import java.util.concurrent.Executors;
import java.util.concurrent.TimeUnit;

import static org.junit.jupiter.api.Assertions.*;
import static org.mockito.ArgumentMatchers.any;
import static org.mockito.Mockito.*;

/** Lifecycle fixtures; the registered-listener/broker assertions live in TelemetryKafkaTransactionVisibilityTest. */
class IngestionTerminalStopProvenanceTest {
    private final IngestionMetrics metrics = new IngestionMetrics();
    @SuppressWarnings("unchecked")
    private final ConcurrentMessageListenerContainer<String, String> parent = mock(ConcurrentMessageListenerContainer.class);
    private final List<Runnable> stopTasks = new java.util.concurrent.CopyOnWriteArrayList<>();
    private final List<KafkaMessageListenerContainer<?, ?>> children = new ArrayList<>();
    private IngestionMetrics.TerminalStopProvenance provenance;

    private void configure(int concurrency) {
        when(parent.getListenerId()).thenReturn("telemetryNumericListener");
        when(parent.getConcurrency()).thenReturn(concurrency);
        provenance = metrics.trackTerminalStops(parent);
        doAnswer(invocation -> {
            // Spring 3.2.4 publishes this before consumer completion.
            provenance.parentStopping(false);
            provenance.onEvent(new ContainerStoppedEvent(parent, parent));
            return null;
        }).when(parent).stopAbnormally(any(Runnable.class));
        for (int i = 0; i < concurrency; i++) {
            var child = mock(KafkaMessageListenerContainer.class);
            when(child.getListenerId()).thenReturn("telemetryNumericListener-" + i);
            children.add(child);
        }
        provenance.beginGeneration();
        provenance.establishChildren(children);
    }

    private ConsumerRecord<String, String> record(int partition) {
        return new ConsumerRecord<>("telemetry.numeric.v1", partition, 17L, "key", "PRIVATE_PAYLOAD");
    }

    private RuntimeException terminal(int partition, TelemetryConsumerFailure reason) {
        var failure = new RuntimeException(reason.reasonCode);
        metrics.recordTerminalFailure(record(partition), reason, failure);
        return failure;
    }

    private void requestStop(Exception failure, ConsumerRecord<?, ?> record) {
        var handler = IngestionKafkaConfiguration.terminalStoppingErrorHandler(metrics, stopTasks::add);
        assertThrows(org.springframework.kafka.KafkaException.class,
                () -> handler.handleRemaining(failure, List.of(record), null, parent));
    }

    private void starting(int child) {
        provenance.onEvent(new ConsumerStartingEvent(children.get(child), parent));
    }

    private void stopped(int child) {
        provenance.onEvent(new ConsumerStoppedEvent(children.get(child), parent, ConsumerStoppedEvent.Reason.NORMAL));
    }

    @Test
    void retiredChildEventsCannotAlterCompletedNewGenerationEvidence() {
        configure(1);
        starting(0);
        requestStop(terminal(0, TelemetryConsumerFailure.DEPENDENCY), record(0));
        var oldStop = stopTasks.remove(0);
        oldStop.run();
        stopped(0);
        var oldChild = children.get(0);
        var newChild = mock(KafkaMessageListenerContainer.class);
        when(newChild.getListenerId()).thenReturn("telemetryNumericListener-0");
        children.set(0, newChild);
        provenance.beginGeneration();
        provenance.establishChildren(children);
        starting(0);
        requestStop(terminal(0, TelemetryConsumerFailure.DEPENDENCY), record(0));
        stopTasks.remove(0).run();
        stopped(0);
        var established = metrics.getConsumerTerminalStop();
        assertEquals("RECOVERABLE_DEPENDENCY_STOP", established.status());
        oldStop.run();
        provenance.onEvent(new ConsumerStoppedEvent(oldChild, parent, ConsumerStoppedEvent.Reason.NORMAL));
        provenance.onEvent(new ConsumerStartingEvent(oldChild, parent));
        assertEquals(established, metrics.getConsumerTerminalStop(), "retired sources have no new-generation authority");
    }

    @Test
    void uncorrelatedParentNotificationCannotAlterCompletedGenerationEvidence() {
        configure(1);
        starting(0);
        requestStop(terminal(0, TelemetryConsumerFailure.DEPENDENCY), record(0));
        stopTasks.remove(0).run();
        stopped(0);
        var established = metrics.getConsumerTerminalStop();
        provenance.onEvent(new ContainerStoppedEvent(parent, parent));
        assertEquals(established, metrics.getConsumerTerminalStop(), "notification has no incarnation or completion proof");
    }

    @Test
    void laterSiblingStartingKeepsProvisionalPoisonAndParentEpoch() throws Exception {
        configure(2);
        provenance.beginGeneration(); // Actual parent hook, before either child starts.
        when(parent.isRunning()).thenReturn(true); // Spring sets this before starting children.
        try (var first = Executors.newSingleThreadExecutor(); var second = Executors.newSingleThreadExecutor()) {
            first.submit(() -> {
                starting(0);
                metrics.recordConsumerFailure(record(0), TelemetryConsumerFailure.POISON);
                terminal(0, TelemetryConsumerFailure.POISON);
            }).get(5, TimeUnit.SECONDS);
            var before = metrics.getConsumerTerminalStop();
            assertEquals("GENERATION_UNCERTAIN", before.status());
            second.submit(() -> starting(1)).get(5, TimeUnit.SECONDS);
            assertEquals(before.failure(), metrics.getConsumerTerminalStop().failure());
            provenance.establishChildren(children); // Parent has now added both child objects.
            assertEquals(before.failure(), metrics.getConsumerTerminalStop().failure());
            assertEquals("TERMINAL_FAILURE", metrics.getConsumerTerminalStop().status());
            assertTrue(metrics.isConsumerDegraded());
            assertEquals(TelemetryConsumerFailure.POISON.reasonCode, metrics.getConsumerDegradedReason());
        }
    }

    @Test
    void partialChildCompletionCannotCompleteParentStop() throws Exception {
        configure(2);
        try (var first = Executors.newSingleThreadExecutor(); var second = Executors.newSingleThreadExecutor()) {
            first.submit(() -> starting(0)).get(5, TimeUnit.SECONDS);
            second.submit(() -> starting(1)).get(5, TimeUnit.SECONDS);
            first.submit(() -> requestStop(terminal(0, TelemetryConsumerFailure.DEPENDENCY), record(0)))
                    .get(5, TimeUnit.SECONDS);
            stopTasks.remove(0).run();
            first.submit(() -> stopped(0)).get(5, TimeUnit.SECONDS);
            assertFalse(metrics.getConsumerTerminalStop().stopCompleted());
            assertEquals("TERMINAL_FAILURE", metrics.getConsumerTerminalStop().status());
            provenance.onEvent(new ContainerStoppedEvent(parent, parent));
            assertFalse(metrics.getConsumerTerminalStop().stopCompleted());
        }
    }

    @Test
    void lateOldCompletionCannotCompleteNewGenerationOrClearBlockedRecord() {
        configure(1);
        starting(0);
        metrics.recordConsumerFailure(record(0), TelemetryConsumerFailure.DEPENDENCY);
        requestStop(terminal(0, TelemetryConsumerFailure.DEPENDENCY), record(0));
        var staleStop = stopTasks.remove(0);
        var oldChild = children.get(0);
        long oldEpoch = metrics.getConsumerTerminalStop().failure().stopEpoch();
        provenance.parentStopping(true); // Previous consumer completion was not observed.
        var newChild = mock(KafkaMessageListenerContainer.class);
        when(newChild.getListenerId()).thenReturn("telemetryNumericListener-0");
        children.set(0, newChild);
        provenance.beginGeneration();
        provenance.establishChildren(children);
        starting(0);
        assertNull(metrics.getConsumerTerminalStop(), "no previous-generation terminal authority");
        metrics.recordConsumerHealthy(record(1));
        assertTrue(metrics.isConsumerDegraded(), "restart and sibling ACK do not clear the blocked record");
        requestStop(terminal(0, TelemetryConsumerFailure.DEPENDENCY), record(0));
        assertTrue(metrics.getConsumerTerminalStop().failure().stopEpoch() > oldEpoch);
        staleStop.run();
        verify(parent, never()).stopAbnormally(any(Runnable.class));
        stopTasks.remove(0).run();
        provenance.onEvent(new ConsumerStoppedEvent(oldChild, parent, ConsumerStoppedEvent.Reason.NORMAL));
        assertFalse(metrics.getConsumerTerminalStop().stopCompleted(), "old child is not a new child completion");
        stopped(0);
        assertEquals("RECOVERABLE_DEPENDENCY_STOP", metrics.getConsumerTerminalStop().status());
        assertTrue(metrics.isConsumerDegraded());
        metrics.recordConsumerHealthy(record(0)); // Existing exact-record persistence-plus-ACK proof.
        assertFalse(metrics.isConsumerDegraded());
    }

    @Test
    void lateOldConsumerCannotMintAStopForNewGeneration() throws Exception {
        configure(1);
        try (var oldThread = Executors.newSingleThreadExecutor(); var newThread = Executors.newSingleThreadExecutor()) {
            oldThread.submit(() -> starting(0)).get(5, TimeUnit.SECONDS);
            var oldFailure = oldThread.submit(() -> terminal(0, TelemetryConsumerFailure.POISON))
                    .get(5, TimeUnit.SECONDS);
            var newChild = mock(KafkaMessageListenerContainer.class);
            when(newChild.getListenerId()).thenReturn("telemetryNumericListener-0");
            children.set(0, newChild);
            provenance.beginGeneration();
            provenance.establishChildren(children);
            newThread.submit(() -> starting(0)).get(5, TimeUnit.SECONDS);
            oldThread.submit(() -> requestStop(oldFailure, record(0))).get(5, TimeUnit.SECONDS);
            stopTasks.remove(0).run();
            verify(parent, never()).stopAbnormally(any(Runnable.class));
            assertNull(metrics.getConsumerTerminalStop());
        }
    }

    @Test
    void provisionalOldSourceCannotRetainStopAuthorityAfterMembershipValidation() {
        configure(1);
        provenance.beginGeneration();
        starting(0); // A delayed old child arrives before authoritative construction finishes.
        requestStop(terminal(0, TelemetryConsumerFailure.POISON), record(0));
        var oldStop = stopTasks.remove(0);
        var newChild = mock(KafkaMessageListenerContainer.class);
        when(newChild.getListenerId()).thenReturn("telemetryNumericListener-0");
        children.set(0, newChild);
        provenance.establishChildren(children); // Old source is not in the actual parent's child set.
        starting(0);
        oldStop.run();
        verify(parent, never()).stopAbnormally(any(Runnable.class));
        assertNull(metrics.getConsumerTerminalStop());
    }

    @ParameterizedTest
    @org.junit.jupiter.params.provider.ValueSource(booleans = {false, true})
    void retiredCompletionBeforeMembershipCannotVetoCurrentGeneration(boolean oldStartingObserved) {
        configure(1);
        var oldChild = children.get(0);
        provenance.beginGeneration();
        if (oldStartingObserved) starting(0);
        provenance.onEvent(new ConsumerStoppedEvent(oldChild, parent, ConsumerStoppedEvent.Reason.AUTH));
        var newChild = mock(KafkaMessageListenerContainer.class);
        when(newChild.getListenerId()).thenReturn("telemetryNumericListener-0");
        children.set(0, newChild);
        provenance.establishChildren(children);
        starting(0);
        requestStop(terminal(0, TelemetryConsumerFailure.DEPENDENCY), record(0));
        stopTasks.remove(0).run();
        stopped(0);
        assertEquals("RECOVERABLE_DEPENDENCY_STOP", metrics.getConsumerTerminalStop().status(),
                "only actual current members can veto this generation");
    }

    @ParameterizedTest
    @org.junit.jupiter.params.provider.CsvSource({"1,false", "1,true", "2,false", "2,true"})
    void stampedPublishersPreserveCurrentStartsBeforeMembership(int concurrency, boolean oldAuth) throws Exception {
        configure(concurrency);
        var downstream = mock(org.springframework.context.ApplicationEventPublisher.class);
        var retiredPublisher = provenance.beginGenerationPublisher(downstream);
        var retired = List.copyOf(children);
        var currentPublisher = provenance.beginGenerationPublisher(downstream);
        for (var child : retired) {
            retiredPublisher.publishEvent(new ConsumerStartingEvent(child, parent));
            if (oldAuth) retiredPublisher.publishEvent(new ConsumerStoppedEvent(child, parent, ConsumerStoppedEvent.Reason.AUTH));
        }
        var threads = new ArrayList<java.util.concurrent.ExecutorService>();
        try {
            for (int i = 0; i < concurrency; i++) {
                var child = mock(KafkaMessageListenerContainer.class);
                when(child.getListenerId()).thenReturn("telemetryNumericListener-" + i);
                children.set(i, child);
                var thread = Executors.newSingleThreadExecutor();
                threads.add(thread);
                thread.submit(() -> currentPublisher.publishEvent(new ConsumerStartingEvent(child, parent)))
                        .get(5, TimeUnit.SECONDS);
            }
            // Every current Starting event precedes authoritative reconciliation, just as in Spring.
            threads.get(0).submit(() -> requestStop(terminal(0, TelemetryConsumerFailure.DEPENDENCY), record(0)))
                    .get(5, TimeUnit.SECONDS);
            assertEquals("GENERATION_UNCERTAIN", metrics.getConsumerTerminalStop().status());
            assertFalse(metrics.getConsumerTerminalStop().stopCompleted());
            provenance.establishChildren(children);
            stopTasks.remove(0).run();
            verify(parent, times(1)).stopAbnormally(any(Runnable.class));
            assertFalse(metrics.getConsumerTerminalStop().stopCompleted(), "initiation is not completion");
            for (int i = 0; i < concurrency; i++) {
                var child = children.get(i);
                threads.get(i).submit(() -> currentPublisher.publishEvent(
                        new ConsumerStoppedEvent(child, parent, ConsumerStoppedEvent.Reason.NORMAL)))
                        .get(5, TimeUnit.SECONDS);
            }
            var completed = metrics.getConsumerTerminalStop();
            assertEquals("RECOVERABLE_DEPENDENCY_STOP", completed.status());
            retiredPublisher.publishEvent(new ConsumerStartingEvent(retired.get(0), parent));
            assertEquals(completed, metrics.getConsumerTerminalStop());
            // One downstream delivery per event; no generation wrapper chains.
            int events = concurrency * (oldAuth ? 4 : 3) + 1;
            verify(downstream, times(events)).publishEvent(any(Object.class));
        } finally {
            threads.forEach(java.util.concurrent.ExecutorService::shutdownNow);
        }
    }

    @Test
    void onlyMatchedConsumerCompletionMakesDependencyStopCandidate() {
        configure(1);
        starting(0);
        var failure = terminal(0, TelemetryConsumerFailure.DEPENDENCY);
        var initial = metrics.getConsumerTerminalStop();
        assertEquals("TERMINAL_FAILURE", initial.status());
        assertFalse(initial.stopCompleted());
        assertEquals("telemetryNumericListener", initial.failure().listener());
        assertEquals("telemetryNumericListener-0", initial.failure().child());
        assertEquals(17L, initial.failure().offset());
        assertFalse(initial.toString().contains("PRIVATE_PAYLOAD"));
        requestStop(new RuntimeException("framework wrapper", failure), record(0));
        stopTasks.remove(0).run();
        assertFalse(metrics.getConsumerTerminalStop().stopCompleted(), "ContainerStoppedEvent is premature");
        assertEquals(initial, metrics.getConsumerTerminalStop(), "retained terminal evidence is immutable");
        stopped(0);
        assertEquals("RECOVERABLE_DEPENDENCY_STOP", metrics.getConsumerTerminalStop().status());
        assertTrue(metrics.getConsumerTerminalStop().stopCompleted());
        verify(parent, times(1)).stopAbnormally(any(Runnable.class));
        verify(parent, never()).start();
    }

    @ParameterizedTest
    @EnumSource(value = TelemetryConsumerFailure.class, names = "DEPENDENCY", mode = EnumSource.Mode.EXCLUDE)
    void everyNonDependencyTerminalReasonRemainsIneligible(TelemetryConsumerFailure reason) {
        configure(1);
        starting(0);
        requestStop(terminal(0, reason), record(0));
        stopTasks.remove(0).run();
        stopped(0);
        assertEquals("INELIGIBLE_STOP", metrics.getConsumerTerminalStop().status());
        assertEquals(reason, metrics.getConsumerTerminalStop().failure().reason());
    }

    @Test
    void transientAttemptMetricsCannotBecomeTerminalEvidence() {
        configure(1);
        starting(0);
        metrics.recordConsumerFailure(record(0), TelemetryConsumerFailure.DEPENDENCY);
        metrics.recordConsumerHealthy(record(0));
        provenance.onEvent(new ContainerStoppedEvent(parent, parent));
        stopped(0);
        assertNull(metrics.getConsumerTerminalStop());
    }

    @Test
    void sameReasonTextCannotSubstituteForExceptionIdentity() {
        configure(1);
        starting(0);
        terminal(0, TelemetryConsumerFailure.DEPENDENCY);
        requestStop(new RuntimeException(TelemetryConsumerFailure.DEPENDENCY.reasonCode), record(0));
        stopTasks.remove(0).run();
        stopped(0);
        assertEquals("INELIGIBLE_STOP", metrics.getConsumerTerminalStop().status());
    }

    @ParameterizedTest
    @org.junit.jupiter.params.provider.ValueSource(strings = {"topic", "partition", "offset"})
    void exactRecordCoordinatesAreRequiredEvenWithTheOriginalException(String mismatch) {
        configure(1);
        starting(0);
        var failure = terminal(0, TelemetryConsumerFailure.DEPENDENCY);
        var different = new ConsumerRecord<>(mismatch.equals("topic") ? "other.topic" : "telemetry.numeric.v1",
                mismatch.equals("partition") ? 1 : 0, mismatch.equals("offset") ? 18L : 17L, "key", "{}");
        requestStop(failure, different);
        stopTasks.remove(0).run();
        stopped(0);
        assertEquals("INELIGIBLE_STOP", metrics.getConsumerTerminalStop().status());
    }

    @Test
    void manualStopVetoesEvenPreviouslyBoundTerminalFailure() {
        configure(1);
        starting(0);
        requestStop(terminal(0, TelemetryConsumerFailure.DEPENDENCY), record(0));
        provenance.parentStopping(true);
        provenance.onEvent(new ContainerStoppedEvent(parent, parent));
        stopTasks.remove(0).run();
        stopped(0);
        assertEquals("INELIGIBLE_STOP", metrics.getConsumerTerminalStop().status());
    }

    @Test
    void contextShutdownVetoesAnAlreadyCompletedCandidate() {
        configure(1);
        starting(0);
        requestStop(terminal(0, TelemetryConsumerFailure.DEPENDENCY), record(0));
        stopTasks.remove(0).run();
        stopped(0);
        assertEquals("RECOVERABLE_DEPENDENCY_STOP", metrics.getConsumerTerminalStop().status());
        assertFalse(metrics.supportsAsyncExecution());
        try (var context = new org.springframework.context.support.GenericApplicationContext()) {
            metrics.onApplicationEvent(new org.springframework.context.event.ContextClosedEvent(context));
        }
        assertEquals("INELIGIBLE_STOP", metrics.getConsumerTerminalStop().status());
    }

    @Test
    void factoryCorrelatesAtPublicationBeforeAsyncMulticasterDelivery() {
        var configuration = new IngestionKafkaConfiguration();
        var consumerFactory = configuration.telemetryKafkaConsumerFactory(
                new org.springframework.boot.autoconfigure.kafka.KafkaProperties());
        // Queued application delivery deliberately never runs during this assertion.
        var deliveries = new ArrayList<Runnable>();
        var multicaster = new org.springframework.context.event.SimpleApplicationEventMulticaster();
        multicaster.setTaskExecutor(deliveries::add);
        multicaster.addApplicationListener(event -> { });
        var factory = configuration.telemetryKafkaListenerContainerFactory(consumerFactory, metrics,
                event -> multicaster.multicastEvent((org.springframework.context.ApplicationEvent) event));
        var endpoint = mock(org.springframework.kafka.config.KafkaListenerEndpoint.class);
        when(endpoint.getId()).thenReturn("telemetryNumericListener");
        when(endpoint.getTopics()).thenReturn(List.of("telemetry.numeric.v1"));
        when(endpoint.getConcurrency()).thenReturn(1);
        var container = factory.createListenerContainer(endpoint);
        var publisher = (org.springframework.context.ApplicationEventPublisher)
                org.springframework.test.util.ReflectionTestUtils.invokeMethod(container, "getApplicationEventPublisher");
        assertNotNull(publisher);
        var child = mock(KafkaMessageListenerContainer.class);
        when(child.getListenerId()).thenReturn("telemetryNumericListener-0");
        var tracker = (IngestionMetrics.TerminalStopProvenance)
                org.springframework.test.util.ReflectionTestUtils.getField(container, "provenance");
        assertNotNull(tracker);
        publisher = tracker.beginGenerationPublisher(publisher);
        tracker.establishChildren(List.of(child));
        publisher.publishEvent(new ConsumerStartingEvent(child, container));
        metrics.recordTerminalFailure(record(0), TelemetryConsumerFailure.DEPENDENCY, new RuntimeException("safe"));
        assertEquals("TERMINAL_FAILURE", metrics.getConsumerTerminalStop().status());
        assertFalse(deliveries.isEmpty(), "application delivery is still queued");
        // No handler provenance: even a synchronous stopped event cannot authorize recovery.
        tracker.parentStopping(true);
        publisher.publishEvent(new ContainerStoppedEvent(container, container));
        publisher.publishEvent(new ConsumerStoppedEvent(child, container, ConsumerStoppedEvent.Reason.NORMAL));
        assertEquals("INELIGIBLE_STOP", metrics.getConsumerTerminalStop().status());
    }

    @ParameterizedTest
    @EnumSource(value = ConsumerStoppedEvent.Reason.class, names = "NORMAL", mode = EnumSource.Mode.EXCLUDE)
    void abnormalKafkaReasonsVetoEvenAMatchedDependencyFailure(ConsumerStoppedEvent.Reason reason) {
        configure(1);
        starting(0);
        requestStop(terminal(0, TelemetryConsumerFailure.DEPENDENCY), record(0));
        stopTasks.remove(0).run();
        provenance.onEvent(new ConsumerStoppedEvent(children.get(0), parent, reason));
        assertEquals("INELIGIBLE_STOP", metrics.getConsumerTerminalStop().status());
    }

    @Test
    void nonMemberChildEventsCannotGrowOrInvalidateTracking() {
        configure(1);
        starting(0);
        requestStop(terminal(0, TelemetryConsumerFailure.DEPENDENCY), record(0));
        for (int extra = 0; extra < 100; extra++) {
            provenance.onEvent(new ConsumerStartingEvent(mock(KafkaMessageListenerContainer.class), parent));
        }
        stopTasks.remove(0).run();
        stopped(0);
        assertEquals("RECOVERABLE_DEPENDENCY_STOP", metrics.getConsumerTerminalStop().status());
    }

    @Test
    void unknownConsumerErrorVetoesDependencyCandidate() {
        configure(1);
        starting(0);
        requestStop(terminal(0, TelemetryConsumerFailure.DEPENDENCY), record(0));
        var handler = IngestionKafkaConfiguration.terminalStoppingErrorHandler(metrics, stopTasks::add);
        assertThrows(org.springframework.kafka.KafkaException.class,
                () -> handler.handleOtherException(new IllegalStateException("unknown"), null, parent, false));
        stopTasks.forEach(Runnable::run);
        stopped(0);
        assertEquals("INELIGIBLE_STOP", metrics.getConsumerTerminalStop().status());
    }

    @Test
    void staleChildAndStopRequestCannotOverrideNewEpoch() {
        configure(1);
        starting(0);
        requestStop(terminal(0, TelemetryConsumerFailure.DEPENDENCY), record(0));
        var staleStop = stopTasks.remove(0);
        staleStop.run();
        stopped(0);
        long oldEpoch = metrics.getConsumerTerminalStop().failure().stopEpoch();
        var oldChild = children.get(0);
        var newChild = mock(KafkaMessageListenerContainer.class);
        when(newChild.getListenerId()).thenReturn("telemetryNumericListener-0");
        children.set(0, newChild);
        provenance.beginGeneration();
        provenance.establishChildren(children);
        starting(0);
        assertNull(metrics.getConsumerTerminalStop());
        requestStop(terminal(0, TelemetryConsumerFailure.DEPENDENCY), record(0));
        staleStop.run();
        provenance.onEvent(new ConsumerStoppedEvent(oldChild, parent, ConsumerStoppedEvent.Reason.NORMAL));
        stopTasks.remove(0).run();
        stopped(0);
        assertTrue(metrics.getConsumerTerminalStop().failure().stopEpoch() > oldEpoch);
        assertEquals("RECOVERABLE_DEPENDENCY_STOP", metrics.getConsumerTerminalStop().status());
    }

    @Test
    void asynchronousOrWrongChildCompletionCannotProveStop() throws Exception {
        configure(1);
        starting(0);
        requestStop(terminal(0, TelemetryConsumerFailure.DEPENDENCY), record(0));
        stopTasks.remove(0).run();
        try (var executor = Executors.newSingleThreadExecutor()) {
            executor.submit(() -> stopped(0)).get(5, TimeUnit.SECONDS);
        }
        assertFalse(metrics.getConsumerTerminalStop().stopCompleted());
        stopped(0);
        assertEquals("INELIGIBLE_STOP", metrics.getConsumerTerminalStop().status());
    }

    @ParameterizedTest
    @EnumSource(value = TelemetryConsumerFailure.class, names = {"POISON", "DEPENDENCY"})
    void siblingTerminalFailuresVetoAnOtherwiseRecoverableChild(TelemetryConsumerFailure siblingReason) throws Exception {
        configure(2);
        try (var first = Executors.newSingleThreadExecutor(); var second = Executors.newSingleThreadExecutor()) {
            first.submit(() -> starting(0)).get(5, TimeUnit.SECONDS);
            second.submit(() -> starting(1)).get(5, TimeUnit.SECONDS);
            first.submit(() -> requestStop(terminal(0, TelemetryConsumerFailure.DEPENDENCY), record(0)))
                    .get(5, TimeUnit.SECONDS);
            second.submit(() -> requestStop(terminal(1, siblingReason), record(1))).get(5, TimeUnit.SECONDS);
            stopTasks.forEach(Runnable::run);
            first.submit(() -> stopped(0)).get(5, TimeUnit.SECONDS);
            assertFalse(metrics.getConsumerTerminalStop().stopCompleted());
            second.submit(() -> stopped(1)).get(5, TimeUnit.SECONDS);
        }
        assertTrue(metrics.getConsumerTerminalStop().stopCompleted());
        assertEquals("INELIGIBLE_STOP", metrics.getConsumerTerminalStop().status());
    }

    @Test
    void successfulSiblingCannotClearTerminalEvidenceAndAllChildrenMustFinish() throws Exception {
        configure(2);
        try (var first = Executors.newSingleThreadExecutor(); var second = Executors.newSingleThreadExecutor()) {
            first.submit(() -> starting(0)).get(5, TimeUnit.SECONDS);
            second.submit(() -> starting(1)).get(5, TimeUnit.SECONDS);
            first.submit(() -> requestStop(terminal(0, TelemetryConsumerFailure.DEPENDENCY), record(0)))
                    .get(5, TimeUnit.SECONDS);
            second.submit(() -> metrics.recordConsumerHealthy(record(1))).get(5, TimeUnit.SECONDS);
            stopTasks.remove(0).run();
            first.submit(() -> stopped(0)).get(5, TimeUnit.SECONDS);
            assertFalse(metrics.getConsumerTerminalStop().stopCompleted());
            second.submit(() -> stopped(1)).get(5, TimeUnit.SECONDS);
        }
        assertEquals("RECOVERABLE_DEPENDENCY_STOP", metrics.getConsumerTerminalStop().status());
    }
}
