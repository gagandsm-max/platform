/**
 * Issue #218 Slice A — internal versioned ICMP typed observation contract.
 *
 * <p><b>NOT_WIRED:</b> These types decode and validate BUILD-020 / PR #217
 * {@code icmp_observation_v1} envelopes for contract tests only. They are not
 * consulted by {@code IngestionValidator}, Sensor ingestion controllers,
 * Kafka producers/consumers, or durable ACK paths. Unsupported ICMP production
 * traffic continues to fail closed before durable ACK.
 */
package com.platform.core.ingestion.icmp;
