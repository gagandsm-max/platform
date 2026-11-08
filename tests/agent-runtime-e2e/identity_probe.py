"""Explicit A/B credentials → Sensor custody; typed rejection at Core acceptance.

Uses real private fixture services, no mocks. Sensor custody ACK is not Core ACK.
Direct Core challenges use independent test streams to inspect each HTTP rejection,
while the production drain also receives both authenticated forged observations.
"""
import hashlib
import json
import os
from pathlib import Path
import subprocess
import sys
import uuid
from datetime import datetime, timezone
from build_envelope import build_envelope

TENANT = "018f0e0a-0000-7000-8000-000000000001"
FIXTURES = {
    "A": {"agent": "agent-e2e-01", "host": "host-e2e-agent-01",
          "entity": "018f0e0a-0000-7000-8000-00000000ae01", "cert": "agent-e2e"},
    "B": {"agent": "agent-e2e-probe-01", "host": "host-e2e-probe-01",
          "entity": "018f0e0a-0000-7000-8000-00000000ae02", "cert": "agent-probe"},
}


def command(args, body=None):
    return subprocess.check_output(args, input=body, text=True, timeout=30)


def core_batch(envelope, sensor, generation):
    record = dict(sensorIngressSequence=1, sensorIngressRecordId="identity-probe-1", tenantId=TENANT,
                  agentId=envelope["agentId"], acceptingSensorId=sensor, assignmentGeneration=generation,
                  agentFirstSequence=envelope["firstSequence"], agentLastSequence=envelope["lastSequence"],
                  agentBatchId=envelope["batchId"], correlationId=envelope["correlationId"],
                  acceptedAt=datetime.now(timezone.utc).isoformat(), records=envelope["records"])
    fields = [1, record["agentBatchId"], record["agentFirstSequence"], record["agentLastSequence"],
              generation, TENANT, record["agentId"], sensor]
    for delivery in record["records"]:
        fields.extend([delivery["sequence"], delivery["id"], delivery["checksum"]])
    return dict(schemaVersion=2, sensorId=sensor, sensorAgentSpoolStreamId="stream-" + uuid.uuid4().hex,
                batchId="identity-probe-" + uuid.uuid4().hex, firstIngressSequence=1, lastIngressSequence=1,
                recordCount=1, checksum=hashlib.sha256("|".join(map(str, fields)).encode()).hexdigest(),
                correlationId=envelope["correlationId"], records=[record])


def main():
    prefix, receiver, run_id, output = sys.argv[1:]
    output = Path(output)
    certs = Path(os.environ["PLATFORM_E2E_CERT_DIR"])
    ca = Path(os.environ["PLATFORM_E2E_CA_DIR"]) / "ca.crt"
    sensor = os.environ.get("SENSOR_INFRA_ID", "sensor-infra-01")
    def sql(query):
        return command(["docker", "exec", prefix + "-postgres", "psql", "-U", "platform_app",
                        "-d", "platform", "-At", "-v", "ON_ERROR_STOP=1", "-c", query]).strip()
    command(["docker", "pause", prefix + "-agent-runtime"])
    command(["docker", "cp", prefix + "-agent-runtime:/var/lib/platform/agent/queue/sequence.meta",
             str(output / "agent-final-sequence.meta")])
    meta = json.loads((output / "agent-final-sequence.meta").read_text())
    results = []
    for source, target in (("A", "B"), ("B", "A")):
        identity, other = FIXTURES[source], FIXTURES[target]
        label = source + "-claims-" + target
        metric = "e2e.forged." + label + "." + run_id
        sequence = meta["highestSequence"] + 1 if source == "A" else 900101
        envelope = json.loads(build_envelope(envelope_tenant=TENANT, payload_tenant=TENANT,
            agent_id=identity["agent"], host_id=other["host"], sequence=sequence,
            metric_key=metric, value=123, batch_id_value=None, correlation_id=label + "-" + run_id))
        if source == "A":
            envelope.update(schemaVersion=2, agentStreamId=meta["agentStreamId"])
        raw = json.dumps(envelope, separators=(",", ":"))
        (output / (label + "-agent.json")).write_text(raw)
        sensor_raw = command(["curl", "-sS", "--connect-timeout", "5", "--max-time", "20",
            "--cacert", str(ca), "--cert", str(certs / (identity["cert"] + ".crt")),
            "--key", str(certs / (identity["cert"] + ".key")), "-H", "Content-Type: application/json",
            "--data-binary", "@-", receiver], raw)
        (output / (label + "-sensor-ack.json")).write_text(sensor_raw)
        assert json.loads(sensor_raw).get("status") == "ACCEPTED", sensor_raw
        generation = int(sql("SELECT generation FROM platform.agent_sensor_assignment WHERE tenant_id='" +
                             TENANT + "' AND agent_id='" + identity["agent"] + "' AND status='ACTIVE'"))
        batch = core_batch(envelope, sensor, generation)
        (output / (label + "-core-request.json")).write_text(json.dumps(batch, indent=2))
        response = command(["docker", "exec", "-i", prefix + "-sensor-infra", "curl", "-sS",
            "--connect-timeout", "5", "--max-time", "20", "--cacert", "/certs/ca.crt",
            "--cert", "/certs/sensor-infra.crt", "--key", "/certs/sensor-infra.key",
            "-H", "Content-Type: application/json", "-H", "Idempotency-Key: " + batch["batchId"],
            "-w", "\n%{http_code}", "--data-binary", "@-", "https://core:8443/api/v1/sensor/agent-telemetry"],
            json.dumps(batch, separators=(",", ":")))
        body, status = response.rsplit("\n", 1)
        (output / (label + "-core-response.txt")).write_text(response)
        assert status == "409" and json.loads(body).get("code") == "ERR_AGENT_IDENTITY_CONFLICT", response
        watermark = sql("SELECT count(*) FROM platform.sensor_agent_ingress_watermark WHERE tenant_id='" +
                        TENANT + "' AND stream_id='" + batch["sensorAgentSpoolStreamId"] + "'")
        assert watermark == "0", watermark
        results.append(dict(case=label, authenticated_agent=identity, claimed_host=other["host"],
                            metric=metric, sensor_custody_ack="ACCEPTED", core_http=409,
                            core_code="ERR_AGENT_IDENTITY_CONFLICT", core_watermarks=0))
    (output / "identity-fixtures.json").write_text(json.dumps(dict(tenant=TENANT, fixtures=FIXTURES,
                                                                 results=results), indent=2) + "\n")


if __name__ == "__main__":
    main()
