package com.platform.core.ingestion;

import org.springframework.stereotype.Component;

import java.time.Instant;
import java.util.concurrent.atomic.AtomicBoolean;
import java.util.concurrent.atomic.AtomicLong;
import java.util.concurrent.atomic.AtomicReference;

@Component
public class IngestionMetrics implements org.springframework.context.ApplicationListener<org.springframework.context.event.ContextClosedEvent> {
    private final AtomicLong batchesReceived = new AtomicLong(0);
    private final AtomicLong batchesRejected = new AtomicLong(0);
    private final AtomicLong batchesReplayed = new AtomicLong(0);
    private final AtomicLong recordsAccepted = new AtomicLong(0);

    private final AtomicLong kafkaPublishes = new AtomicLong(0);
    private final AtomicLong kafkaPublishTotalMs = new AtomicLong(0);
    private final AtomicLong kafkaFailures = new AtomicLong(0);

    private final AtomicLong clickHouseInserts = new AtomicLong(0);
    private final AtomicLong clickHouseInsertTotalMs = new AtomicLong(0);
    private final AtomicLong clickHouseFailures = new AtomicLong(0);

    private final AtomicLong consumerPoisonEvents = new AtomicLong(0);
    private final AtomicBoolean consumerDegraded = new AtomicBoolean(false);
    private final AtomicReference<String> consumerDegradedReason = new AtomicReference<>("");
    // One retained failure per blocked partition. A sibling's success cannot clear it.
    // A validated INSERT return is not a no-write proof for earlier attempts/redeliveries.
    // Missing history is uncertain; these facts never authorize replay.
    record InsertCompletion(boolean insertCompleted, boolean persistenceUncertain) {
        static final InsertCompletion UNKNOWN = new InsertCompletion(false, true);

        InsertCompletion merge(InsertCompletion other) {
            return new InsertCompletion(insertCompleted || other.insertCompleted,
                    persistenceUncertain || other.persistenceUncertain);
        }
    }
    private record ConsumerFailure(long offset, TelemetryConsumerFailure reason, InsertCompletion completion) { }
    private final java.util.Map<org.apache.kafka.common.TopicPartition, ConsumerFailure> consumerFailures =
            new java.util.LinkedHashMap<>();

    private final AtomicLong previousBatchPending = new AtomicLong(0);
    private final AtomicLong replayConflicts = new AtomicLong(0);
    private final AtomicLong sequenceGaps = new AtomicLong(0);
    private final AtomicLong sequenceRegressions = new AtomicLong(0);
    private final AtomicLong streamMismatches = new AtomicLong(0);
    private final AtomicLong publisherLeaseContentions = new AtomicLong(0);
    private final AtomicLong finalizeFailures = new AtomicLong(0);
    private final AtomicLong pendingReservationsObserved = new AtomicLong(0);

    private final AtomicReference<Instant> lastPersistenceTimestamp = new AtomicReference<>(null);
    private final AtomicReference<String> lastError = new AtomicReference<>("");

    // K2a is ephemeral stop provenance, never recovery authority or durable quarantine.
    private volatile TerminalStopProvenance terminalStops;
    private final AtomicLong terminalStopEpoch = new AtomicLong();
    private final ThreadLocal<StopRequest> executingStop = new ThreadLocal<>();

    @Override
    public boolean supportsAsyncExecution() {
        return false;
    }

    @Override
    public void onApplicationEvent(org.springframework.context.event.ContextClosedEvent event) {
        var provenance = terminalStops;
        if (provenance != null) provenance.shutdown();
    }

    record TerminalFailure(String listener, String child, String topic, int partition, long offset,
                           TelemetryConsumerFailure reason, long stopEpoch, InsertCompletion completion) { }
    record TerminalStop(TerminalFailure failure, String status, boolean stopCompleted) { }
    record StopRequest(TerminalStopProvenance provenance, long epoch, Object child) { }

    synchronized TerminalStopProvenance trackTerminalStops(
            org.springframework.kafka.listener.ConcurrentMessageListenerContainer<?, ?> parent) {
        if (terminalStops != null && !terminalStops.isComplete()) {
            throw new IllegalStateException("Telemetry stop provenance still active");
        }
        terminalStops = new TerminalStopProvenance(parent);
        return terminalStops;
    }

    void recordTerminalFailure(org.apache.kafka.clients.consumer.ConsumerRecord<?, ?> record,
                               TelemetryConsumerFailure reason, RuntimeException failure) {
        recordTerminalFailure(record, reason, failure, InsertCompletion.UNKNOWN);
    }

    void recordTerminalFailure(org.apache.kafka.clients.consumer.ConsumerRecord<?, ?> record,
                               TelemetryConsumerFailure reason, RuntimeException failure, InsertCompletion completion) {
        // Finish the metrics update before acquiring the lifecycle monitor.
        var retained = recordConsumerFailure(record, reason, completion);
        var provenance = terminalStops;
        if (provenance != null) provenance.terminal(record, reason, failure, retained);
    }

    void bindTerminalStop(Exception failure, java.util.List<? extends org.apache.kafka.clients.consumer.ConsumerRecord<?, ?>> records,
                          org.springframework.kafka.listener.MessageListenerContainer container) {
        var provenance = terminalStops;
        if (provenance != null) provenance.bind(failure, records, container);
    }

    StopRequest captureStopRequest() {
        var provenance = terminalStops;
        return provenance == null ? null : provenance.request();
    }

    void runStopRequest(StopRequest request, Runnable stop) {
        if (request == null) {
            if (terminalStops == null) stop.run();
            return;
        }
        executingStop.set(request);
        try {
            if (request.provenance().parent instanceof IngestionKafkaConfiguration.ProvenanceListenerContainer container) {
                container.runIfCurrent(request, stop);
            } else {
                // Lifecycle fixtures without a real container lock use the same epoch check.
                request.provenance().runIfCurrent(request, stop);
            }
        } finally {
            executingStop.remove();
        }
    }

    TerminalStop getConsumerTerminalStop() {
        var provenance = terminalStops;
        return provenance == null ? null : provenance.snapshot();
    }

    /**
     * A single registered listener, bounded by its configured child concurrency, with no
     * partition/offset history. Invoked at publication, before the application multicaster.
     * All state transitions are serialized; no container lifecycle locks are taken here.
     */
    final class TerminalStopProvenance {
        private final org.springframework.kafka.listener.ConcurrentMessageListenerContainer<?, ?> parent;
        private int expectedChildren;
        private final java.util.Map<Object, ChildStop> children = new java.util.IdentityHashMap<>();
        private long epoch;
        private boolean ineligible;
        private boolean parentStopObserved;
        private boolean contextClosed;
        private boolean generationEstablished;
        private TerminalFailure firstTerminal;

        private final class ChildStop {
            private Thread thread;
            private boolean started;
            private TerminalFailure terminal;
            private RuntimeException exception;
            private boolean bound;
            private boolean stopped;
            private boolean ineligible;
        }

        TerminalStopProvenance(org.springframework.kafka.listener.ConcurrentMessageListenerContainer<?, ?> parent) {
            this.parent = parent;
        }

        synchronized void beginGeneration() {
            epoch = terminalStopEpoch.updateAndGet(value -> Math.incrementExact(value));
            expectedChildren = parent.getConcurrency();
            children.clear();
            firstTerminal = null;
            ineligible = false;
            parentStopObserved = false;
            generationEstablished = false;
            // Deliberately retain consumerFailures: only persistence plus ACK clears them.
        }

        synchronized org.springframework.context.ApplicationEventPublisher beginGenerationPublisher(
                org.springframework.context.ApplicationEventPublisher downstream) {
            beginGeneration();
            long generation = epoch;
            return event -> {
                onGenerationEvent(generation, event);
                if (downstream != null) downstream.publishEvent(event);
            };
        }

        private synchronized void onGenerationEvent(long generation, Object event) {
            // Children inherit this immutable stamp before their consumer threads start.
            // Reject retired publishers BEFORE provisional child capacity is considered.
            if (terminalStops == this && generation == epoch) onEvent(event);
        }

        synchronized void establishChildren(java.util.List<? extends org.springframework.kafka.listener.MessageListenerContainer> actual) {
            if (actual.size() > expectedChildren) {
                ineligible = true;
                return;
            }
            var members = java.util.Collections.newSetFromMap(new java.util.IdentityHashMap<Object, Boolean>());
            members.addAll(actual);
            children.keySet().removeIf(source -> !members.contains(source));
            for (var source : actual) children.computeIfAbsent(source, ignored -> new ChildStop());
            if (children.values().stream().noneMatch(child -> child.terminal == firstTerminal)) {
                firstTerminal = children.values().stream().map(child -> child.terminal)
                        .filter(java.util.Objects::nonNull).findFirst().orElse(null);
            }
            generationEstablished = actual.size() == expectedChildren;
            if (!generationEstablished) ineligible = true;
        }

        synchronized boolean isCurrent(StopRequest request) {
            return terminalStops == this && request.provenance() == this && epoch > 0
                    && request.epoch() == epoch && generationEstablished && !contextClosed
                    && children.values().stream().anyMatch(child -> child == request.child());
        }

        synchronized void runIfCurrent(StopRequest request, Runnable stop) {
            if (isCurrent(request)) stop.run();
        }

        synchronized void parentStopping(boolean normal) {
            parentStopObserved = true; // Initiation only; all current child completions still required.
            var request = executingStop.get();
            if (normal || request == null || !isCurrent(request)) ineligible = true;
        }

        synchronized void onEvent(Object published) {
            if (!(published instanceof org.springframework.kafka.event.KafkaEvent event)
                    || event.getContainer(Object.class) != parent) return;
            Object source = event.getSource();
            if (!(source instanceof org.springframework.kafka.listener.MessageListenerContainer)) {
                ineligible = true;
                return;
            }
            // Once actual membership is established, a retired/nonmember source has
            // no authority to mutate this generation, including vetoing its evidence.
            if (generationEstablished && source != parent && !children.containsKey(source)) return;
            if (event instanceof org.springframework.kafka.event.ConsumerStartingEvent) {
                var child = children.get(source);
                if (child != null && child.started) {
                    child.ineligible = true;
                    return;
                }
                if (epoch == 0 || source == parent
                        || (child == null && (generationEstablished || children.size() >= expectedChildren))) {
                    ineligible = true;
                    return;
                }
                if (child == null) {
                    child = new ChildStop();
                    children.put(source, child);
                }
                child.started = true;
                child.thread = Thread.currentThread();
            } else if (event instanceof org.springframework.kafka.event.ConsumerStoppedEvent stopped) {
                var child = children.get(source);
                // Without a matching start this source cannot prove a current completion.
                if (child == null) return;
                if (child.stopped || child.thread != Thread.currentThread()) {
                    child.ineligible = true;
                    return;
                }
                // In 3.2.4 an application error-handler stop reports NORMAL, not ABNORMAL.
                if (stopped.getReason() != org.springframework.kafka.event.ConsumerStoppedEvent.Reason.NORMAL) {
                    child.ineligible = true;
                }
                child.stopped = true;
                child.thread = null;
                child.exception = null;
            }
            // ContainerStoppedEvent has no generation/completion proof. The actual
            // parent doStop hook records initiation and operator/typed-stop provenance.
        }

        private ChildStop currentChild() {
            ChildStop found = null;
            for (var child : children.values()) {
                if (!child.stopped && child.thread == Thread.currentThread()) {
                    if (found != null) return null;
                    found = child;
                }
            }
            return found;
        }

        synchronized void terminal(org.apache.kafka.clients.consumer.ConsumerRecord<?, ?> record,
                                   TelemetryConsumerFailure reason, RuntimeException failure, InsertCompletion completion) {
            var child = currentChild();
            if (child == null) return;
            if (child.terminal != null) {
                child.ineligible = true;
                return;
            }
            var source = children.entrySet().stream().filter(entry -> entry.getValue() == child)
                    .map(java.util.Map.Entry::getKey).findFirst().orElseThrow();
            child.terminal = new TerminalFailure(parent.getListenerId(),
                    ((org.springframework.kafka.listener.MessageListenerContainer) source).getListenerId(),
                    record.topic(), record.partition(), record.offset(), reason, epoch, completion);
            if (firstTerminal == null) firstTerminal = child.terminal;
            child.exception = failure; // sanitized exception identity, discarded on bind/stop
        }

        synchronized void bind(Exception failure,
                               java.util.List<? extends org.apache.kafka.clients.consumer.ConsumerRecord<?, ?>> records,
                               org.springframework.kafka.listener.MessageListenerContainer container) {
            var child = currentChild();
            if (child == null) return;
            if (container != parent || child.terminal == null || child.bound || records.isEmpty()) {
                child.ineligible = true;
                return;
            }
            var record = records.get(0);
            var terminal = child.terminal;
            boolean matches = terminal.topic().equals(record.topic()) && terminal.partition() == record.partition()
                    && terminal.offset() == record.offset();
            boolean sameException = false;
            Throwable cause = failure;
            for (int depth = 0; cause != null && depth < 8; depth++, cause = cause.getCause()) {
                if (cause == child.exception) sameException = true;
            }
            child.exception = null;
            if (!matches || !sameException) child.ineligible = true;
            else child.bound = true;
        }

        synchronized StopRequest request() {
            var child = currentChild();
            if (child == null) {
                return null; // An old/unattributed consumer cannot mint a stop for this generation.
            }
            return new StopRequest(this, epoch, child);
        }

        synchronized void shutdown() {
            contextClosed = true;
            ineligible = true;
        }

        private boolean complete() {
            return generationEstablished && parentStopObserved && children.size() == expectedChildren
                    && children.values().stream().allMatch(child -> child.started && child.stopped);
        }

        synchronized boolean isComplete() {
            return children.isEmpty() || complete();
        }

        synchronized TerminalStop snapshot() {
            TerminalFailure terminal = firstTerminal;
            boolean bound = false;
            int count = 0;
            for (var child : children.values()) {
                if (child.terminal != null) {
                    if (terminal == child.terminal) bound = child.bound;
                    count++;
                }
            }
            if (terminal == null) return null;
            boolean completed = complete();
            boolean eligible = completed && !ineligible && !contextClosed && count == 1 && bound
                    && children.values().stream().noneMatch(child -> child.ineligible)
                    && terminal.reason() == TelemetryConsumerFailure.DEPENDENCY;
            return new TerminalStop(terminal, !generationEstablished ? "GENERATION_UNCERTAIN"
                    : eligible ? "RECOVERABLE_DEPENDENCY_STOP"
                    : completed ? "INELIGIBLE_STOP" : "TERMINAL_FAILURE", completed);
        }
    }

    public void recordBatchReceived() {
        batchesReceived.incrementAndGet();
    }

    public void recordBatchRejected(String errorCode) {
        batchesRejected.incrementAndGet();
        lastError.set(errorCode);
    }

    public void recordBatchReplayed() {
        batchesReplayed.incrementAndGet();
    }

    public void recordDuplicateReplay() {
        recordBatchReplayed();
    }

    public void recordKafkaPublish(int count, long durationMs) {
        recordsAccepted.addAndGet(count);
        kafkaPublishes.incrementAndGet();
        kafkaPublishTotalMs.addAndGet(durationMs);
        lastPersistenceTimestamp.set(Instant.now());
    }

    public void recordKafkaFailure(String errorCode) {
        kafkaFailures.incrementAndGet();
        lastError.set(stableOrEmpty(errorCode));
    }

    public void recordClickHouseInsert(long count, long durationMs) {
        clickHouseInserts.addAndGet(count);
        clickHouseInsertTotalMs.addAndGet(durationMs);
        lastPersistenceTimestamp.set(Instant.now());
    }

    public void recordClickHouseFailure(String errorCode) {
        clickHouseFailures.incrementAndGet();
        lastError.set(stableOrEmpty(errorCode));
    }

    public void recordConsumerPoisonEvent(String topic, long offset, String error) {
        consumerPoisonEvents.incrementAndGet();
        consumerDegraded.set(true);
        consumerDegradedReason.set(com.platform.core.logging.CoreSupportabilityEventCodes.TELEMETRY_POISON_EVENT);
        lastError.set(com.platform.core.logging.CoreSupportabilityEventCodes.TELEMETRY_POISON_EVENT);
    }

    public void recordConsumerDegraded(String reasonCode) {
        consumerDegraded.set(true);
        consumerDegradedReason.set(stableOrEmpty(reasonCode));
    }

    public void recordConsumerHealthy() {
        consumerDegraded.set(false);
        consumerDegradedReason.set("");
    }

    synchronized void recordConsumerFailure(org.apache.kafka.clients.consumer.ConsumerRecord<?, ?> record,
                                            TelemetryConsumerFailure reason) {
        recordConsumerFailure(record, reason, InsertCompletion.UNKNOWN);
    }

    synchronized InsertCompletion consumerInsertCompletion(org.apache.kafka.clients.consumer.ConsumerRecord<?, ?> record) {
        var blocked = consumerFailures.get(new org.apache.kafka.common.TopicPartition(record.topic(), record.partition()));
        return blocked != null && blocked.offset() == record.offset() ? blocked.completion() : InsertCompletion.UNKNOWN;
    }

    synchronized InsertCompletion recordConsumerFailure(org.apache.kafka.clients.consumer.ConsumerRecord<?, ?> record,
                                                        TelemetryConsumerFailure reason, InsertCompletion completion) {
        var key = new org.apache.kafka.common.TopicPartition(record.topic(), record.partition());
        var blocked = consumerFailures.get(key);
        if (blocked == null || blocked.offset() == record.offset()) {
            if (blocked != null) completion = blocked.completion().merge(completion);
            consumerFailures.put(key, new ConsumerFailure(record.offset(), reason, completion));
        }
        // A different offset must neither replace the retained slot nor inherit its completion.
        lastError.set(reason.reasonCode);
        return completion;
    }

    synchronized void recordConsumerHealthy(org.apache.kafka.clients.consumer.ConsumerRecord<?, ?> record) {
        // Legacy notification carries no persistence/ACK proof.
        recordConsumerHealthy(record, false, false);
    }

    synchronized void recordConsumerHealthy(org.apache.kafka.clients.consumer.ConsumerRecord<?, ?> record,
                                            boolean insertCompleted, boolean acknowledged) {
        if (!insertCompleted || !acknowledged) return;
        var key = new org.apache.kafka.common.TopicPartition(record.topic(), record.partition());
        var blocked = consumerFailures.get(key);
        // Only successful persistence AND ACK of the exact retained record proves recovery.
        if (blocked != null && blocked.offset() == record.offset()) consumerFailures.remove(key);
        if (consumerFailures.isEmpty()) recordConsumerHealthy();
    }

    public void recordPreviousBatchPending() { previousBatchPending.incrementAndGet(); }
    public void recordReplayConflict() { replayConflicts.incrementAndGet(); }
    public void recordSequenceGap() { sequenceGaps.incrementAndGet(); }
    public void recordSequenceRegression() { sequenceRegressions.incrementAndGet(); }
    public void recordStreamMismatch() { streamMismatches.incrementAndGet(); }
    public void recordPublisherLeaseContention() { publisherLeaseContentions.incrementAndGet(); }
    public void recordFinalizeFailure(String errorCode) {
        finalizeFailures.incrementAndGet();
        lastError.set(stableOrEmpty(errorCode));
    }
    public void recordPendingReservationObserved() { pendingReservationsObserved.incrementAndGet(); }

    public long getBatchesReceived() { return batchesReceived.get(); }
    public long getBatchesRejected() { return batchesRejected.get(); }
    public long getBatchesReplayed() { return batchesReplayed.get(); }
    public long getRecordsAccepted() { return recordsAccepted.get(); }

    public long getKafkaPublishes() { return kafkaPublishes.get(); }
    public long getKafkaPublishTotalMs() { return kafkaPublishTotalMs.get(); }
    public long getKafkaFailures() { return kafkaFailures.get(); }

    public long getClickHouseInserts() { return clickHouseInserts.get(); }
    public long getClickHouseInsertTotalMs() { return clickHouseInsertTotalMs.get(); }
    public long getClickHouseFailures() { return clickHouseFailures.get(); }

    public long getConsumerPoisonEvents() { return consumerPoisonEvents.get(); }
    public synchronized boolean isConsumerDegraded() { return !consumerFailures.isEmpty() || consumerDegraded.get(); }
    public synchronized String getConsumerDegradedReason() {
        return consumerFailures.isEmpty() ? consumerDegradedReason.get()
                : consumerFailures.values().iterator().next().reason().reasonCode;
    }

    public long getPreviousBatchPending() { return previousBatchPending.get(); }
    public long getReplayConflicts() { return replayConflicts.get(); }
    public long getSequenceGaps() { return sequenceGaps.get(); }
    public long getSequenceRegressions() { return sequenceRegressions.get(); }
    public long getStreamMismatches() { return streamMismatches.get(); }
    public long getPublisherLeaseContentions() { return publisherLeaseContentions.get(); }
    public long getFinalizeFailures() { return finalizeFailures.get(); }
    public long getPendingReservationsObserved() { return pendingReservationsObserved.get(); }

    public Instant getLastPersistenceTimestamp() { return lastPersistenceTimestamp.get(); }
    public String getLastError() { return lastError.get(); }

    private static String stableOrEmpty(String code) {
        return code == null ? "" : code;
    }
}
