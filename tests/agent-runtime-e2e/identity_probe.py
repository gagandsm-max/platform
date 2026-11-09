"""Both forged host claims receive Sensor custody and durable Core terminal settlement."""
import json
import os
from pathlib import Path
import subprocess
import sys
import time
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
        query = ("SELECT row_to_json(t) FROM (SELECT disposition_id, stream_id, first_sequence, "
                 "last_sequence, checksum, batch_id, agent_batch_id, agent_id, reason_code "
                 "FROM platform.sensor_agent_ingress_terminal_disposition WHERE tenant_id='" + TENANT +
                 "' AND sensor_id='" + sensor + "' AND agent_id='" + identity["agent"] +
                 "' AND agent_batch_id='" + envelope["batchId"] + "') t")
        terminal = None
        deadline = time.monotonic() + 90
        while time.monotonic() < deadline:
            rows = sql(query).splitlines()
            if len(rows) == 1:
                terminal = json.loads(rows[0])
                break
            time.sleep(1)
        assert terminal is not None, label + " did not receive Core terminal disposition"
        assert terminal["reason_code"] == "ERR_AGENT_IDENTITY_CONFLICT", terminal
        assert terminal["first_sequence"] == terminal["last_sequence"], terminal
        (output / (label + "-terminal-disposition.json")).write_text(json.dumps(terminal, indent=2) + "\n")
        watermark = sql("SELECT settled_through_sequence FROM platform.sensor_agent_ingress_watermark WHERE tenant_id='" +
                        TENANT + "' AND stream_id='" + terminal["stream_id"] + "'")
        assert int(watermark) >= terminal["last_sequence"], (watermark, terminal)
        results.append(dict(case=label, authenticated_agent=identity, claimed_host=other["host"],
                            metric=metric, sensor_custody_ack="ACCEPTED",
                            core_disposition="TERMINALLY_REJECTED", core_code="ERR_AGENT_IDENTITY_CONFLICT",
                            disposition_id=terminal["disposition_id"], settled_through=int(watermark)))
    (output / "identity-fixtures.json").write_text(json.dumps(dict(tenant=TENANT, fixtures=FIXTURES,
                                                                 results=results), indent=2) + "\n")


if __name__ == "__main__":
    main()
