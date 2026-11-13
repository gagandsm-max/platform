#!/usr/bin/env python3
"""J9: real Sensor custody inside an uncommitted A-to-B identifier gap."""

import base64
import hashlib
import json
import os
from pathlib import Path
import select
import subprocess
import sys
import time
from datetime import datetime, timedelta, timezone

from binding_history_activation import confirm_active_open_agent_binding_sql
from sequence_disposition import Matrix, TENANT, AGENT, SENSOR, GOOD_HOST, ENTITY_A, ENTITY_B
from build_envelope import build_envelope

SPOOL = "/var/lib/platform/sensor/agent-spool"
BARRIER_KEY = 211021109


def instant(value):
    return datetime.fromisoformat(value.replace("Z", "+00:00"))


class Gap(Matrix):
    def __init__(self, prefix, port, evidence):
        super().__init__(prefix, port, evidence)
        self.marker = self.evidence.parent / "timesync-marker"

    def mark(self, label, detail):
        self.results[label] = {"status": "PASS", "detail": detail}
        self.write("real-sensor-gap-matrix.json", self.results)
        print("[PASS]", label, flush=True)

    def session(self):
        return subprocess.Popen(
            ["docker", "exec", "-i", self.prefix + "-postgres", "psql", "-X", "-qAt",
             "-U", "platform_app", "-d", "platform", "-v", "ON_ERROR_STOP=1"],
            stdin=subprocess.PIPE, stdout=subprocess.PIPE, stderr=subprocess.PIPE,
            text=True, bufsize=1)

    @staticmethod
    def statement(proc, sql, expected=None):
        assert proc.poll() is None, proc.stderr.read()
        proc.stdin.write(sql + "\n")
        proc.stdin.flush()
        if expected is None:
            return None
        ready, _, _ = select.select([proc.stdout], [], [], 30)
        assert ready, f"psql statement timed out: {sql[:80]}"
        got = proc.stdout.readline().strip()
        assert got == expected, (expected, got, sql)
        return got

    def wait_sql(self, predicate, seconds=45):
        until = time.monotonic() + seconds
        last = None
        while time.monotonic() < until:
            last = self.sql(predicate)
            if last == "t":
                return
            time.sleep(0.2)
        raise AssertionError((predicate, last))

    def wait_after(self, timestamp, extra_ms=5000):
        target = instant(timestamp) + timedelta(milliseconds=extra_ms)
        self.wait_sql("SELECT clock_timestamp() > '" + target.isoformat() + "'::timestamptz", 30)

    def current_identifier(self):
        return self.sql("SELECT entity_id FROM platform.entity_identifier WHERE tenant_id='" + TENANT +
                        "' AND namespace='platform.agent' AND identifier_type='HOST_ID' "
                        "AND identifier_value='" + GOOD_HOST + "'")

    def history(self):
        return json.loads(self.sql("SELECT coalesce(json_agg(row_to_json(t) ORDER BY history_version),'[]') "
                 "FROM (SELECT history_version,entity_identifier_id,entity_id,change_started_at,"
                 "valid_from,valid_to,activation_state,activated_at FROM "
                 "platform.entity_identifier_binding_history WHERE tenant_id='" + TENANT +
                 "' AND namespace='platform.agent' AND identifier_type='HOST_ID' "
                 "AND identifier_value='" + GOOD_HOST + "' ORDER BY history_version) t"))

    def sensor_file(self, relative):
        return self.call(["docker", "exec", self.prefix + "-sensor-infra", "cat", SPOOL + "/" + relative])

    def restart_sensor(self):
        self.call(self.compose + ["restart", "sensor-infra"], timeout=90)
        self.wait_healthy("sensor-infra")
        endpoint = self.call(self.compose + ["port", "sensor-infra", "8444"]).strip()
        assert endpoint.startswith("127.0.0.1:"), endpoint
        self.port = endpoint.rsplit(":", 1)[1]

    def sensor_record(self, seq, envelope):
        path = f"record-{seq:020d}.json"
        sidecar_path = f"acceptance-evidence/{seq:020d}.json"
        raw_record = self.sensor_file(path)
        raw_sidecar = self.sensor_file(sidecar_path)
        record, sidecar = json.loads(raw_record), json.loads(raw_sidecar)
        payload_bytes = base64.b64decode(record["payload"], validate=True)
        payload = json.loads(payload_bytes)
        assert record["sequence"] == seq and record["type"] == "agent.delivery.batch"
        assert hashlib.sha256(payload_bytes).hexdigest() == record["checksum"] == sidecar["payloadSha256"]
        assert payload["batchId"] == envelope["batchId"] and payload["agentId"] == AGENT
        assert payload["envelope"]["batchId"] == envelope["batchId"]
        assert sidecar["spoolSequence"] == seq == sidecar["evidence"]["durableSpoolSequence"]
        assert sidecar["evidence"]["acceptedAtUtc"] == sidecar["acceptedAtUtc"]
        assert sidecar["evidence"]["clockQuality"] == "QUALIFIED", sidecar
        assert 0 <= sidecar["evidence"]["clockMaxErrorMs"] <= 5000
        assert 0 <= sidecar["evidence"]["clockSyncAgeMs"] <= 300000
        assert sidecar["evidence"]["clockSource"] == "LINUX_ADJTIMEX_SYSTEMD_TIMESYNCD"
        self.write(f"j9-spool-{seq}.json", {"path": path, "record": record,
                                            "payload": payload, "sidecar_path": sidecar_path,
                                            "sidecar": sidecar,
                                            "sidecar_sha256": hashlib.sha256(raw_sidecar.encode()).hexdigest()})
        return payload, sidecar

    def accept(self, seq, metric):
        self.marker.touch()
        envelope = self.submit(seq, GOOD_HOST, metric)
        payload, sidecar = self.sensor_record(seq, envelope)
        state = json.loads(self.sensor_file("state.json"))
        assert state["ackedThrough"] < seq and state["nextSequence"] > seq, state
        assert self.watermark() is None or self.watermark()["settled_through_sequence"] < seq
        return envelope, payload, sidecar, state

    def batch_from_custody(self, seq, payload, sidecar):
        delivery = payload["envelope"]["records"]
        fields = [seq, payload["batchId"], payload["firstSequence"], payload["lastSequence"],
                  payload["assignmentGeneration"], payload["tenantId"], payload["agentId"],
                  payload["sensorId"]]
        line = "|".join(map(str, fields))
        for rec in delivery:
            line += "|" + "|".join(map(str, (rec["sequence"], rec["id"], rec["checksum"])))
        checksum = hashlib.sha256(line.encode()).hexdigest()
        stream = sidecar["spoolStreamId"]
        batch_id = f"agent-ingress-{SENSOR}-{stream}-{seq:020d}-{seq:020d}-{checksum[:16]}"
        rec = {"sensorIngressSequence": seq, "sensorIngressRecordId": f"agent-spool-{seq}",
               "tenantId": payload["tenantId"], "agentId": payload["agentId"],
               "acceptingSensorId": payload["sensorId"],
               "assignmentGeneration": payload["assignmentGeneration"],
               "agentFirstSequence": payload["firstSequence"], "agentLastSequence": payload["lastSequence"],
               "agentBatchId": payload["batchId"], "correlationId": payload["correlationId"],
               "acceptedAt": sidecar["acceptedAtUtc"], "acceptanceEvidence": sidecar["evidence"],
               "records": delivery}
        return {"schemaVersion": 2, "sensorId": SENSOR, "sensorAgentSpoolStreamId": stream,
                "batchId": batch_id, "firstIngressSequence": seq, "lastIngressSequence": seq,
                "recordCount": 1, "checksum": checksum, "correlationId": "agentdrain-" + batch_id,
                "records": [rec]}

    def wait_pending(self, seq):
        until = time.monotonic() + 60
        while time.monotonic() < until:
            row = self.watermark()
            if row and row["pending_first_sequence"] == seq:
                return row
            time.sleep(0.3)
        raise AssertionError(("pending not created", seq, self.watermark()))

    def wait_sensor_rejection(self, code, seq):
        until = time.monotonic() + 40
        while time.monotonic() < until:
            logs = subprocess.run(["docker", "logs", self.prefix + "-sensor-infra"],
                                  text=True, capture_output=True, timeout=10)
            assert logs.returncode == 0
            events = [json.loads(line) for line in (logs.stdout + logs.stderr).splitlines()
                      if line.startswith("{")]
            matching = [event for event in events if event.get("event_code") == code
                        and event.get("safe_context", {}).get("first_sequence") == seq]
            if matching:
                return matching[-1]
            time.sleep(0.3)
        raise AssertionError(("Sensor did not retry exact custodied head", code, seq))

    def unrelated_stream(self):
        # A second authenticated Sensor identity and separate Agent principal
        # prove the pending infrastructure-Sensor stream is not a Core-wide
        # schema/ingress wedge. This is a direct Core fixture, not a claim of
        # second real Sensor durable acceptance.
        other_sensor = "sensor-net-01"
        other_agent = "agent-e2e-probe-01"
        other_host = "host-e2e-probe-01"
        other_entity = "018f0e0a-0000-7000-8000-00000000ae02"
        self.sql("UPDATE platform.sensor SET role='COMBINED' WHERE sensor_id='" + other_sensor +
                 "' AND tenant_id='" + TENANT + "'")
        self.sql("UPDATE platform.agent_sensor_assignment SET sensor_id='" + other_sensor +
                 "' WHERE tenant_id='" + TENANT + "' AND agent_id='" + other_agent +
                 "' AND generation=1")
        env = json.loads(build_envelope(envelope_tenant=TENANT, payload_tenant=TENANT,
                    agent_id=other_agent, host_id=other_host, sequence=1,
                    metric_key="p0.j9.unrelated.1", value=1.0,
                    batch_id_value=None, correlation_id="p0-j9-unrelated"))
        delivery = env["records"][0]
        line = "|".join(map(str, (1, env["batchId"], 1, 1, 1, TENANT, other_agent,
                                  other_sensor, 1, delivery["id"], delivery["checksum"])))
        checksum = hashlib.sha256(line.encode()).hexdigest()
        stream = "stream-11111111111111111111111111111111"
        batch_id = f"agent-ingress-{other_sensor}-{stream}-{1:020d}-{1:020d}-{checksum[:16]}"
        record = {"sensorIngressSequence": 1, "sensorIngressRecordId": "j9-unrelated-1",
                  "tenantId": TENANT, "agentId": other_agent,
                  "acceptingSensorId": other_sensor, "assignmentGeneration": 1,
                  "agentFirstSequence": 1, "agentLastSequence": 1,
                  "agentBatchId": env["batchId"], "correlationId": env["correlationId"],
                  "acceptedAt": datetime.now(timezone.utc).isoformat().replace("+00:00", "Z"),
                  "records": env["records"]}
        batch = {"schemaVersion": 2, "sensorId": other_sensor,
                 "sensorAgentSpoolStreamId": stream, "batchId": batch_id,
                 "firstIngressSequence": 1, "lastIngressSequence": 1, "recordCount": 1,
                 "checksum": checksum, "correlationId": "agentdrain-" + batch_id,
                 "records": [record]}
        code, response = self.http(["docker", "exec", "-i", self.prefix + "-sensor-infra", "curl",
                "-sS", "--connect-timeout", "5", "--max-time", "30", "--cacert", "/certs/ca.crt",
                "--cert", "/certs/sensor-net.crt", "--key", "/certs/sensor-net.key",
                "-H", "Content-Type: application/json", "-H", "Idempotency-Key: " + batch_id,
                "https://core:8443/api/v1/sensor/agent-telemetry"],
                json.dumps(batch, separators=(",", ":")))
        assert code == 200 and response["status"] == "ACCEPTED", (code, response)
        row = self.sql("SELECT row_to_json(t) FROM (SELECT acked_through_sequence,"
                       "settled_through_sequence,pending_first_sequence FROM "
                       "platform.sensor_agent_ingress_watermark WHERE tenant_id='" + TENANT +
                       "' AND sensor_id='" + other_sensor + "') t")
        assert row and json.loads(row)["settled_through_sequence"] == 1
        hits = self.kafka_metric("p0.j9.unrelated.1")
        assert len(hits) == 1 and hits[0]["entityId"] == other_entity
        return {"sensor": other_sensor, "response": response,
                "watermark": json.loads(row), "kafka_count": 1, "entity": other_entity}

    def run_gap(self):
        assert self.current_identifier() == ENTITY_A
        a = self.history()
        assert len(a) == 1 and a[0]["entity_id"] == ENTITY_A and a[0]["activation_state"] == "ACTIVE"
        # Seed must have activated Agent↔entity history after commit (V022).
        assert self.sql(confirm_active_open_agent_binding_sql(TENANT, AGENT)) == "1"
        self.wait_after(a[0]["valid_from"])

        # Positive control: the same real Sensor spool path resolves A after
        # generation 1 is revoked, proving historical lookup can succeed.
        env1, payload1, evidence1, custody1 = self.accept(1, "p0.j9.positive.1")
        self.wait_after(evidence1["acceptedAtUtc"], evidence1["evidence"]["clockMaxErrorMs"])
        self.sql("UPDATE platform.agent_sensor_assignment SET status='REVOKED',revoked_at=clock_timestamp() "
                 "WHERE tenant_id='" + TENANT + "' AND agent_id='" + AGENT + "' AND generation=1")
        self.sql("INSERT INTO platform.agent_sensor_assignment (assignment_id,tenant_id,agent_id,sensor_id,"
                 "generation,status,created_at,updated_at) VALUES (gen_random_uuid(),'" + TENANT + "','" +
                 AGENT + "','" + SENSOR + "',2,'ACTIVE',clock_timestamp(),clock_timestamp())")
        self.restart_sensor()
        positive = self.wait_watermark(1, 1)
        hits = self.kafka_metric("p0.j9.positive.1")
        assert len(hits) == 1 and hits[0]["entityId"] == ENTITY_A
        self.mark("J9_POSITIVE_REAL_SENSOR_HISTORICAL_A", {
            "agent_batch_id": env1["batchId"], "sensor_evidence": evidence1,
            "custody_before_drain": custody1, "watermark": positive,
            "kafka_count": 1, "resolved_entity": ENTITY_A})

        self.sql("INSERT INTO platform.entity (entity_id,tenant_id,entity_type,display_name,lifecycle_state,"
                 "first_seen_at,last_seen_at,created_at,updated_at) VALUES ('" + ENTITY_B + "','" + TENANT +
                 "','HOST','j9-entity-b','ACTIVE',now(),now(),now(),now())")
        holder, t1 = self.session(), self.session()
        try:
            self.statement(holder, f"SELECT 'HELD' FROM (SELECT pg_advisory_lock({BARRIER_KEY})) s;", "HELD")
            self.statement(t1, "BEGIN;")
            self.statement(t1, "SELECT pg_backend_pid()::text || ':' || txid_current()::text;", None)
            ready, _, _ = select.select([t1.stdout], [], [], 30)
            assert ready
            backend, xid = t1.stdout.readline().strip().split(":")
            self.statement(t1, "UPDATE platform.entity_identifier SET entity_id='" + ENTITY_B +
                           "' WHERE tenant_id='" + TENANT + "' AND namespace='platform.agent' "
                           "AND identifier_type='HOST_ID' AND identifier_value='" + GOOD_HOST + "';")
            self.statement(t1, "SELECT 'TRIGGER_DONE' FROM platform.entity_identifier_binding_history "
                           "WHERE tenant_id='" + TENANT + "' AND entity_id='" + ENTITY_B +
                           "' AND activation_state='PENDING_COMMIT_VISIBILITY' AND valid_from IS NULL;",
                           "TRIGGER_DONE")
            self.statement(t1, "SELECT row_to_json(h) FROM (SELECT change_started_at,valid_from,"
                           "activation_state FROM platform.entity_identifier_binding_history WHERE tenant_id='" +
                           TENANT + "' AND entity_id='" + ENTITY_B +
                           "' AND activation_state='PENDING_COMMIT_VISIBILITY') h;")
            ready, _, _ = select.select([t1.stdout], [], [], 30)
            assert ready
            pending_in_t1 = json.loads(t1.stdout.readline())
            assert pending_in_t1["valid_from"] is None
            self.statement(t1, f"SELECT 'BARRIER_RELEASED' FROM (SELECT pg_advisory_lock({BARRIER_KEY})) s;")
            self.wait_sql("SELECT EXISTS (SELECT 1 FROM pg_stat_activity WHERE pid=" + backend +
                          " AND state='active' AND wait_event_type='Lock' AND wait_event='advisory' "
                          "AND backend_xid IS NOT NULL)")
            t1_before = self.sql("SELECT row_to_json(t) FROM (SELECT pid,backend_xid,state,wait_event_type,"
                                 "wait_event,xact_start FROM pg_stat_activity WHERE pid=" + backend + ") t")
            # T1's own pending row is not visible to committed readers. Keep
            # it blocked until its conservative closure precedes the complete
            # acceptance uncertainty interval.
            self.wait_after(pending_in_t1["change_started_at"])
            env2, payload2, evidence2, custody2 = self.accept(2, "p0.j9.gap.2")
            committed_during = {"entity": self.current_identifier(), "history": self.history(),
                                "t1": json.loads(self.sql("SELECT row_to_json(t) FROM (SELECT pid,backend_xid,"
                                "state,wait_event_type,wait_event,xact_start FROM pg_stat_activity WHERE pid=" +
                                backend + ") t"))}
            assert committed_during["entity"] == ENTITY_A
            assert all(r["entity_id"] != ENTITY_B for r in committed_during["history"])
            assert committed_during["t1"]["backend_xid"] is not None
            assert committed_during["t1"]["wait_event"] == "advisory"
            assert self.watermark()["settled_through_sequence"] == 1
            self.mark("J9_REAL_SENSOR_ACCEPTANCE_DURING_UNCOMMITTED_ENTITY_REPOINT", {
                "t1_backend_pid": int(backend), "t1_xid": xid,
                "t1_private_pending": pending_in_t1,
                "t1_barrier_before_acceptance": json.loads(t1_before),
                "real_sensor_batch_id": env2["batchId"], "sensor_evidence": evidence2,
                "custody": custody2, "committed_reader_while_blocked": committed_during})
            self.wait_after(evidence2["acceptedAtUtc"], evidence2["evidence"]["clockMaxErrorMs"])
            self.statement(holder, f"SELECT pg_advisory_unlock({BARRIER_KEY});", "t")
            ready, _, _ = select.select([t1.stdout], [], [], 30)
            assert ready and t1.stdout.readline().strip() == "BARRIER_RELEASED"
            self.statement(t1, "COMMIT;")
            self.statement(t1, "SELECT 'COMMITTED';", "COMMITTED")
        finally:
            for proc in (t1, holder):
                if proc.poll() is None:
                    proc.stdin.close()
                    proc.terminate()
                    proc.wait(timeout=5)

        assert self.current_identifier() == ENTITY_B
        self.activate_identifier_history()
        after = self.history()
        a_row = next(r for r in after if r["entity_id"] == ENTITY_A)
        b_row = next(r for r in after if r["entity_id"] == ENTITY_B)
        lower = instant(evidence2["acceptedAtUtc"]) - timedelta(milliseconds=evidence2["evidence"]["clockMaxErrorMs"])
        upper = instant(evidence2["acceptedAtUtc"]) + timedelta(milliseconds=evidence2["evidence"]["clockMaxErrorMs"])
        assert a_row["valid_to"] and instant(a_row["valid_to"]) <= lower
        assert b_row["activation_state"] == "ACTIVE" and instant(b_row["valid_from"]) > upper
        assert not (instant(a_row["valid_from"]) <= lower and instant(a_row["valid_to"]) > upper)
        historical = self.sql("SELECT count(*) FROM platform.entity_identifier_binding_history WHERE tenant_id='" +
                    TENANT + "' AND namespace='platform.agent' AND identifier_type='HOST_ID' "
                    "AND identifier_value='" + GOOD_HOST + "' AND activation_state='ACTIVE' "
                    "AND valid_from <= '" + lower.isoformat() + "'::timestamptz "
                    "AND (valid_to IS NULL OR valid_to > '" + upper.isoformat() + "'::timestamptz)")
        assert historical == "0", historical
        self.mark("J9_POST_COMMIT_NO_HISTORICAL_BINDING", {"current_entity": ENTITY_B,
                  "history": after, "accepted_lower": lower.isoformat(),
                  "accepted_upper": upper.isoformat(), "historical_count": 0})

        # Make the current binding coherent with B only for the initial,
        # pre-migration checkpoint attempt. The persisted Sensor acceptance
        # remains the earlier A-to-B gap and is never edited. Activate Agent
        # history after this committed write so the negative case stays the
        # intended identifier-gap / LEGACY reconciliation failure — not
        # ERR_AGENT_LEGACY_ENTITY_UNPROVEN from leftover PENDING history.
        self.sql("UPDATE platform.entity_agent_binding SET entity_id='" + ENTITY_B +
                 "' WHERE tenant_id='" + TENANT + "' AND agent_id='" + AGENT + "'")
        self.activate_agent_binding_history()
        # E2E-only database trigger fails the first production Core checkpoint
        # after its committed reservation. It never writes a pending row.
        self.sql("CREATE FUNCTION platform.test_j9_checkpoint_failure() RETURNS trigger LANGUAGE plpgsql "
                 "AS $$ BEGIN RAISE EXCEPTION 'j9 checkpoint fault'; END; $$")
        self.sql("CREATE TRIGGER test_j9_checkpoint_failure BEFORE UPDATE ON "
                 "platform.sensor_agent_ingress_watermark FOR EACH ROW WHEN "
                 "(OLD.pending_authority_state='UNVALIDATED' AND "
                 "NEW.pending_authority_state='AUTHORITY_VALIDATED' AND NEW.pending_first_sequence=2) "
                 "EXECUTE FUNCTION platform.test_j9_checkpoint_failure()")
        try:
            self.restart_sensor()
            pending = self.wait_pending(2)
            assert pending["pending_authority_state"] == "UNVALIDATED" and pending["pending_request_sha256"]
        finally:
            self.sql("DROP TRIGGER test_j9_checkpoint_failure ON platform.sensor_agent_ingress_watermark")
            self.sql("DROP FUNCTION platform.test_j9_checkpoint_failure()")
        self.sql("UPDATE platform.agent_sensor_assignment SET status='REVOKED',revoked_at=clock_timestamp() "
                 "WHERE tenant_id='" + TENANT + "' AND agent_id='" + AGENT + "' AND generation=2")
        self.sql("UPDATE platform.sensor_agent_ingress_watermark SET pending_authority_state='LEGACY_UNKNOWN', "
                 "pending_publication_attempted=NULL "
                 "WHERE tenant_id='" + TENANT + "' AND sensor_id='" + SENSOR +
                 "' AND pending_first_sequence=2 AND pending_authority_state='UNVALIDATED'")
        legacy = self.watermark()
        assert legacy["pending_authority_state"] == "LEGACY_UNKNOWN"
        batch = self.batch_from_custody(2, payload2, evidence2)
        body = json.dumps(batch, separators=(",", ":"))
        assert hashlib.sha256(body.encode()).hexdigest() == legacy["pending_request_sha256"]
        code, response = self.core(batch)
        assert code == 503 and response["code"] == "ERR_AGENT_LEGACY_PENDING_RECONCILIATION_REQUIRED", (code, response)
        self.restart_sensor()
        sensor_retry = self.wait_sensor_rejection(
            "ERR_AGENT_LEGACY_PENDING_RECONCILIATION_REQUIRED", 2)
        final = self.watermark()
        assert final["acked_through_sequence"] == 1 and final["settled_through_sequence"] == 1
        assert final["pending_first_sequence"] == 2 and final["pending_authority_state"] == "LEGACY_UNKNOWN"
        assert self.terminal(2) is None
        retained = json.loads(self.sensor_file("state.json"))
        assert retained["ackedThrough"] == 1 and retained["nextSequence"] == 3
        assert json.loads(self.sensor_file("acceptance-evidence/00000000000000000002.json")) == evidence2
        assert not self.kafka_metric("p0.j9.gap.2")
        assert self.clickhouse_count("p0.j9.gap.2") == 0
        assert self.sql("SELECT count(*) FROM pg_locks WHERE relation="
                        "'platform.sensor_agent_ingress_watermark'::regclass "
                        "AND mode='AccessExclusiveLock' AND granted") == "0"
        unrelated = self.unrelated_stream()
        self.mark("J9_U5B_REAL_SENSOR_RETRY_RETAINS_CUSTODY", {
            "core_http_code": code, "core_response": response, "pending": final,
            "sensor_retry_log": sensor_retry, "unrelated_stream": unrelated,
            "sensor_spool_state": retained, "sensor_sidecar_retained": True,
            "kafka_count": 0, "clickhouse_count": 0, "ack_or_deletion": False,
            "terminal": None})


def main():
    prefix, port, evidence = sys.argv[1:]
    gap = Gap(prefix, port, evidence)
    try:
        gap.run_gap()
    except Exception as exc:
        gap.write("real-sensor-gap-failure.json", {"error": repr(exc), "results": gap.results})
        raise


if __name__ == "__main__":
    main()
