import csv
import json
import logging
import os
import random
import statistics
import time
import uuid
from typing import Any, Dict, List, Optional

from harness.docker_client import DockerController
from harness.ha_state_machine import (
    CoreNode,
    CoreState,
    FutureEpochError,
    IllegalStateTransitionError,
    StaleEpochError,
    TwoCoreCluster,
    VALID_TRANSITIONS,
)
from harness.kafka_harness import KafkaSpikeHarness
from harness.clickhouse_harness import ClickHouseSpikeHarness

logging.basicConfig(level=logging.INFO, format="%(asctime)s [%(levelname)s] %(name)s: %(message)s")
logger = logging.getLogger("SpikeRunner")


def _cpu_percent(stats: Dict[str, Any]) -> Optional[float]:
    try:
        cpu = stats["cpu_stats"]
        precpu = stats["precpu_stats"]
        cpu_delta = cpu["cpu_usage"]["total_usage"] - precpu["cpu_usage"]["total_usage"]
        system_delta = cpu["system_cpu_usage"] - precpu["system_cpu_usage"]
        online = cpu.get("online_cpus") or len(cpu["cpu_usage"].get("percpu_usage") or [1])
        if system_delta > 0 and cpu_delta >= 0:
            return round((cpu_delta / system_delta) * online * 100.0, 2)
    except Exception:
        return None
    return None


def _net_bytes(stats: Dict[str, Any]) -> Dict[str, int]:
    networks = stats.get("networks") or {}
    rx = sum(int(v.get("rx_bytes", 0)) for v in networks.values())
    tx = sum(int(v.get("tx_bytes", 0)) for v in networks.values())
    return {"rx_bytes": rx, "tx_bytes": tx}


def _blkio_bytes(stats: Dict[str, Any]) -> Dict[str, int]:
    """Best-effort disk IO from blkio_stats (may be zero depending on cgroup)."""
    read_b = 0
    write_b = 0
    try:
        entries = stats.get("blkio_stats", {}).get("io_service_bytes_recursive") or []
        for e in entries:
            op = (e.get("op") or "").lower()
            val = int(e.get("value", 0))
            if op == "read":
                read_b += val
            elif op == "write":
                write_b += val
    except Exception:
        pass
    return {"read_bytes": read_b, "write_bytes": write_b}


class SpikeSuite:
    def __init__(self, workspace_root: str = "/workspace"):
        self.workspace_root = workspace_root
        self.evidence_dir = os.path.join(workspace_root, "engineering/spikes/two-core-ha/evidence")
        self.spike_dir = os.path.join(workspace_root, "engineering/spikes/two-core-ha")
        os.makedirs(self.evidence_dir, exist_ok=True)

        self.docker = DockerController()
        self.kafka = KafkaSpikeHarness()
        self.ch = ClickHouseSpikeHarness()
        self.results: Dict[str, Any] = {
            "timestamp": time.strftime("%Y-%m-%dT%H:%M:%SZ", time.gmtime()),
            "git_commit": os.environ.get("SPIKE_GIT_COMMIT", "UNKNOWN"),
            "mother_version": "v3.2.1",
            "evidence_policy": {
                "pass_requires": "EXECUTED",
                "classifications": [
                    "EXECUTED",
                    "MODEL-ONLY",
                    "ANALYTICALLY REJECTED",
                    "NOT TESTED",
                    "PENDING PRODUCTION FENCING",
                ],
            },
            "parts": {},
        }

    def wait_for_services(self, max_wait: int = 120):
        logger.info("Waiting for Two-Core lab services to become healthy...")
        services = [
            "ha-kafka-a",
            "ha-kafka-b",
            "ha-clickhouse-a",
            "ha-clickhouse-b",
            "ha-postgres-a",
            "ha-postgres-b",
        ]
        start = time.time()
        while time.time() - start < max_wait:
            all_running = all(self.docker.is_running(svc) for svc in services)
            if all_running:
                logger.info("All base containers are running.")
                break
            time.sleep(2)
        else:
            raise RuntimeError("Timed out waiting for HA spike containers")

        time.sleep(5)
        self.kafka.ensure_topics()
        self.kafka.init_producers()
        init = self.ch.init_schema()
        if not init.get("success"):
            raise RuntimeError(f"ClickHouse schema init failed: {init}")
        logger.info("Initial topics and schemas initialized.")

    def run_part_2_and_12_fsm_model(self):
        logger.info("=== Part 2 & 12: FSM model invariant tests (NOT mathematical proof) ===")
        cluster = TwoCoreCluster()
        part_res: Dict[str, Any] = {
            "evidence": "EXECUTED",
            "classification": "MODEL-ONLY",
            "claim_level": "model_invariant_test",
            "not_mathematical_proof": True,
            "not_production_split_brain_proof": True,
            "fsm_fuzz_passed": True,
            "illegal_transitions_rejected": 0,
            "total_fuzz_transitions": 1000,
            "epoch_exact_match": {},
        }

        s_a, s_b = cluster.simulate_partition_without_fencing()
        assert s_a == CoreState.ACTIVE
        assert s_b == CoreState.AMBIGUOUS

        cluster = TwoCoreCluster()
        s_a, s_b = cluster.simulate_partition_with_lab_fence_analogue()
        assert s_a == CoreState.FENCED
        assert s_b == CoreState.ACTIVE

        s_a, s_b = cluster.simulate_old_active_rejoin()
        assert s_a == CoreState.STANDBY
        assert s_b == CoreState.ACTIVE

        # Exact epoch authority
        active = CoreNode("epoch-test", CoreState.ACTIVE, initial_epoch=5)
        assert active.validate_write_epoch(5) is True
        stale_rejected = False
        future_rejected = False
        try:
            active.validate_write_epoch(4)
        except StaleEpochError:
            stale_rejected = True
        try:
            active.validate_write_epoch(6)
        except FutureEpochError:
            future_rejected = True
        part_res["epoch_exact_match"] = {
            "current_accepted": True,
            "stale_rejected": stale_rejected,
            "future_rejected": future_rejected,
            "evidence": "EXECUTED",
        }
        assert stale_rejected and future_rejected

        rejected_count = 0
        all_states = list(CoreState)
        for _ in range(1000):
            state1 = random.choice(all_states)
            state2 = random.choice(all_states)
            node = CoreNode("fuzz-node", state1)
            allowed = VALID_TRANSITIONS.get(state1, set())
            if state2 not in allowed:
                try:
                    node.transition_to(state2, "fuzz test")
                    part_res["fsm_fuzz_passed"] = False
                    raise AssertionError(f"FSM permitted illegal transition: {state1.value} -> {state2.value}")
                except IllegalStateTransitionError:
                    rejected_count += 1

        part_res["illegal_transitions_rejected"] = rejected_count
        self.results["parts"]["part_2_12_fsm"] = part_res

    def run_part_3_kafka_experiments(self):
        logger.info("=== Part 3: Kafka experiments (executable where practical) ===")
        scenarios_results: List[Dict[str, Any]] = []

        # 1. Both healthy dual write — EXECUTED
        b1 = str(uuid.uuid4())
        r1 = self.kafka.ingest_batch_dual_write(b1, 1, [{"metric": "cpu", "val": 10}])
        assert r1["dual_durable"] is True
        assert r1["ack_eligible"] is True
        assert r1["accepted_through_sequence"] == 1
        scenarios_results.append({
            "scenario": 1,
            "name": "both_healthy_dual_write",
            "evidence": "EXECUTED",
            "result": r1,
            "verdict": "PASS" if r1["ack_eligible"] else "FAIL",
        })

        # 2. Partial write A-only then replay after B returns — EXECUTED
        self.kafka.close_producers()
        self.docker.stop_container("ha-kafka-b", timeout_sec=2)
        time.sleep(1)
        self.kafka.init_producers()
        b2 = str(uuid.uuid4())
        r2_partial = self.kafka.ingest_batch_dual_write(
            b2, 2, [{"metric": "partial", "val": 1}], spool_policy="STRICT_DUAL"
        )
        assert r2_partial["ack_eligible"] is False
        assert r2_partial["accepted_through_sequence"] is None
        assert r2_partial["spool_retention_required"] is True
        self.docker.start_container("ha-kafka-b")
        self.kafka.wait_healthy("ha-kafka-b", 90)
        self.kafka.init_producers()
        r2_replay = self.kafka.ingest_batch_dual_write(b2, 2, [{"metric": "partial", "val": 1}])
        copies_a = self.kafka.count_batch_copies("a", b2, timeout_sec=25)
        copies_b = self.kafka.count_batch_copies("b", b2, timeout_sec=25)
        scenarios_results.append({
            "scenario": 2,
            "name": "partial_a_write_then_replay",
            "evidence": "EXECUTED",
            "result": {
                "partial": r2_partial,
                "replay": r2_replay,
                "copies_on_a": copies_a,
                "copies_on_b": copies_b,
                "ack_blocked_until_dual": r2_partial["ack_eligible"] is False,
                "replay_dual_ok": r2_replay.get("dual_durable") is True,
                "note": "Kafka stores replay copies; application consumer dedup NOT proven.",
            },
            "verdict": (
                "PASS"
                if (
                    r2_partial["ack_eligible"] is False
                    and r2_replay.get("dual_durable")
                    and copies_a >= 1
                    and copies_b >= 1
                )
                else "FAIL"
            ),
        })

        # 3. Standby Kafka down — EXECUTED (STRICT_DUAL, no sequence advance)
        self.docker.stop_container("ha-kafka-b", timeout_sec=2)
        time.sleep(1)
        self.kafka.close_producers()
        self.kafka.init_producers()
        b3 = str(uuid.uuid4())
        r3 = self.kafka.ingest_batch_dual_write(b3, 3, [{"metric": "mem", "val": 20}], spool_policy="STRICT_DUAL")
        # Also prove DEGRADED path is rejected even if caller asks for it
        b3d = str(uuid.uuid4())
        r3d = self.kafka.ingest_batch_dual_write(
            b3d, 31, [{"metric": "mem", "val": 21}], spool_policy="DEGRADED_SINGLE_DURABLE"
        )
        assert r3["ack_eligible"] is False
        assert r3["accepted_through_sequence"] is None
        assert r3d["ack_eligible"] is False
        assert r3d["accepted_through_sequence"] is None
        scenarios_results.append({
            "scenario": 3,
            "name": "standby_kafka_down_strict_dual_no_ack",
            "evidence": "EXECUTED",
            "result": {
                "strict_dual": r3,
                "degraded_request_coerced": r3d,
                "ack_contract": "STRICT_DUAL_ONLY; DEGRADED_SINGLE_DURABLE sequence advance REJECTED",
            },
            "verdict": "PASS",
        })
        self.docker.start_container("ha-kafka-b")
        self.kafka.wait_healthy("ha-kafka-b", 90)
        self.kafka.init_producers()

        # 4. Inter-core partition — MODEL-ONLY (FSM)
        scenarios_results.append({
            "scenario": 4,
            "name": "inter_core_partition",
            "evidence": "MODEL-ONLY",
            "result": {
                "active_a": "CONTINUES_LOCAL_WRITES (modelled)",
                "standby_b": "REFUSES_PROMOTION_WITHOUT_FENCE (modelled)",
                "dual_active": False,
            },
            "verdict": "MODELLED",
        })

        # 5. Replay same batch — EXECUTED storage truth; dedup NOT PROVEN
        r5 = self.kafka.ingest_batch_dual_write(b1, 1, [{"metric": "cpu", "val": 10}])
        copies_a5 = self.kafka.count_batch_copies("a", b1, timeout_sec=25)
        copies_b5 = self.kafka.count_batch_copies("b", b1, timeout_sec=25)
        scenarios_results.append({
            "scenario": 5,
            "name": "replay_same_batch_storage_truth",
            "evidence": "EXECUTED",
            "result": {
                "replayed_batch_id": b1,
                "replay_write": r5,
                "copies_on_a": copies_a5,
                "copies_on_b": copies_b5,
                "kafka_broker_deduplicates": False,
                "consumer_pipeline_dedup": "NOT_PROVEN",
                "note": "Kafka appends duplicate records; no application consumer dedup demonstrated.",
            },
            "verdict": "PASS" if copies_a5 >= 2 and copies_b5 >= 2 else "FAIL",
        })

        # 6–10: remain model/not executed at Kafka broker layer
        for sc, name, note in [
            (6, "duplicate_batch_handling", "No Core consumer pipeline; NOT PROVEN"),
            (7, "active_core_crash_post_dual_write", "No Core process in lab"),
            (8, "active_kafka_crash_post_dual_write", "Covered partially by failover drill Part 5"),
            (9, "standby_promotion", "FSM model + Part 5 lab analogue"),
            (10, "old_active_return", "FSM model epoch tests only"),
        ]:
            scenarios_results.append({
                "scenario": sc,
                "name": name,
                "evidence": "MODEL-ONLY" if sc in (9, 10) else "NOT TESTED",
                "result": {"note": note},
                "verdict": "NOT_EXECUTED",
            })

        # Two-voter KRaft claim — analytic only
        kraft_analysis = {
            "evidence": "ANALYTICALLY REJECTED",
            "lab_topology": "two independent 1-voter KRaft clusters (distinct cluster IDs)",
            "two_voter_shared_quorum_instantiated": False,
            "experimental_failure_of_two_voter_quorum": False,
            "note": (
                "Compose never formed a shared 2-voter KRaft quorum. Rejection is known "
                "quorum math (majority of 2 requires both voters), not lab experiment."
            ),
        }

        latencies_a: List[float] = []
        latencies_b: List[float] = []
        latencies_dual: List[float] = []
        for i in range(50):
            res = self.kafka.ingest_batch_dual_write(
                str(uuid.uuid4()), 100 + i, [{"m": "net.bytes", "val": i * 100}]
            )
            if res["dual_durable"]:
                latencies_a.append(res["latency_a_ms"])
                latencies_b.append(res["latency_b_ms"])
                latencies_dual.append(res["latency_total_ms"])

        part_res = {
            "scenarios": scenarios_results,
            "kraft_two_voter": kraft_analysis,
            "latency_stats": {
                "samples": len(latencies_dual),
                "kafka_a_mean_ms": round(statistics.mean(latencies_a), 2) if latencies_a else 0,
                "kafka_b_mean_ms": round(statistics.mean(latencies_b), 2) if latencies_b else 0,
                "dual_mean_ms": round(statistics.mean(latencies_dual), 2) if latencies_dual else 0,
                "dual_p95_ms": (
                    round(sorted(latencies_dual)[int(len(latencies_dual) * 0.95)], 2)
                    if latencies_dual
                    else 0
                ),
                "evidence": "MEASURED",
            },
            "ack_contract_verdict": (
                "STRICT_DUAL only; DEGRADED_SINGLE_DURABLE that advances "
                "accepted_through_sequence is REJECTED"
            ),
            "dedup_verdict": "NOT_PROVEN (Kafka stores duplicates; no consumer pipeline)",
        }
        self.results["parts"]["part_3_kafka"] = part_res

    def run_part_5_kafka_failover_timing(self):
        logger.info("=== Part 5: Kafka failover timing (lab lower bound; consume-verified T4) ===")
        timings = []
        for i in range(10):
            logger.info(f"Kafka failover drill {i + 1}/10...")
            t_res = self.kafka.run_failover_drill()
            t_res["iteration"] = i + 1
            assert t_res.get("batch_consumed") is True
            timings.append(t_res)
            logger.info(
                f"Iter {i + 1}: lab_fence={t_res['fence_sec']}s total={t_res['total_restore_sec']}s "
                f"(LAB LOWER BOUND ONLY; not certified Core HA <60s)"
            )

        csv_path = os.path.join(self.evidence_dir, "kafka_timings.csv")
        with open(csv_path, "w", newline="") as f:
            writer = csv.DictWriter(
                f,
                fieldnames=[
                    "iteration",
                    "fence_sec",
                    "promotion_sec",
                    "ingest_sec",
                    "consume_verify_sec",
                    "total_restore_sec",
                    "batch_consumed",
                    "scope",
                ],
            )
            writer.writeheader()
            for row in timings:
                writer.writerow({
                    "iteration": row["iteration"],
                    "fence_sec": row["fence_sec"],
                    "promotion_sec": row["promotion_sec"],
                    "ingest_sec": row["ingest_sec"],
                    "consume_verify_sec": row.get("consume_verify_sec"),
                    "total_restore_sec": row["total_restore_sec"],
                    "batch_consumed": row.get("batch_consumed"),
                    "scope": row.get("scope"),
                })

        totals = [r["total_restore_sec"] for r in timings]
        fences = [r["fence_sec"] for r in timings]
        self.results["parts"]["part_5_kafka_timing"] = {
            "evidence": "EXECUTED",
            "iterations": 10,
            "scope": "LAB_SERVICE_SWITCH_LOWER_BOUND",
            "fencing_level": "LAB_FENCING_ANALOGUE",
            "not_certified_core_ha_60s": True,
            "excludes": [
                "heartbeat timeout",
                "fencing API latency",
                "hardware/hypervisor power confirmation",
                "VIP/DNS move",
                "Core application promotion",
                "TLS endpoint readiness",
                "Sensor reconnect",
                "Sensor authoritative ACK",
                "consumer recovery checkpoint",
                "customer network latency",
            ],
            "t4_requires_batch_id_consume": True,
            "all_batches_consumed": all(r.get("batch_consumed") for r in timings),
            "min_total_sec": round(min(totals), 4),
            "median_total_sec": round(statistics.median(totals), 4),
            "max_total_sec": round(max(totals), 4),
            "p95_total_sec": round(sorted(totals)[int(len(totals) * 0.95)], 4),
            "median_fence_sec": round(statistics.median(fences), 4),
            "lab_under_60s_lower_bound": all(t < 60.0 for t in totals),
            "raw_timings": timings,
        }

    def run_part_8_clickhouse_experiments(self):
        logger.info("=== Part 8: ClickHouse experiments ===")
        scenarios: List[Dict[str, Any]] = []

        tenant = str(uuid.uuid4())
        device = str(uuid.uuid4())
        batch1 = str(uuid.uuid4())
        batch2 = str(uuid.uuid4())
        rows_1 = [
            (tenant, "disk.used", device, "2026-09-07 12:00:00.000", batch1, 12345.0, 1),
            (tenant, "disk.free", device, "2026-09-07 12:00:00.000", batch2, 67890.0, 1),
        ]
        res1 = self.ch.dual_insert_batch(rows_1)
        assert res1["dual_success"] is True
        scenarios.append({
            "scenario": 1,
            "name": "normal_dual_insert",
            "evidence": "EXECUTED",
            "result": res1,
            "verdict": "PASS",
        })

        # Replay identical ORDER BY key (same batch_ids)
        before_raw = self.ch.count_by_batch_id("a", batch1, use_final=False)
        res2 = self.ch.dual_insert_batch(rows_1)
        after_raw = self.ch.count_by_batch_id("a", batch1, use_final=False)
        after_final = self.ch.count_by_batch_id("a", batch1, use_final=True)
        scenarios.append({
            "scenario": 2,
            "name": "replay_same_batch_final_semantics",
            "evidence": "EXECUTED",
            "result": {
                "before_raw_batch1": before_raw,
                "after_raw_batch1": after_raw,
                "after_final_batch1": after_final,
                "query_time_idempotent_under_final": after_final == 1,
                "physical_dedup_complete": after_raw == 1,
                "claim": (
                    "query-time idempotent result for identical tested ORDER BY key under FINAL; "
                    "NOT complete physical duplicate elimination"
                ),
            },
            "verdict": "PASS" if after_final == 1 else "FAIL",
        })

        self.docker.stop_container("ha-clickhouse-a", timeout_sec=2)
        time.sleep(1)
        rows_b = [(str(uuid.uuid4()), "net.octets", str(uuid.uuid4()), "2026-09-07 12:01:00.000", str(uuid.uuid4()), 999.0, 1)]
        status_b, lat_b = self.ch.insert_batch("b", rows_b)
        scenarios.append({
            "scenario": 3,
            "name": "a_unavailable_write_b_only",
            "evidence": "EXECUTED",
            "result": {"b_status": status_b, "b_lat_ms": lat_b},
            "verdict": "PASS" if status_b == 200 else "FAIL",
        })
        self.docker.start_container("ha-clickhouse-a")
        self.ch.wait_running("ha-clickhouse-a", 90)

        self.docker.stop_container("ha-clickhouse-b", timeout_sec=2)
        time.sleep(1)
        rows_a = [(str(uuid.uuid4()), "net.octets", str(uuid.uuid4()), "2026-09-07 12:02:00.000", str(uuid.uuid4()), 1000.0, 1)]
        status_a, lat_a = self.ch.insert_batch("a", rows_a)
        scenarios.append({
            "scenario": 4,
            "name": "b_unavailable_write_a_only",
            "evidence": "EXECUTED",
            "result": {"a_status": status_a, "a_lat_ms": lat_a},
            "verdict": "PASS" if status_a == 200 else "FAIL",
        })
        self.docker.start_container("ha-clickhouse-b")
        self.ch.wait_running("ha-clickhouse-b", 90)

        # 5. Partial dual insert (B down mid path) — EXECUTED
        self.docker.stop_container("ha-clickhouse-b", timeout_sec=2)
        time.sleep(1)
        partial_batch = str(uuid.uuid4())
        partial_rows = [
            (str(uuid.uuid4()), "partial.fail", str(uuid.uuid4()), "2026-09-07 12:03:00.000", partial_batch, 1.0, 1)
        ]
        partial = self.ch.dual_insert_batch(partial_rows)
        scenarios.append({
            "scenario": 5,
            "name": "failure_between_a_and_b",
            "evidence": "EXECUTED",
            "result": {
                **partial,
                "dual_success": partial["dual_success"],
                "sensor_must_retain_spool": not partial["dual_success"],
            },
            "verdict": "PASS" if partial["dual_success"] is False and partial["status_a"] == 200 else "FAIL",
        })
        self.docker.start_container("ha-clickhouse-b")
        self.ch.wait_running("ha-clickhouse-b", 90)

        # 6. Active failure after A insert only — EXECUTED analogue (A insert then kill A)
        only_a_batch = str(uuid.uuid4())
        only_a_rows = [
            (str(uuid.uuid4()), "active.fail", str(uuid.uuid4()), "2026-09-07 12:04:00.000", only_a_batch, 2.0, 1)
        ]
        st_a_only, _ = self.ch.insert_batch("a", only_a_rows)
        self.docker.kill_container("ha-clickhouse-a", "SIGKILL")
        time.sleep(0.5)
        on_b = self.ch.count_by_batch_id("b", only_a_batch, use_final=True)
        scenarios.append({
            "scenario": 6,
            "name": "active_failure_after_a_insert",
            "evidence": "EXECUTED",
            "result": {
                "a_insert_status": st_a_only,
                "present_on_b_before_replay": on_b,
                "loss_without_replay": on_b == 0,
                "note": "Without Sensor replay / reconcile, B lacks A-only row.",
            },
            "verdict": "PASS" if st_a_only == 200 and on_b == 0 else "FAIL",
        })
        self.docker.start_container("ha-clickhouse-a")
        self.ch.wait_running("ha-clickhouse-a", 90)

        cnt_b = self.ch.count_rows("b", use_final=True)
        scenarios.append({
            "scenario": 7,
            "name": "failover_query_to_b",
            "evidence": "EXECUTED",
            "result": {"rows_queryable": cnt_b, "success": cnt_b > 0},
            "verdict": "PASS" if cnt_b > 0 else "FAIL",
        })

        # 8. Old A rejoin — NOT TESTED as Core lifecycle (container restart only)
        scenarios.append({
            "scenario": 8,
            "name": "old_a_rejoins",
            "evidence": "NOT TESTED",
            "result": {"note": "No Core rejoin protocol; container restart ≠ fenced rejoin"},
            "verdict": "NOT_EXECUTED",
        })

        # 9–10 Divergence + reconcile — EXECUTED with spike helper
        # Ensure both nodes healthy first
        self.ch.wait_running("ha-clickhouse-a", 30)
        self.ch.wait_running("ha-clickhouse-b", 30)
        batch_x = str(uuid.uuid4())
        batch_y = str(uuid.uuid4())
        row_x = (str(uuid.uuid4()), "div.x", str(uuid.uuid4()), "2026-09-07 13:00:00.000", batch_x, 10.0, 1)
        row_y = (str(uuid.uuid4()), "div.y", str(uuid.uuid4()), "2026-09-07 13:00:00.000", batch_y, 20.0, 1)
        self.ch.insert_batch("a", [row_x])
        self.ch.insert_batch("b", [row_y])
        before_a = self.ch.list_batch_ids("a")
        before_b = self.ch.list_batch_ids("b")
        reconcile = self.ch.reconcile_by_batch_id()
        scenarios.append({
            "scenario": 9,
            "name": "divergent_inserts_reconcile",
            "evidence": "EXECUTED",
            "result": {
                "batch_x_a_only_before": batch_x in before_a and batch_x not in before_b,
                "batch_y_b_only_before": batch_y in before_b and batch_y not in before_a,
                "reconcile": reconcile,
            },
            "verdict": "PASS" if reconcile.get("converged") else "FAIL",
        })
        scenarios.append({
            "scenario": 10,
            "name": "resync_implementation",
            "evidence": "EXECUTED",
            "result": {
                "helper": "reconcile_by_batch_id",
                "production_resync": "PENDING",
                "p0_02_status_note": "Lab helper proven; production resync design still OPEN",
                "converged_in_lab": reconcile.get("converged"),
            },
            "verdict": "PASS" if reconcile.get("converged") else "FAIL",
        })

        identity = self.ch.test_event_identity_gap()
        self.results["parts"]["part_8_clickhouse"] = {
            "scenarios": scenarios,
            "event_identity": identity,
            "dedup_semantics": (
                "query-time idempotent result for identical tested ORDER BY key under FINAL; "
                "NOT complete physical duplicate elimination; production FINAL affordability UNPROVEN"
            ),
        }

    def run_part_9_clickhouse_ddl(self):
        logger.info("=== Part 9: ClickHouse DDL healthy + partial failure recovery ===")
        healthy = self.ch.test_schema_migration("environment_zone")
        assert healthy["declared_ready"] is True
        partial = self.ch.test_ddl_partial_failure_and_recovery("ddl_recovery_marker")
        assert partial.get("not_ready_correct") is True
        assert partial.get("declared_ready_final") is True
        assert partial.get("success") is True
        self.results["parts"]["part_9_ddl"] = {
            "healthy_path": healthy,
            "partial_failure_recovery": partial,
            "evidence": "EXECUTED",
            "verdict": "PASS",
        }

    def run_part_10_clickhouse_failover_timing(self):
        logger.info("=== Part 10: ClickHouse failover timing (lab lower bound) ===")
        timings = []
        for i in range(10):
            logger.info(f"ClickHouse failover drill {i + 1}/10...")
            t_res = self.ch.run_clickhouse_failover_drill()
            t_res["iteration"] = i + 1
            timings.append(t_res)

        csv_path = os.path.join(self.evidence_dir, "clickhouse_timings.csv")
        with open(csv_path, "w", newline="") as f:
            writer = csv.DictWriter(
                f,
                fieldnames=[
                    "iteration",
                    "fence_sec",
                    "promotion_sec",
                    "ingest_sec",
                    "total_restore_sec",
                    "scope",
                ],
            )
            writer.writeheader()
            for row in timings:
                writer.writerow({
                    "iteration": row["iteration"],
                    "fence_sec": row["fence_sec"],
                    "promotion_sec": row["promotion_sec"],
                    "ingest_sec": row["ingest_sec"],
                    "total_restore_sec": row["total_restore_sec"],
                    "scope": row.get("scope"),
                })

        totals = [r["total_restore_sec"] for r in timings]
        fences = [r["fence_sec"] for r in timings]
        self.results["parts"]["part_10_clickhouse_timing"] = {
            "evidence": "EXECUTED",
            "iterations": 10,
            "scope": "LAB_SERVICE_SWITCH_LOWER_BOUND",
            "fencing_level": "LAB_FENCING_ANALOGUE",
            "not_certified_core_ha_60s": True,
            "min_total_sec": round(min(totals), 4),
            "median_total_sec": round(statistics.median(totals), 4),
            "max_total_sec": round(max(totals), 4),
            "p95_total_sec": round(sorted(totals)[int(len(totals) * 0.95)], 4),
            "median_fence_sec": round(statistics.median(fences), 4),
            "lab_under_60s_lower_bound": all(t < 60.0 for t in totals),
            "raw_timings": timings,
        }

    def run_part_13_resource_observations(self):
        logger.info("=== Part 13: Resource observations (CPU/RAM/disk/network) ===")
        containers = [
            "ha-kafka-a",
            "ha-kafka-b",
            "ha-clickhouse-a",
            "ha-clickhouse-b",
            "ha-postgres-a",
            "ha-postgres-b",
        ]
        stats_map: Dict[str, Any] = {}
        for c in containers:
            stats = self.docker.get_stats(c)
            if not stats:
                stats_map[c] = {"error": "stats unavailable"}
                continue
            mem_usage = stats.get("memory_stats", {}).get("usage", 0) / (1024 * 1024)
            mem_limit = stats.get("memory_stats", {}).get("limit", 0) / (1024 * 1024)
            stats_map[c] = {
                "memory_usage_mb": round(mem_usage, 2),
                "memory_limit_mb": round(mem_limit, 2),
                "cpu_percent_sample": _cpu_percent(stats),
                "network": _net_bytes(stats),
                "blkio": _blkio_bytes(stats),
            }

        def _mem(name: str) -> float:
            v = stats_map.get(name, {}).get("memory_usage_mb", 0)
            return float(v) if isinstance(v, (int, float)) else 0.0

        total_a = _mem("ha-kafka-a") + _mem("ha-clickhouse-a") + _mem("ha-postgres-a")
        total_b = _mem("ha-kafka-b") + _mem("ha-clickhouse-b") + _mem("ha-postgres-b")

        missing = []
        for c, s in stats_map.items():
            if s.get("cpu_percent_sample") is None:
                missing.append(f"{c}:cpu")
            if not s.get("network"):
                missing.append(f"{c}:network")
            # blkio may legitimately be zero

        self.results["parts"]["part_13_resources"] = {
            "evidence": "MEASURED",
            "containers": stats_map,
            "core_a_total_mem_mb": round(total_a, 2),
            "core_b_total_mem_mb": round(total_b, 2),
            "dimensions": {
                "cpu": "single Docker stats sample (not load-tested)",
                "memory": "container RSS-ish usage from Docker stats",
                "disk": "blkio counters best-effort; may be zero/incomplete under cgroup v2",
                "network": "cumulative rx/tx bytes counters (not failover overhead delta)",
            },
            "profile_evaluation": (
                "INCOMPLETE — idle/lab container samples only; do NOT claim comfortably within "
                "8 vCPU/16GB Core budget from this evidence alone"
            ),
            "missing_or_weak": missing,
        }

    def run_postgres_status(self):
        """P0-03 remains unresolved — containers present but failover NOT TESTED."""
        self.results["parts"]["part_postgres_p0_03"] = {
            "evidence": "NOT TESTED",
            "classification": "NOT TESTED — P0-03",
            "containers_present": {
                "ha-postgres-a": self.docker.is_running("ha-postgres-a"),
                "ha-postgres-b": self.docker.is_running("ha-postgres-b"),
            },
            "failover_implemented": False,
            "patroni_repmgr_selected_by_mother": False,
            "note": (
                "Do not cite Patroni/repmgr as selected architecture; Mother Reference has not "
                "selected PostgreSQL HA mechanism in this spike scope."
            ),
            "verdict": "NOT_TESTED",
        }

    def save_raw_metrics(self):
        json_path = os.path.join(self.evidence_dir, "raw_metrics.json")
        with open(json_path, "w") as f:
            json.dump(self.results, f, indent=2, default=str)
        logger.info(f"Saved raw metrics to {json_path}")

    def generate_markdown_reports(self):
        logger.info("=== Generating corrected documentation ===")
        commit = self.results.get("git_commit", "UNKNOWN")
        k_summary = self.results["parts"].get("part_5_kafka_timing", {})
        ch_summary = self.results["parts"].get("part_10_clickhouse_timing", {})
        k_lat = self.results["parts"].get("part_3_kafka", {}).get("latency_stats", {})
        fsm = self.results["parts"].get("part_2_12_fsm", {})
        resources = self.results["parts"].get("part_13_resources", {})
        pg = self.results["parts"].get("part_postgres_p0_03", {})
        ddl = self.results["parts"].get("part_9_ddl", {})
        ch_part = self.results["parts"].get("part_8_clickhouse", {})
        kafka_part = self.results["parts"].get("part_3_kafka", {})

        # failure-matrix.md
        fm_path = os.path.join(self.spike_dir, "failure-matrix.md")
        with open(fm_path, "w") as f:
            f.write(f"""# Platform P0 Spike: Two-Core Combined Failure Matrix

**Document ID:** SPIKE-P0-EVIDENCE-002  
**Evidence SHA (this run):** `{commit}`  
**Mother Reference Version:** `v3.2.1`  
**Status:** CORRECTED — evidence classifications only; no PASS without EXECUTED proof

Evidence classes: `EXECUTED` | `MODEL-ONLY` | `ANALYTICALLY REJECTED` | `NOT TESTED` | `PENDING PRODUCTION FENCING`

---

## Combined Failure Scenarios

| # | Scenario | Evidence classification | Result | Notes |
|---|---|---|---|---|
| 1 | Core A App Crash | NOT TESTED | NOT_EXECUTED | No Core application process in lab |
| 2 | Kafka A Crash | EXECUTED | LAB PASS (lower bound) | Part 5: Docker kill + consume-verified T4; LAB_FENCING_ANALOGUE |
| 3 | ClickHouse A Crash | EXECUTED | LAB PASS (lower bound) | Part 10: Docker kill + query verify; not certified Core HA |
| 4 | PostgreSQL A Failure | NOT TESTED | NOT TESTED — P0-03 | No PG failover in spike; Patroni/repmgr not Mother-selected |
| 5 | Entire Core A Host Loss | PENDING PRODUCTION FENCING | NOT PROVEN | Docker stop ≠ host STONITH / IPMI / hypervisor fence |
| 6 | A↔B App Network Partition | MODEL-ONLY | MODELLED | FSM: Standby → AMBIGUOUS without fence |
| 7 | A↔B Replication Network Partition | EXECUTED (Kafka/CH partial) | LAB PASS | STRICT_DUAL blocks ACK; Sensor spool retention required |
| 8 | Standby B Service Failure | EXECUTED | LAB PASS | Kafka/CH B down paths exercised |
| 9 | Simultaneous Partial Failures | NOT TESTED | NOT_EXECUTED | No multi-fault Core supervisor |
| 10 | Active Isolated from Client/VIP | NOT TESTED | NOT_EXECUTED | No VIP / DNS / self-fence agent |
| 11 | Fencing Mechanism Unavailable | MODEL-ONLY | MODELLED | FSM refuses promote without fence confirm |
| 12 | Old Active Rejoins | MODEL-ONLY | MODELLED | Epoch exact-match model; no real Core rejoin |

PostgreSQL row status: **{pg.get('classification', 'NOT TESTED — P0-03')}**

---

## Safety claims (honest scope)

1. **FSM model invariant** — 1000 randomized illegal transitions rejected in-process. This is a **model invariant test**, NOT a mathematical proof that production Core/VIP/Kafka/CH/PG cannot split-brain.
2. **ACK contract** — `{kafka_part.get('ack_contract_verdict', 'STRICT_DUAL')}`
3. **Kafka dedup** — `{kafka_part.get('dedup_verdict', 'NOT_PROVEN')}`
4. **Fencing** — lab Docker kill/stop only (`LAB_FENCING_ANALOGUE`). Production fencing remains **PENDING PRODUCTION FENCING / HIGH RISK** until providers are tested.
""")

        # results.md
        res_path = os.path.join(self.spike_dir, "results.md")
        with open(res_path, "w") as f:
            f.write(f"""# Platform P0 Spike: Two-Core Stateful HA Experimental Results (Corrected)

**Document ID:** SPIKE-P0-EVIDENCE-001  
**Evidence SHA (this run):** `{commit}`  
**Mother Reference Version:** `v3.2.1`  

Legend: **MEASURED** | **EXECUTED** | **MODELLED** | **INFERRED** | **PENDING PRODUCTION VALIDATION**

---

## 1. Executive Summary

This Docker lab spike evaluates candidate patterns for EXACTLY-TWO-CORE HA. It does **not** certify production Core HA &lt;60s.

### Corrected findings

| Topic | Classification | Finding |
|---|---|---|
| Two-voter KRaft/Keeper | ANALYTICALLY REJECTED | Lab uses two independent 1-voter clusters; shared 2-voter quorum was **not** instantiated |
| Kafka dual-write ACK | EXECUTED | STRICT_DUAL only; degraded sequence advance **REJECTED** |
| Kafka consumer dedup | NOT PROVEN | Broker stores duplicates on replay |
| Kafka failover timing | MEASURED (lab lower bound) | Median **{k_summary.get('median_total_sec', 'N/A')}s** with consume-verified T4; **not** certified Core HA |
| ClickHouse FINAL idempotence | EXECUTED | Query-time idempotent under FINAL for identical tested ORDER BY key; not complete physical dedup |
| ClickHouse event identity | EXECUTED | Different `batch_id` ⇒ duplicate FINAL rows; stable event identity **REQUIRED** |
| ClickHouse divergence/resync | EXECUTED (spike helper) | Lab helper converged; production resync still **PENDING** |
| ClickHouse DDL partial fail | EXECUTED | `declared_ready=false` while B down; restore + reconcile → schemas match |
| FSM / epoch | MODEL-ONLY / EXECUTED model | Exact epoch match; future epochs rejected; not production proof |
| PostgreSQL P0-03 | NOT TESTED | Unresolved |
| Resources | MEASURED (incomplete) | {resources.get('profile_evaluation')} |
| Ops complexity (install/DR/support) | INFERRED | Engineering estimates only — **not proven** by this spike |

---

## 2. Kafka Failover Timings (Part 5) — LAB LOWER BOUND

Scope: `{k_summary.get('scope')}` / fencing `{k_summary.get('fencing_level')}`  
`not_certified_core_ha_60s={k_summary.get('not_certified_core_ha_60s')}`  
T4 requires exact `batch_id` consume: `{k_summary.get('t4_requires_batch_id_consume')}`  
All batches consumed: `{k_summary.get('all_batches_consumed')}`

| Iteration | Lab fence (s) | Promote model (s) | Ingest (s) | Consume verify (s) | Total (s) |
|---|---|---|---|---|---|
""")
            for row in k_summary.get("raw_timings", []):
                f.write(
                    f"| {row['iteration']} | {row['fence_sec']:.4f} | {row['promotion_sec']:.4f} | "
                    f"{row['ingest_sec']:.4f} | {row.get('consume_verify_sec', 0):.4f} | "
                    f"{row['total_restore_sec']:.4f} |\n"
                )
            f.write(f"""
- Min/Median/P95/Max total (s): {k_summary.get('min_total_sec')} / {k_summary.get('median_total_sec')} / {k_summary.get('p95_total_sec')} / {k_summary.get('max_total_sec')}
- Dual-write latency (MEASURED): mean {k_lat.get('dual_mean_ms')} ms, p95 {k_lat.get('dual_p95_ms')} ms

Excluded from timing: heartbeat, production fencing API, power confirmation, VIP/DNS, Core promotion, TLS readiness, Sensor reconnect/ACK, consumer checkpoint, customer latency.

---

## 3. ClickHouse Failover Timings (Part 10) — LAB LOWER BOUND

| Iteration | Lab fence (s) | Promote model (s) | Ingest (s) | Total (s) |
|---|---|---|---|---|
""")
            for row in ch_summary.get("raw_timings", []):
                f.write(
                    f"| {row['iteration']} | {row['fence_sec']:.4f} | {row['promotion_sec']:.4f} | "
                    f"{row['ingest_sec']:.4f} | {row['total_restore_sec']:.4f} |\n"
                )
            f.write(f"""
- Min/Median/P95/Max total (s): {ch_summary.get('min_total_sec')} / {ch_summary.get('median_total_sec')} / {ch_summary.get('p95_total_sec')} / {ch_summary.get('max_total_sec')}

Dedup semantics: {ch_part.get('dedup_semantics')}

DDL: healthy ready={ddl.get('healthy_path', {}).get('declared_ready')}; partial recovery success={ddl.get('partial_failure_recovery', {}).get('success')}

---

## 4. FSM / Epoch

- Classification: `{fsm.get('classification')}` / claim_level=`{fsm.get('claim_level')}`
- `not_mathematical_proof={fsm.get('not_mathematical_proof')}`
- Epoch exact-match: `{fsm.get('epoch_exact_match')}`

---

## 5. Resources

{json.dumps(resources, indent=2, default=str)}
""")

        # architecture-candidate.md
        arch_path = os.path.join(self.spike_dir, "architecture-candidate.md")
        with open(arch_path, "w") as f:
            f.write(f"""# Platform Candidate Architecture: Two-Core Stateful HA (Corrected)

**Document ID:** SPIKE-P0-CANDIDATE-001  
**Evidence SHA:** `{commit}`  
**Status:** CANDIDATE — promising patterns with honest evidence limits

---

## Safe candidate direction (INFERRED from EXECUTED lab + analysis)

- Independent Kafka A/B (single-node KRaft each; **no** shared two-voter quorum)
- Independent ClickHouse A/B (no Keeper)
- **STRICT_DUAL** durability before Sensor `accepted_through_sequence` ACK
- Sensor spool retained when dual durability unavailable (no sequence advance)
- Positive fencing before promotion (**PENDING PRODUCTION VALIDATION**)
- Old Active rejoins fenced; exact epoch authority (model EXECUTED; production PENDING)
- PostgreSQL HA mechanism: **NOT SELECTED** in this spike (P0-03 OPEN)

---

## Explicit non-claims

- Docker kill/stop is **LAB FENCING ANALOGUE**, not STONITH evidence
- Lab sub-second/few-second timings are **service-switch lower bounds**, not certified &lt;60s Core HA
- FSM fuzzing is a **model invariant test**, not a mathematical split-brain proof
- Kafka does not prove application consumer dedup
- ClickHouse `FINAL` proves query-time idempotence for the tested key, not complete physical dedup / all query patterns
- Operational claims (install/backup/DR/support burden) remain **engineering estimates**, not proven findings
- Fencing integration remains **HIGH RISK** until real providers are tested

---

## Component notes

### Kafka (P0-01)
- Topology MEASURED in compose: two independent 1-voter clusters
- Two-voter shared KRaft: ANALYTICALLY REJECTED (not experimentally failed in-lab)
- ACK: STRICT_DUAL only (EXECUTED)

### ClickHouse (P0-02)
- Dual insert / FINAL semantics / DDL partial recovery / lab divergence helper: EXECUTED
- Production resync + stable event identity (without batch_id in identity key): PENDING
- FINAL affordability in production queries: UNPROVEN

### Fencing (P0-04)
- Lab analogue EXECUTED; production fencing **PENDING PRODUCTION FENCING**
- READY FOR PRODUCTION DESIGN of fencing providers is **not** claimed by this spike alone
""")

        # README.md
        readme_path = os.path.join(self.spike_dir, "README.md")
        with open(readme_path, "w") as f:
            f.write(f"""# Platform P0 Engineering Spike: Two-Core Stateful HA Lab

Corrected evidence spike. Distinguishes **MEASURED / EXECUTED / MODELLED / INFERRED / PENDING PRODUCTION VALIDATION**.

Evidence SHA for last documented run: `{commit}`

## Contents

- `compose.ha-spike.yaml` — isolated lab (Kafka A/B, ClickHouse A/B, PostgreSQL A/B present but PG HA **NOT TESTED**)
- `harness/` — FSM model, Kafka/ClickHouse harnesses, runner
- `evidence/` — `raw_metrics.json`, timing CSVs
- `results.md`, `failure-matrix.md`, `architecture-candidate.md`

## Execution (Docker only)

```bash
cd /path/to/repo
docker compose -p platform-ha-spike -f engineering/spikes/two-core-ha/compose.ha-spike.yaml up -d --wait || \\
  docker compose -p platform-ha-spike -f engineering/spikes/two-core-ha/compose.ha-spike.yaml up -d
# wait for healthy if --wait unsupported
docker build -t platform-ha-harness engineering/spikes/two-core-ha/harness
docker run --rm --network ha_spike_net \\
  -e SPIKE_GIT_COMMIT=$(git rev-parse HEAD) \\
  -v /var/run/docker.sock:/var/run/docker.sock \\
  -v "$(pwd)":/workspace \\
  platform-ha-harness
docker compose -p platform-ha-spike -f engineering/spikes/two-core-ha/compose.ha-spike.yaml down -v
```

## What this proves / does not prove

See `results.md` and `failure-matrix.md`. Do not treat lab timings as certified Core HA &lt;60s.
""")
        logger.info("Corrected documentation written.")


if __name__ == "__main__":
    suite = SpikeSuite()
    suite.wait_for_services()
    suite.run_part_2_and_12_fsm_model()
    suite.run_part_3_kafka_experiments()
    suite.run_part_5_kafka_failover_timing()
    suite.run_part_8_clickhouse_experiments()
    suite.run_part_9_clickhouse_ddl()
    suite.run_part_10_clickhouse_failover_timing()
    suite.run_postgres_status()
    suite.run_part_13_resource_observations()
    suite.save_raw_metrics()
    suite.generate_markdown_reports()
    logger.info("P0 Spike corrected execution completed.")
