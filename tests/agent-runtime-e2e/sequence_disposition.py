#!/usr/bin/env python3
"""Real isolated Agent→Sensor→Core sequence-disposition regression matrix."""

import hashlib
import json
import os
from pathlib import Path
import subprocess
import sys
import time

from build_envelope import build_envelope

TENANT = "018f0e0a-0000-7000-8000-000000000001"
OTHER_TENANT = "018f0e0a-0000-7000-8000-000000000099"
AGENT = "agent-e2e-01"
SENSOR = os.environ.get("SENSOR_INFRA_ID", "sensor-infra-01")
GOOD_HOST = "host-e2e-agent-01"
CONFLICT_HOST = "host-e2e-probe-01"


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
                        "settled_through_sequence, pending_first_sequence, last_batch_id "
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
                  "acceptedAt": payload["timestamp"], "records": env["records"]}
        return {"schemaVersion": 2, "sensorId": SENSOR, "sensorAgentSpoolStreamId": terminal["stream_id"],
                "batchId": terminal["batch_id"], "firstIngressSequence": 3, "lastIngressSequence": 3,
                "recordCount": 1, "checksum": terminal["checksum"],
                "correlationId": terminal["correlation_id"], "records": [record]}

    def direct_batch(self, seq, stream, metric, generation=1, tenant=TENANT):
        env = self.envelope(seq, GOOD_HOST, metric)
        record = {"sensorIngressSequence": seq, "sensorIngressRecordId": f"direct-p0-{seq}",
                  "tenantId": tenant, "agentId": AGENT, "acceptingSensorId": SENSOR,
                  "assignmentGeneration": generation, "agentFirstSequence": seq,
                  "agentLastSequence": seq, "agentBatchId": env["batchId"],
                  "correlationId": env["correlationId"], "acceptedAt": "2026-11-09T00:00:00Z",
                  "records": env["records"]}
        delivery = env["records"][0]
        line = "|".join(map(str, [seq, record["agentBatchId"], seq, seq, generation,
                                  tenant, AGENT, SENSOR, seq, delivery["id"], delivery["checksum"]]))
        checksum = hashlib.sha256(line.encode()).hexdigest()
        bid = f"agent-ingress-{SENSOR}-{stream}-{seq:020d}-{seq:020d}-{checksum[:16]}"
        return {"schemaVersion": 2, "sensorId": SENSOR, "sensorAgentSpoolStreamId": stream,
                "batchId": bid, "firstIngressSequence": seq, "lastIngressSequence": seq,
                "recordCount": 1, "checksum": checksum, "correlationId": "agentdrain-" + bid,
                "records": [record]}

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
        assert self.watermark()["settled_through_sequence"] == 5 and self.terminal(6) is None
        self.mark("J", denied)

        malformed_code, _ = self.sensor("{")
        assert malformed_code == 400
        oversized = self.envelope(6, GOOD_HOST, "p0.sequence.oversize.6")
        oversized["correlationId"] = "x" * (1024 * 1024)
        oversized_code, oversize_response = self.sensor(json.dumps(oversized, separators=(",", ":")))
        assert oversized_code == 413, (oversized_code, oversize_response)
        assert self.watermark()["settled_through_sequence"] == 5
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
