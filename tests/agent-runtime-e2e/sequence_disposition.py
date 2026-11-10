#!/usr/bin/env python3
"""Real isolated Agent→Sensor→Core sequence-disposition regression matrix."""

import hashlib
import json
import os
from pathlib import Path
import subprocess
import sys
import time
from datetime import datetime, timezone
import secrets
import uuid

from build_envelope import build_envelope

TENANT = "018f0e0a-0000-7000-8000-000000000001"
OTHER_TENANT = "018f0e0a-0000-7000-8000-000000000099"
AGENT = "agent-e2e-01"
SENSOR = os.environ.get("SENSOR_INFRA_ID", "sensor-infra-01")
GOOD_HOST = "host-e2e-agent-01"
CONFLICT_HOST = "host-e2e-probe-01"
ENTITY_A = "018f0e0a-0000-7000-8000-00000000ae01"
ENTITY_B = "018f0e0a-0000-7000-8000-00000000ae03"


def fresh_uuid7():
    milliseconds = int(time.time() * 1000)
    bits = ((milliseconds << 80) | (7 << 76) | (secrets.randbits(12) << 64)
            | (2 << 62) | secrets.randbits(62))
    return str(uuid.UUID(int=bits))


class Matrix:
    def __init__(self, prefix, port, evidence):
        self.prefix, self.port, self.evidence = prefix, port, Path(evidence)
        self.ca = Path(os.environ["PLATFORM_E2E_CA_DIR"]) / "ca.crt"
        self.cert = Path(os.environ["PLATFORM_E2E_CERT_DIR"]) / "agent-e2e.crt"
        self.key = self.cert.with_suffix(".key")
        self.compose = ["docker", "compose", "-p", prefix, "-f", os.environ["PLATFORM_E2E_ISOLATED_CONFIG"]]
        self.results = {}

    def call(self, args, body=None, timeout=45):
        result = subprocess.run(args, input=body, text=True, capture_output=True, timeout=timeout)
        if result.returncode:
            raise AssertionError(f"command failed {args[:5]} exit={result.returncode}: {result.stderr[-1000:]}")
        return result.stdout

    def sql(self, query):
        return self.call(["docker", "exec", self.prefix + "-postgres", "psql", "-U", "platform_app",
                          "-d", "platform", "-At", "-v", "ON_ERROR_STOP=1", "-c", query]).strip()

    def watermark(self):
        rows = self.sql("SELECT row_to_json(t) FROM (SELECT stream_id, acked_through_sequence, "
                        "settled_through_sequence, pending_first_sequence, pending_last_sequence, "
                        "pending_checksum, pending_request_sha256, pending_batch_id, "
                        "pending_correlation_id, pending_reservation_id, pending_publisher_owner, "
                        "pending_publisher_epoch, pending_publisher_claimed_at, "
                        "pending_publisher_lease_until, pending_publication_attempted, "
                        "pending_authority_state, pending_authority_validated_at, "
                        "pending_canonical_batch_sha256, last_batch_id "
                        "FROM platform.sensor_agent_ingress_watermark WHERE tenant_id='" + TENANT +
                        "' AND sensor_id='" + SENSOR + "') t")
        return json.loads(rows) if rows else None

    def terminal(self, seq=3):
        rows = self.sql("SELECT row_to_json(t) FROM (SELECT disposition_id, stream_id, "
                        "first_sequence, last_sequence, checksum, batch_id, correlation_id, "
                        "reason_code, sensor_ingress_record_id, agent_id, assignment_generation "
                        "FROM platform.sensor_agent_ingress_terminal_disposition WHERE tenant_id='" + TENANT +
                        "' AND sensor_id='" + SENSOR + "' AND first_sequence=" + str(seq) + ") t")
        return json.loads(rows) if rows else None

    def bindings(self, reservation_id):
        rows = self.sql("SELECT coalesce(json_agg(row_to_json(t) ORDER BY sensor_ingress_sequence), '[]') "
                        "FROM (SELECT sensor_ingress_sequence, agent_id, assignment_generation, "
                        "host_id, entity_identifier_id, entity_id, binding_version "
                        "FROM platform.sensor_agent_ingress_pending_canonical_binding WHERE tenant_id='" + TENANT +
                        "' AND sensor_id='" + SENSOR + "' AND reservation_id='" + reservation_id +
                        "' ORDER BY sensor_ingress_sequence) t")
        return json.loads(rows)

    def wait_watermark(self, acked, settled, seconds=90):
        until = time.monotonic() + seconds
        last = None
        while time.monotonic() < until:
            last = self.watermark()
            if last and last["acked_through_sequence"] == acked and last["settled_through_sequence"] == settled:
                return last
            time.sleep(1)
        raise AssertionError(f"watermark not ({acked},{settled}); last={last}")

    def wait_healthy(self, service, seconds=90):
        until = time.monotonic() + seconds
        while time.monotonic() < until:
            state = self.call(["docker", "inspect", "--format", "{{.State.Health.Status}}",
                               self.prefix + "-" + service]).strip()
            if state == "healthy":
                return
            time.sleep(2)
        raise AssertionError(f"{service} did not become healthy after restart")

    def http(self, args, body):
        result = self.call(args + ["-w", "\n%{http_code}", "--data-binary", "@-"], body)
        raw, code = result.rsplit("\n", 1)
        return int(code), json.loads(raw)

    def sensor(self, body):
        return self.http(["curl", "-sS", "--connect-timeout", "5", "--max-time", "30",
                          "--cacert", str(self.ca), "--cert", str(self.cert), "--key", str(self.key),
                          "-H", "Content-Type: application/json", "-X", "POST",
                          "https://127.0.0.1:" + self.port + "/api/v1/agent/telemetry"], body)

    def core(self, batch):
        body = json.dumps(batch, separators=(",", ":"))
        return self.http(["docker", "exec", "-i", self.prefix + "-sensor-infra", "curl", "-sS",
                          "--connect-timeout", "5", "--max-time", "30", "--cacert", "/certs/ca.crt",
                          "--cert", "/certs/sensor-infra.crt", "--key", "/certs/sensor-infra.key",
                          "-H", "Content-Type: application/json", "-H", "Idempotency-Key: " + batch["batchId"],
                          "https://core:8443/api/v1/sensor/agent-telemetry"], body)

    def envelope(self, seq, host, metric, tenant=TENANT):
        return json.loads(build_envelope(envelope_tenant=tenant, payload_tenant=tenant,
                          agent_id=AGENT, host_id=host, sequence=seq, metric_key=metric,
                          value=float(seq), batch_id_value=None, correlation_id=f"p0-corr-{seq}"))

    def submit(self, seq, host, metric):
        env = self.envelope(seq, host, metric)
        code, response = self.sensor(json.dumps(env, separators=(",", ":")))
        assert code == 200 and response["status"] == "ACCEPTED", (code, response)
        self.write(f"sensor-{seq}.json", {"request": env, "response": response})
        return env

    def write(self, name, value):
        (self.evidence / name).write_text(json.dumps(value, indent=2, sort_keys=True) + "\n")

    def mark(self, label, detail):
        self.results[label] = {"status": "PASS", "detail": detail}
        self.write("sequence-matrix.json", self.results)
        print("[PASS]", label, detail, flush=True)

    def poison(self):
        return self.call(["docker", "exec", self.prefix + "-sensor-infra", "sh", "-c",
                          "cat /var/lib/platform/sensor/agent-spool/agent-poison/00000000000000000003-ERR_AGENT_IDENTITY_CONFLICT.json"])

    def terminal_batch(self, terminal, payload):
        env = payload["envelope"]
        record = {"sensorIngressSequence": 3, "sensorIngressRecordId": terminal["sensor_ingress_record_id"],
                  "tenantId": payload["tenantId"], "agentId": payload["agentId"],
                  "acceptingSensorId": payload["sensorId"],
                  "assignmentGeneration": payload["assignmentGeneration"],
                  "agentFirstSequence": payload["firstSequence"], "agentLastSequence": payload["lastSequence"],
                  "agentBatchId": payload["batchId"], "correlationId": payload["correlationId"],
                  "acceptedAt": payload["timestamp"],
                  "acceptanceEvidence": {**payload["acceptanceEvidence"], "durableSpoolSequence": 3},
                  "records": env["records"]}
        return {"schemaVersion": 2, "sensorId": SENSOR, "sensorAgentSpoolStreamId": terminal["stream_id"],
                "batchId": terminal["batch_id"], "firstIngressSequence": 3, "lastIngressSequence": 3,
                "recordCount": 1, "checksum": terminal["checksum"],
                "correlationId": terminal["correlation_id"], "records": [record]}

    def direct_batch(self, seq, stream, metric, generation=1, tenant=TENANT,
                     accepted_at="2026-11-09T00:00:00Z", qualified=False):
        env = self.envelope(seq, GOOD_HOST, metric)
        record = {"sensorIngressSequence": seq, "sensorIngressRecordId": f"direct-p0-{seq}",
                  "tenantId": tenant, "agentId": AGENT, "acceptingSensorId": SENSOR,
                  "assignmentGeneration": generation, "agentFirstSequence": seq,
                  "agentLastSequence": seq, "agentBatchId": env["batchId"],
                  "correlationId": env["correlationId"], "acceptedAt": accepted_at,
                  "records": env["records"]}
        if qualified:
            record["acceptanceEvidence"] = {
                "evidenceVersion": 1, "acceptedAtUtc": accepted_at, "clockQuality": "QUALIFIED",
                "clockMaxErrorMs": 0, "clockSyncAgeMs": 0, "clockSource": "TEST_QUALIFIED",
                "sensorServiceInstanceId": "runtime-matrix-instance", "durableSpoolSequence": seq}
        delivery = env["records"][0]
        line = "|".join(map(str, [seq, record["agentBatchId"], seq, seq, generation,
                                  tenant, AGENT, SENSOR, seq, delivery["id"], delivery["checksum"]]))
        checksum = hashlib.sha256(line.encode()).hexdigest()
        bid = f"agent-ingress-{SENSOR}-{stream}-{seq:020d}-{seq:020d}-{checksum[:16]}"
        return {"schemaVersion": 2, "sensorId": SENSOR, "sensorAgentSpoolStreamId": stream,
                "batchId": bid, "firstIngressSequence": seq, "lastIngressSequence": seq,
                "recordCount": 1, "checksum": checksum, "correlationId": "agentdrain-" + bid,
                "records": [record]}

    def crash_pending(self, batch, settled):
        body = json.dumps(batch, separators=(",", ":"))
        digest = hashlib.sha256(body.encode()).hexdigest()
        sequence = batch["firstIngressSequence"]
        changed = self.sql("UPDATE platform.sensor_agent_ingress_watermark SET "
                           "pending_first_sequence=" + str(sequence) + ", pending_last_sequence=" + str(sequence) + ", "
                           "pending_checksum='" + batch["checksum"] + "', "
                           "pending_request_sha256='" + digest + "', "
                           "pending_batch_id='" + batch["batchId"] + "', "
                           "pending_correlation_id='" + batch["correlationId"] + "', "
                           "pending_reserved_at=clock_timestamp(), pending_reservation_id=gen_random_uuid(), "
                           "pending_authority_state='UNVALIDATED', pending_authority_validated_at=NULL, "
                           "pending_canonical_batch_sha256=NULL, pending_publication_attempted=FALSE, "
                           "pending_publisher_owner=NULL, pending_publisher_claimed_at=NULL, "
                           "pending_publisher_lease_until=NULL "
                           "WHERE tenant_id='" + TENANT + "' AND sensor_id='" + SENSOR + "' "
                           "AND settled_through_sequence=" + str(settled) + " AND pending_first_sequence IS NULL "
                           "RETURNING pending_reservation_id")
        assert changed, (sequence, self.watermark())
        pending = self.watermark()
        assert pending["pending_authority_state"] == "UNVALIDATED"
        assert pending["pending_request_sha256"] == digest
        return pending

    def kafka_metric(self, metric):
        lines = []
        for partition in range(3):
            cmd = ["docker", "exec", self.prefix + "-kafka", "/opt/kafka/bin/kafka-console-consumer.sh",
                   "--bootstrap-server", "localhost:9092", "--topic", "telemetry.numeric.v1",
                   "--partition", str(partition), "--from-beginning", "--timeout-ms", "3000"]
            proc = subprocess.run(cmd, text=True, capture_output=True, timeout=20)
            assert proc.returncode in (0, 1), proc.stderr[-500:]
            lines.extend(proc.stdout.splitlines())
        hits = [json.loads(line) for line in lines if line.startswith("{") and
                json.loads(line).get("metricId") == metric]
        return hits

    def clickhouse_count(self, metric):
        query = ("SELECT count() FROM platform.metric_numeric WHERE tenant_id='" + TENANT +
                 "' AND metric_id='" + metric + "'")
        return int(self.call(["docker", "exec", self.prefix + "-clickhouse", "sh", "-c",
                              'exec clickhouse-client --user "$CLICKHOUSE_USER" --password "$CLICKHOUSE_PASSWORD" --query "$1"',
                              "sh", query]).strip())

    def clickhouse_value(self, metric):
        query = ("SELECT value FROM platform.metric_numeric WHERE tenant_id='" + TENANT +
                 "' AND metric_id='" + metric + "' FORMAT TSV")
        return float(self.call(["docker", "exec", self.prefix + "-clickhouse", "sh", "-c",
                                'exec clickhouse-client --user "$CLICKHOUSE_USER" --password "$CLICKHOUSE_PASSWORD" --query "$1"',
                                "sh", query]).strip())

    def run(self):
        # Every Sensor send traverses real mTLS receiver, durable spool and drain.
        self.submit(1, GOOD_HOST, "p0.sequence.accepted.1")
        self.wait_watermark(1, 1)
        self.submit(2, GOOD_HOST, "p0.sequence.accepted.2")
        before = self.wait_watermark(2, 2)
        self.mark("A", before)
        self.mark("M", {"acked": 2, "settled": 2})

        self.submit(3, CONFLICT_HOST, "p0.sequence.rejected.3")
        after = self.wait_watermark(2, 3)
        terminal = self.terminal()
        assert terminal and terminal["reason_code"] == "ERR_AGENT_IDENTITY_CONFLICT"
        assert terminal["checksum"] and terminal["disposition_id"]
        raw_poison = self.poison()
        meta, raw_payload = raw_poison.split("\n", 1)
        marker = json.loads(meta)
        payload = json.loads(raw_payload)
        assert marker["coreDispositionId"] == terminal["disposition_id"]
        assert marker["replayPolicy"] == "AUTHORITATIVE_RECONCILIATION_REQUIRED"
        assert hashlib.sha256(raw_payload.encode()).hexdigest() == marker["payloadSHA256"]
        self.write("terminal-db.json", terminal)
        self.write("terminal-quarantine-metadata.json", marker)
        self.mark("B", {"watermark": after, "terminal": terminal, "quarantine": marker})
        self.mark("N", {"acked": 2, "settled": 3})

        prior = self.terminal_batch(terminal, payload)
        code, response = self.core(prior)
        assert code == 200 and response["status"] == "TERMINALLY_REJECTED"
        assert response["dispositionId"] == terminal["disposition_id"]
        self.mark("C", response)

        same_terminal_checksum = json.loads(json.dumps(prior))
        same_terminal_checksum["records"][0]["records"][0]["payload"]["observations"][0]["value"] = 999
        code, altered_body = self.core(same_terminal_checksum)
        assert code == 409 and altered_body["code"] == "ERR_REPLAY_CONFLICT", (code, altered_body)
        self.mark("C-altered-body", altered_body)

        code, changed = self.core({**prior, "checksum": "f" * 64})
        assert code == 400 and changed["code"] == "ERR_CHECKSUM_MISMATCH", (code, changed)
        # A well-formed altered checksum must be bound to altered delivery identity.
        altered = json.loads(json.dumps(prior))
        altered["records"][0]["records"][0]["checksum"] = "f" * 64
        fields = altered["records"][0]
        line = "|".join(map(str, [3, fields["agentBatchId"], fields["agentFirstSequence"],
                                  fields["agentLastSequence"], fields["assignmentGeneration"],
                                  fields["tenantId"], fields["agentId"], fields["acceptingSensorId"],
                                  3, fields["records"][0]["id"], "f" * 64]))
        altered["checksum"] = hashlib.sha256(line.encode()).hexdigest()
        code, conflict = self.core(altered)
        assert code == 409 and conflict["code"] == "ERR_REPLAY_CONFLICT", (code, conflict)
        self.mark("E-terminal", conflict)

        self.call(self.compose + ["restart", "sensor-infra"], timeout=90)
        self.wait_healthy("sensor-infra")
        endpoint = self.call(self.compose + ["port", "sensor-infra", "8444"]).strip()
        assert endpoint.startswith("127.0.0.1:"), endpoint
        self.port = endpoint.rsplit(":", 1)[1]
        assert "RECONCILIATION_REQUIRED" in self.poison()
        self.mark("F", "quarantine artifact and durable marker survived Sensor restart")
        self.call(self.compose + ["restart", "core"], timeout=90)
        self.wait_healthy("core")
        code, replayed = self.core(prior)
        assert code == 200 and replayed["dispositionId"] == terminal["disposition_id"]
        self.mark("G", replayed)

        self.submit(4, GOOD_HOST, "p0.sequence.accepted.4")
        after_next = self.wait_watermark(2, 4)
        assert after_next["pending_first_sequence"] is None
        self.mark("H", after_next)

        # Direct Core send deliberately discards the first application response.
        stream = after_next["stream_id"]
        fifth = self.direct_batch(5, stream, "p0.sequence.accepted.5")
        code, first = self.core(fifth)
        assert code == 200 and first["status"] == "ACCEPTED", (code, first)
        self.wait_watermark(2, 5)
        first_kafka = self.kafka_metric("p0.sequence.accepted.5")
        assert len(first_kafka) == 1, first_kafka
        code, repeated = self.core(fifth)
        assert code == 200 and repeated == first, (code, repeated, first)
        second_kafka = self.kafka_metric("p0.sequence.accepted.5")
        assert len(second_kafka) == 1, second_kafka
        until = time.monotonic() + 30
        while time.monotonic() < until and self.clickhouse_count("p0.sequence.accepted.5") != 1:
            time.sleep(1)
        assert self.clickhouse_count("p0.sequence.accepted.5") == 1
        self.write("kafka-accepted-replay.json", second_kafka)
        self.mark("D", {"first": first, "retry": repeated, "kafka_hits": 1, "clickhouse_rows": 1})

        same_accepted_checksum = json.loads(json.dumps(fifth))
        same_accepted_checksum["records"][0]["records"][0]["payload"]["observations"][0]["value"] = 999
        code, altered_body = self.core(same_accepted_checksum)
        assert code == 409 and altered_body["code"] == "ERR_REPLAY_CONFLICT", (code, altered_body)
        self.mark("D-altered-body", altered_body)

        changed_fifth = json.loads(json.dumps(fifth))
        changed_fifth["records"][0]["records"][0]["checksum"] = "e" * 64
        rec = changed_fifth["records"][0]
        line = "|".join(map(str, [5, rec["agentBatchId"], 5, 5, 1, TENANT, AGENT, SENSOR,
                                  5, rec["records"][0]["id"], "e" * 64]))
        changed_fifth["checksum"] = hashlib.sha256(line.encode()).hexdigest()
        code, conflict = self.core(changed_fifth)
        assert code == 409 and conflict["code"] == "ERR_REPLAY_CONFLICT", (code, conflict)
        self.mark("E-accepted", conflict)

        # Fixture emulates a V019 row whose last_batch_id was absent at migration.
        self.sql("UPDATE platform.sensor_agent_ingress_watermark SET last_batch_id=NULL WHERE tenant_id='" +
                 TENANT + "' AND sensor_id='" + SENSOR + "'")
        code, legacy = self.core(fifth)
        assert code == 200 and legacy["status"] == "ACCEPTED", (code, legacy)
        self.mark("L", legacy)

        before_negative = self.watermark()
        cross_tenant = self.direct_batch(6, stream, "p0.sequence.cross-tenant.6", tenant=OTHER_TENANT)
        code, denied = self.core(cross_tenant)
        assert code == 403 and denied["code"] == "ERR_TENANT_SCOPE_MISMATCH", (code, denied)
        assert self.watermark() == before_negative and self.terminal(6) is None
        self.mark("I", denied)

        stale = self.direct_batch(6, stream, "p0.sequence.stale.6", generation=999)
        code, denied = self.core(stale)
        assert code == 403 and denied["code"] == "ERR_AGENT_ASSIGNMENT_UNKNOWN", (code, denied)
        after_stale = self.watermark()
        assert after_stale["acked_through_sequence"] == before_negative["acked_through_sequence"]
        assert after_stale["settled_through_sequence"] == before_negative["settled_through_sequence"]
        assert after_stale["pending_first_sequence"] is None
        assert after_stale["pending_request_sha256"] is None
        assert after_stale["pending_reservation_id"] is None
        assert after_stale["pending_publisher_owner"] is None
        assert after_stale["pending_publisher_lease_until"] is None
        assert after_stale["pending_publication_attempted"] is False
        assert self.terminal(6) is None
        assert not self.kafka_metric("p0.sequence.stale.6")
        assert self.clickhouse_count("p0.sequence.stale.6") == 0
        self.mark("J", {"response": denied, "watermark": after_stale,
                        "terminal": None, "kafka_hits": 0, "clickhouse_rows": 0})

        code, denied_again = self.core(stale)
        after_repeat = self.watermark()
        assert code == 403 and denied_again["code"] == "ERR_AGENT_ASSIGNMENT_UNKNOWN", (code, denied_again)
        assert after_repeat == after_stale and self.terminal(6) is None
        assert not self.kafka_metric("p0.sequence.stale.6")
        assert self.clickhouse_count("p0.sequence.stale.6") == 0
        self.mark("J3", {"response": denied_again, "watermark": after_repeat,
                         "terminal": None, "kafka_hits": 0, "clickhouse_rows": 0})

        corrected = self.direct_batch(6, stream, "p0.sequence.corrected.6", generation=1)
        code, accepted = self.core(corrected)
        assert code == 200 and accepted["status"] == "ACCEPTED", (code, accepted)
        after_corrected = self.wait_watermark(2, 6)
        assert after_corrected["pending_first_sequence"] is None
        assert self.terminal(6) is None
        corrected_hits = self.kafka_metric("p0.sequence.corrected.6")
        assert len(corrected_hits) == 1
        until = time.monotonic() + 30
        while time.monotonic() < until and self.clickhouse_count("p0.sequence.corrected.6") != 1:
            time.sleep(1)
        assert self.clickhouse_count("p0.sequence.corrected.6") == 1
        self.mark("J2", {"response": accepted, "watermark": after_corrected,
                         "terminal": None, "kafka_hits": 1, "clickhouse_rows": 1})

        # Failure injection targets only the exact pre-publication pending clear.
        self.sql("CREATE FUNCTION platform.test_pending_release_failure() RETURNS trigger "
                 "LANGUAGE plpgsql AS $$ BEGIN RAISE EXCEPTION 'injected pending release failure'; END; $$")
        self.sql("CREATE TRIGGER test_pending_release_failure BEFORE UPDATE "
                 "ON platform.sensor_agent_ingress_watermark FOR EACH ROW "
                 "WHEN (OLD.pending_first_sequence IS NOT NULL AND NEW.pending_first_sequence IS NULL "
                 "AND NEW.settled_through_sequence = OLD.settled_through_sequence) "
                 "EXECUTE FUNCTION platform.test_pending_release_failure()")
        stale_seven = self.direct_batch(7, stream, "p0.sequence.release-fault.7", generation=999)
        try:
            code, uncertain = self.core(stale_seven)
            assert code == 503 and uncertain["code"] == "ERR_AGENT_PENDING_RELEASE_UNCERTAIN", (code, uncertain)
            pending_fault = self.watermark()
            assert pending_fault["acked_through_sequence"] == 2
            assert pending_fault["settled_through_sequence"] == 6
            assert pending_fault["pending_first_sequence"] == 7
            assert pending_fault["pending_request_sha256"] is not None
            assert pending_fault["pending_reservation_id"] is not None
            assert pending_fault["pending_publisher_owner"] is None
            assert pending_fault["pending_publication_attempted"] is False
            assert self.terminal(7) is None
            assert not self.kafka_metric("p0.sequence.release-fault.7")
            assert self.clickhouse_count("p0.sequence.release-fault.7") == 0
        finally:
            self.sql("DROP TRIGGER test_pending_release_failure ON platform.sensor_agent_ingress_watermark")
            self.sql("DROP FUNCTION platform.test_pending_release_failure()")
        code, recovered = self.core(stale_seven)
        assert code == 403 and recovered["code"] == "ERR_AGENT_ASSIGNMENT_UNKNOWN", (code, recovered)
        after_fault = self.watermark()
        assert after_fault["pending_first_sequence"] is None
        assert after_fault["settled_through_sequence"] == 6
        assert self.terminal(7) is None
        self.mark("J5", {"retryable_response": uncertain, "pending_after_failure": pending_fault,
                         "recovered_response": recovered, "watermark_after_retry": after_fault,
                         "terminal": None, "kafka_hits": 0, "clickhouse_rows": 0,
                         "sensor_custody_test": "TestDrainRetainsSpoolOnAuthorityAndContinuityErrors/pending_release_uncertain"})

        # An unvalidated rejected request cannot acquire publisher ownership.
        # The PostgreSQL store suite exercises the concurrent claim/release race.
        stale_claimed = self.direct_batch(7, stream, "p0.sequence.claim-race.7", generation=999)
        code, claim_denied = self.core(stale_claimed)
        assert code == 403 and claim_denied["code"] == "ERR_AGENT_ASSIGNMENT_UNKNOWN", (code, claim_denied)
        claim_after = self.watermark()
        assert claim_after["pending_first_sequence"] is None
        assert claim_after["pending_publisher_owner"] is None
        assert self.terminal(7) is None
        assert not self.kafka_metric("p0.sequence.claim-race.7")
        assert self.clickhouse_count("p0.sequence.claim-race.7") == 0
        self.mark("J4", {"response": claim_denied, "watermark": claim_after,
                         "terminal": None, "kafka_hits": 0, "clickhouse_rows": 0,
                         "postgres_tests": ["concurrentPublisherClaimAndPendingReleaseSerializeWithoutErasingClaim",
                                            "pendingReleaseCannotErasePublisherOwnedAttemptedOrFinalizedBatch"]})

        # Model the committed reservation after Core died between mutable
        # assignment rejection and release. Restart the real Core before retry.
        crash_stale = self.direct_batch(7, stream, "p0.sequence.crash-stale.7", generation=999)
        crash_body = json.dumps(crash_stale, separators=(",", ":"))
        crash_digest = hashlib.sha256(crash_body.encode()).hexdigest()
        self.sql("UPDATE platform.sensor_agent_ingress_watermark SET "
                 "pending_first_sequence=7, pending_last_sequence=7, "
                 "pending_checksum='" + crash_stale["checksum"] + "', "
                 "pending_request_sha256='" + crash_digest + "', "
                 "pending_batch_id='" + crash_stale["batchId"] + "', "
                 "pending_correlation_id='" + crash_stale["correlationId"] + "', "
                 "pending_reserved_at=clock_timestamp(), pending_reservation_id=gen_random_uuid(), "
                 "pending_authority_state='UNVALIDATED', "
                 "pending_authority_validated_at=NULL, pending_canonical_batch_sha256=NULL, "
                 "pending_publication_attempted=FALSE, pending_publisher_owner=NULL, "
                 "pending_publisher_claimed_at=NULL, pending_publisher_lease_until=NULL "
                 "WHERE tenant_id='" + TENANT + "' AND sensor_id='" + SENSOR + "' "
                 "AND settled_through_sequence=6 AND pending_first_sequence IS NULL")
        crash_pending = self.watermark()
        assert crash_pending["pending_first_sequence"] == 7
        assert crash_pending["pending_request_sha256"] == crash_digest
        assert crash_pending["pending_reservation_id"] is not None
        assert crash_pending["pending_publisher_owner"] is None
        assert crash_pending["pending_publication_attempted"] is False
        self.call(self.compose + ["restart", "core"], timeout=90)
        self.wait_healthy("core")
        code, crash_denied = self.core(crash_stale)
        assert code == 403 and crash_denied["code"] == "ERR_AGENT_ASSIGNMENT_UNKNOWN", (code, crash_denied)
        crash_cleared = self.watermark()
        assert crash_cleared["acked_through_sequence"] == 2
        assert crash_cleared["settled_through_sequence"] == 6
        assert crash_cleared["pending_first_sequence"] is None
        assert crash_cleared["pending_request_sha256"] is None
        assert crash_cleared["pending_reservation_id"] is None
        assert crash_cleared["pending_publisher_owner"] is None
        assert crash_cleared["pending_publication_attempted"] is False
        assert self.terminal(7) is None
        assert not self.kafka_metric("p0.sequence.crash-stale.7")
        assert self.clickhouse_count("p0.sequence.crash-stale.7") == 0
        crash_corrected = self.direct_batch(7, stream, "p0.sequence.crash-corrected.7", generation=1)
        code, crash_accepted = self.core(crash_corrected)
        assert code == 200 and crash_accepted["status"] == "ACCEPTED", (code, crash_accepted)
        crash_final = self.wait_watermark(2, 7)
        assert crash_final["pending_first_sequence"] is None and self.terminal(7) is None
        assert len(self.kafka_metric("p0.sequence.crash-corrected.7")) == 1
        until = time.monotonic() + 30
        while time.monotonic() < until and self.clickhouse_count("p0.sequence.crash-corrected.7") != 1:
            time.sleep(1)
        assert self.clickhouse_count("p0.sequence.crash-corrected.7") == 1
        self.mark("J6-crash-restart-recovery", {
            "pending_before_restart": crash_pending, "rejected_after_restart": crash_denied,
            "watermark_after_release": crash_cleared, "corrected_response": crash_accepted,
            "final_watermark": crash_final, "stale_kafka_hits": 0, "stale_clickhouse_rows": 0,
            "corrected_kafka_hits": 1, "corrected_clickhouse_rows": 1})

        # Kafka commit survives a failed PostgreSQL finalize. The exact retry
        # must use the persisted canonical entity after Agent authority changes.
        uncertain_eight = self.direct_batch(8, stream, "p0.sequence.uncertain.8", generation=1)
        self.sql("CREATE FUNCTION platform.test_finalize_uncertain() RETURNS trigger "
                 "LANGUAGE plpgsql AS $$ BEGIN RAISE EXCEPTION 'injected finalize uncertainty'; END; $$")
        self.sql("CREATE TRIGGER test_finalize_uncertain BEFORE UPDATE "
                 "ON platform.sensor_agent_ingress_watermark FOR EACH ROW "
                 "WHEN (OLD.pending_first_sequence = 8 AND NEW.pending_first_sequence IS NULL "
                 "AND NEW.settled_through_sequence = 8) "
                 "EXECUTE FUNCTION platform.test_finalize_uncertain()")
        try:
            code, first_uncertain = self.core(uncertain_eight)
            assert code != 200, (code, first_uncertain)
            pending_eight = self.watermark()
            assert pending_eight["pending_first_sequence"] == 8
            assert pending_eight["pending_authority_state"] == "AUTHORITY_VALIDATED"
            assert pending_eight["pending_authority_validated_at"] is not None
            assert pending_eight["pending_canonical_batch_sha256"]
            bindings_eight = self.bindings(pending_eight["pending_reservation_id"])
            assert len(bindings_eight) == 1 and bindings_eight[0]["entity_id"] == ENTITY_A
            assert pending_eight["pending_publication_attempted"] is True
            assert pending_eight["pending_publisher_owner"] is None
            first_hits = self.kafka_metric("p0.sequence.uncertain.8")
            assert len(first_hits) == 1, first_hits
        finally:
            self.sql("DROP TRIGGER test_finalize_uncertain ON platform.sensor_agent_ingress_watermark")
            self.sql("DROP FUNCTION platform.test_finalize_uncertain()")
        assignment_before = self.sql("SELECT row_to_json(a) FROM (SELECT tenant_id, agent_id, "
                                     "sensor_id, generation, status, created_at, revoked_at "
                                     "FROM platform.agent_sensor_assignment WHERE tenant_id='" + TENANT +
                                     "' AND agent_id='" + AGENT + "' AND generation=1) a")
        self.sql("UPDATE platform.agent_sensor_assignment SET status='REVOKED', "
                 "revoked_at=clock_timestamp() WHERE tenant_id='" + TENANT +
                 "' AND agent_id='" + AGENT + "' AND generation=1")
        self.sql("UPDATE platform.entity_agent_binding SET binding_state='REVOKED' "
                 "WHERE tenant_id='" + TENANT + "' AND agent_id='" + AGENT + "'")
        self.sql("INSERT INTO platform.entity (entity_id, tenant_id, entity_type, display_name, "
                 "lifecycle_state, first_seen_at, last_seen_at, created_at, updated_at) VALUES ('" +
                 ENTITY_B + "','" + TENANT + "','HOST','reassigned-entity-b','ACTIVE',now(),now(),now(),now())")
        self.sql("DELETE FROM platform.entity_identifier WHERE tenant_id='" + TENANT +
                 "' AND namespace='platform.agent' AND identifier_type='HOST_ID' "
                 "AND identifier_value='" + GOOD_HOST + "'")
        self.sql("INSERT INTO platform.entity_identifier "
                 "(identifier_id, tenant_id, entity_id, identifier_type, namespace, identifier_value, "
                 "confidence, first_seen_at, last_seen_at) VALUES ('" + fresh_uuid7() + "','" +
                 TENANT + "','" + ENTITY_B + "','HOST_ID','platform.agent','" + GOOD_HOST +
                 "',1.0,now(),now())")
        self.sql("UPDATE platform.entity_agent_binding SET entity_id='" + ENTITY_B +
                 "', binding_state='ACTIVE' WHERE tenant_id='" + TENANT + "' AND agent_id='" + AGENT + "'")
        live_b = self.sql("SELECT entity_id FROM platform.entity_identifier WHERE tenant_id='" + TENANT +
                          "' AND namespace='platform.agent' AND identifier_type='HOST_ID' "
                          "AND identifier_value='" + GOOD_HOST + "'")
        assert live_b == ENTITY_B
        assignment_revoked = self.sql("SELECT row_to_json(a) FROM (SELECT tenant_id, agent_id, "
                                      "sensor_id, generation, status, created_at, revoked_at "
                                      "FROM platform.agent_sensor_assignment WHERE tenant_id='" + TENANT +
                                      "' AND agent_id='" + AGENT + "' AND generation=1) a")
        code, recovered_eight = self.core(uncertain_eight)
        assert code == 200 and recovered_eight["status"] == "ACCEPTED", (code, recovered_eight)
        after_eight = self.wait_watermark(2, 8)
        assert after_eight["pending_first_sequence"] is None
        assert after_eight["pending_authority_state"] == "NONE"
        assert after_eight["pending_publisher_owner"] is None
        assert self.terminal(8) is None
        hits = self.kafka_metric("p0.sequence.uncertain.8")
        assert len(hits) >= 2, hits
        assert len({(h["sequenceId"], h["entityId"]) for h in hits}) == 1
        assert all(h["entityId"] == ENTITY_A for h in hits)
        assert self.bindings(pending_eight["pending_reservation_id"]) == []
        until = time.monotonic() + 30
        while time.monotonic() < until and self.clickhouse_count("p0.sequence.uncertain.8") != 1:
            time.sleep(1)
        assert self.clickhouse_count("p0.sequence.uncertain.8") == 1
        assert self.clickhouse_value("p0.sequence.uncertain.8") == 8.0
        assert all(h["value"] == 8.0 for h in hits)
        self.mark("J7_PUBLICATION_UNCERTAIN_AFTER_AUTHORITY_CHANGE", {
            "first_response": first_uncertain, "pending_after_finalize_failure": pending_eight,
            "assignment_before": json.loads(assignment_before),
            "assignment_revoked": json.loads(assignment_revoked),
            "live_mapping_after_change": live_b, "pending_bindings": bindings_eight,
            "recovery_response": recovered_eight, "final_watermark": after_eight,
            "physical_kafka_count": len(hits), "canonical_clickhouse_count": 1,
            "canonical_value": 8.0,
            "stable_identity": [hits[0]["sequenceId"], hits[0]["entityId"]]})

        # Restore fixture authority for a following independent sequence.
        self.sql("UPDATE platform.agent_sensor_assignment SET status='ACTIVE', revoked_at=NULL "
                 "WHERE tenant_id='" + TENANT + "' AND agent_id='" + AGENT + "' AND generation=1")
        self.sql("UPDATE platform.entity_agent_binding SET binding_state='ACTIVE' "
                 "WHERE tenant_id='" + TENANT + "' AND agent_id='" + AGENT + "'")
        self.sql("UPDATE platform.entity_agent_binding SET entity_id='" + ENTITY_A +
                 "' WHERE tenant_id='" + TENANT + "' AND agent_id='" + AGENT + "'")
        self.sql("DELETE FROM platform.entity_identifier WHERE tenant_id='" + TENANT +
                 "' AND namespace='platform.agent' AND identifier_type='HOST_ID' "
                 "AND identifier_value='" + GOOD_HOST + "'")
        self.sql("INSERT INTO platform.entity_identifier "
                 "(identifier_id, tenant_id, entity_id, identifier_type, namespace, identifier_value, "
                 "confidence, first_seen_at, last_seen_at) VALUES ('" + fresh_uuid7() + "','" +
                 TENANT + "','" + ENTITY_A + "','HOST_ID','platform.agent','" + GOOD_HOST +
                 "',1.0,now(),now())")
        next_nine = self.direct_batch(9, stream, "p0.sequence.next.9", generation=1)
        code, accepted_nine = self.core(next_nine)
        assert code == 200 and accepted_nine["status"] == "ACCEPTED", (code, accepted_nine)
        next_watermark = self.wait_watermark(2, 9)
        assert next_watermark["pending_first_sequence"] is None
        self.mark("J7-next-sequence", {"response": accepted_nine, "watermark": next_watermark})

        # UNVALIDATED reservation survives a real Core restart. Its original
        # qualified Sensor acceptance precedes revocation of generation 1.
        accepted_at = datetime.now(timezone.utc).isoformat(timespec="milliseconds").replace("+00:00", "Z")
        historical_ten = self.direct_batch(10, stream, "p0.sequence.historical.10",
                                           generation=1, accepted_at=accepted_at, qualified=True)
        pending_ten = self.crash_pending(historical_ten, 9)
        self.call(self.compose + ["restart", "core"], timeout=90)
        self.wait_healthy("core")
        self.sql("UPDATE platform.agent_sensor_assignment SET status='REVOKED', "
                 "revoked_at=clock_timestamp() WHERE tenant_id='" + TENANT +
                 "' AND agent_id='" + AGENT + "' AND generation=1")
        self.sql("INSERT INTO platform.agent_sensor_assignment "
                 "(assignment_id, tenant_id, agent_id, sensor_id, generation, status, "
                 "created_at, updated_at, revoked_at) VALUES "
                 "(gen_random_uuid(),'" + TENANT + "','" + AGENT + "','" + SENSOR +
                 "',2,'ACTIVE',clock_timestamp(),clock_timestamp(),NULL)")
        self.sql("CREATE TABLE platform.test_j8_checkpoint_audit "
                 "(reservation_id uuid NOT NULL, authority_state text NOT NULL, "
                 "canonical_digest text NOT NULL)")
        self.sql("CREATE FUNCTION platform.test_j8_checkpoint_capture() RETURNS trigger "
                 "LANGUAGE plpgsql AS $$ BEGIN INSERT INTO platform.test_j8_checkpoint_audit "
                 "VALUES (NEW.pending_reservation_id, NEW.pending_authority_state, "
                 "NEW.pending_canonical_batch_sha256); RETURN NEW; END; $$")
        self.sql("CREATE TRIGGER test_j8_checkpoint_capture AFTER UPDATE "
                 "ON platform.sensor_agent_ingress_watermark FOR EACH ROW "
                 "WHEN (OLD.pending_authority_state = 'UNVALIDATED' "
                 "AND NEW.pending_authority_state = 'AUTHORITY_VALIDATED' "
                 "AND NEW.pending_first_sequence = 10) "
                 "EXECUTE FUNCTION platform.test_j8_checkpoint_capture()")
        code, recovered_ten = self.core(historical_ten)
        assert code == 200 and recovered_ten["status"] == "ACCEPTED", (code, recovered_ten)
        checkpoint_ten = self.sql("SELECT row_to_json(a) FROM (SELECT reservation_id, authority_state, "
                                  "canonical_digest FROM platform.test_j8_checkpoint_audit) a")
        checkpoint_ten = json.loads(checkpoint_ten)
        assert checkpoint_ten["reservation_id"] == pending_ten["pending_reservation_id"]
        assert checkpoint_ten["authority_state"] == "AUTHORITY_VALIDATED"
        assert len(checkpoint_ten["canonical_digest"]) == 64
        self.sql("DROP TRIGGER test_j8_checkpoint_capture ON platform.sensor_agent_ingress_watermark")
        self.sql("DROP FUNCTION platform.test_j8_checkpoint_capture()")
        self.sql("DROP TABLE platform.test_j8_checkpoint_audit")
        after_ten = self.wait_watermark(2, 10)
        assert after_ten["pending_first_sequence"] is None
        assert self.bindings(pending_ten["pending_reservation_id"]) == []
        ten_hits = self.kafka_metric("p0.sequence.historical.10")
        assert len(ten_hits) == 1 and ten_hits[0]["entityId"] == ENTITY_A
        self.mark("J8_UNVALIDATED_CRASH_THEN_REVOCATION_positive", {
            "pending_before_restart": pending_ten, "response": recovered_ten,
            "committed_authority_checkpoint": checkpoint_ten,
            "final_watermark": after_ten, "entity": ten_hits[0]["entityId"],
            "physical_kafka_count": len(ten_hits), "binding_cleanup": True})

        next_eleven = self.direct_batch(11, stream, "p0.sequence.next.11", generation=2)
        code, accepted_eleven = self.core(next_eleven)
        assert code == 200 and accepted_eleven["status"] == "ACCEPTED", (code, accepted_eleven)
        self.wait_watermark(2, 11)

        negative_at = datetime.now(timezone.utc).isoformat(timespec="milliseconds").replace("+00:00", "Z")
        unqualified_twelve = self.direct_batch(12, stream, "p0.sequence.unqualified.12",
                                               generation=2, accepted_at=negative_at)
        unqualified_twelve["records"][0]["acceptanceEvidence"] = {
            "evidenceVersion": 1, "acceptedAtUtc": negative_at, "clockQuality": "UNQUALIFIED",
            "clockSource": "TEST_CLOCK_UNPROVEN", "sensorServiceInstanceId": "runtime-matrix-instance",
            "durableSpoolSequence": 12}
        pending_twelve = self.crash_pending(unqualified_twelve, 11)
        self.call(self.compose + ["restart", "core"], timeout=90)
        self.wait_healthy("core")
        self.sql("UPDATE platform.agent_sensor_assignment SET status='REVOKED', "
                 "revoked_at=clock_timestamp() WHERE tenant_id='" + TENANT +
                 "' AND agent_id='" + AGENT + "' AND generation=2")
        code, denied_twelve = self.core(unqualified_twelve)
        assert code == 403 and denied_twelve["code"] == "ERR_AGENT_AUTHORITY_ORDER_UNCERTAIN", (code, denied_twelve)
        after_denied = self.watermark()
        assert after_denied["acked_through_sequence"] == 2
        assert after_denied["settled_through_sequence"] == 11
        assert after_denied["pending_first_sequence"] is None
        assert after_denied["pending_publisher_owner"] is None
        assert self.bindings(pending_twelve["pending_reservation_id"]) == []
        assert not self.kafka_metric("p0.sequence.unqualified.12")
        assert self.clickhouse_count("p0.sequence.unqualified.12") == 0
        assert self.terminal(12) is None
        self.mark("J8_UNVALIDATED_CRASH_THEN_REVOCATION_negative", {
            "pending_before_restart": pending_twelve, "response": denied_twelve,
            "watermark_after_release": after_denied, "physical_kafka_count": 0,
            "canonical_clickhouse_count": 0, "binding_cleanup": True,
            "next_sequence_response": accepted_eleven})

        malformed_code, _ = self.sensor("{")
        assert malformed_code == 400
        oversized = self.envelope(6, GOOD_HOST, "p0.sequence.oversize.6")
        oversized["correlationId"] = "x" * (1024 * 1024)
        oversized_code, oversize_response = self.sensor(json.dumps(oversized, separators=(",", ":")))
        assert oversized_code == 413, (oversized_code, oversize_response)
        assert self.watermark()["settled_through_sequence"] == 11
        self.mark("K", {"malformed_http": malformed_code, "oversize_http": oversized_code})

        rejected_hits = self.kafka_metric("p0.sequence.rejected.3")
        assert not rejected_hits and self.clickhouse_count("p0.sequence.rejected.3") == 0
        self.mark("B-no-customer-telemetry", {"kafka_hits": 0, "clickhouse_rows": 0})
        self.write("final-watermark.json", self.watermark())


def main():
    prefix, port, evidence = sys.argv[1:]
    matrix = Matrix(prefix, port, evidence)
    try:
        matrix.run()
    except Exception as exc:
        matrix.write("sequence-matrix-failure.json", {"error": repr(exc), "results": matrix.results,
                                                      "watermark": matrix.watermark()})
        raise


if __name__ == "__main__":
    main()
