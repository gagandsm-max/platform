"""Compare a real Agent Kafka observation with the existing ClickHouse contract."""
import json
from pathlib import Path
import sys

FIELDS = {"tenant_id": "tenantId", "entity_id": "entityId", "metric_id": "metricId", "sensor_id": "sensorId",
          "value": "value", "sequence_id": "sequenceId", "source_protocol": "sourceProtocol"}


def expected_observation(text, agent_id):
    for line in text.splitlines():
        try:
            obj = json.loads(line.split("|", 1)[-1])
        except ValueError:
            continue
        if obj.get("agentId") != agent_id:
            continue
        result = {dest: obj[src] for dest, src in FIELDS.items()}
        stamp = obj["timestamp"]
        if not stamp.endswith("Z"):
            raise ValueError("expected canonical UTC timestamp")
        second, _, fraction = stamp[:-1].partition(".")
        result["ts"] = second.replace("T", " ") + "." + (fraction + "000")[:3]
        return {"stored_fields": result, "kafka_observation": obj,
                "timestamp_exact_at_storage_precision": not fraction[3:].strip("0")}
    raise ValueError("No real Agent observation in retained Kafka output")


def verify(expected, text):
    rows = [json.loads(line) for line in text.splitlines() if line.strip()]
    if len(rows) != 1 or rows[0] != expected["stored_fields"]:
        raise ValueError(f"Canonical readback differs: expected={expected['stored_fields']} actual={rows}")


def entity_binding_result(text, metric, tenant, agent, entity):
    rows = []
    for line in text.splitlines():
        try:
            row = json.loads(line.split("|", 1)[-1])
        except ValueError:
            continue
        if row.get("metricId") == metric:
            rows.append(row)
    # Absence cannot prove rejection. A server may safely ignore the claimed
    # host and use the authenticated Agent's entity instead of rejecting it.
    status = "NOT_RUN"
    if rows:
        status = "PASS" if all(row.get("tenantId") == tenant and row.get("agentId") == agent
                               and row.get("entityId") == entity for row in rows) else "FAIL"
    return {"status": status, "expected_tenant": tenant, "expected_agent": agent,
            "expected_entity": entity, "observed": rows}


def verify_health(ready, health, http_code, queue_count, outage):
    if not isinstance(ready, dict) or not isinstance(health, dict):
        return False
    if http_code != 200 or ready.get("ready") is not True or ready.get("http_semantic") != 200:
        return False
    expected = "DEGRADED" if outage else "UP"
    if ready.get("status") != expected or health.get("component_state") != expected:
        return False
    transport = health.get("dependency_status", {}).get("sensor_transport")
    if outage:
        return (queue_count > 0 and health.get("queue_depth", 0) > 0 and transport == "DOWN"
                and health.get("delivery_status") == "FAILED" and bool(health.get("reason_codes")))
    return transport == "UP" and health.get("delivery_status") in ("OK", "BACKLOG")


if __name__ == "__main__":
    action, source, target = sys.argv[1:4]
    if action == "prepare":
        expected = expected_observation(Path(source).read_text(), "agent-e2e-01")
        Path(target).write_text(json.dumps(expected, indent=2) + "\n")
        fields = expected["stored_fields"]
        predicates = " AND ".join(key + "='" + str(fields[key]).replace("'", "''") + "'" for key in ("tenant_id", "sequence_id"))
        print("SELECT tenant_id, entity_id, metric_id, sensor_id, toString(ts) AS ts, value, sequence_id, source_protocol FROM platform.metric_numeric WHERE " + predicates + " FORMAT JSONEachRow")
    elif action == "verify":
        verify(json.loads(Path(source).read_text()), Path(target).read_text())
    elif action == "entity-binding":
        result = entity_binding_result(Path(source).read_text(), *sys.argv[4:8])
        Path(target).write_text(json.dumps(result, indent=2) + "\n")
        print(result["status"])
    elif action in ("health-outage", "health-recovered"):
        valid = verify_health(json.loads(Path(source).read_text()), json.loads(Path(target).read_text()),
                              int(sys.argv[4]), int(sys.argv[5]), action == "health-outage")
        raise SystemExit(0 if valid else 1)
    else:
        raise SystemExit("Unknown readback operation")
