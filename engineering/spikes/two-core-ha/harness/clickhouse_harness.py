import logging
import time
import urllib.error
import urllib.parse
import urllib.request
import uuid
from typing import Any, Dict, List, Optional, Set, Tuple

from harness.docker_client import DockerController
from harness.ha_state_machine import CoreNode, CoreState

logger = logging.getLogger("ClickHouseHarness")


class ClickHouseSpikeHarness:
    def __init__(
        self,
        url_a: str = "http://ha-clickhouse-a:8123",
        url_b: str = "http://ha-clickhouse-b:8123",
        database: str = "platform",
        user: str = "platform_app",
        password: str = "platform_dev_ch_password_change_in_prod",
    ):
        self.url_a = url_a
        self.url_b = url_b
        self.database = database
        self.user = user
        self.password = password
        self.docker = DockerController()

    def query(self, target: str, sql: str, timeout: int = 10) -> Tuple[int, str]:
        base_url = self.url_a if target == "a" else self.url_b
        params = {
            "database": self.database,
            "user": self.user,
            "password": self.password,
            "query": sql,
        }
        full_url = f"{base_url}/?{urllib.parse.urlencode(params)}"
        req = urllib.request.Request(full_url, method="POST")
        try:
            with urllib.request.urlopen(req, timeout=timeout) as resp:
                return resp.status, resp.read().decode("utf-8")
        except urllib.error.HTTPError as e:
            return e.code, e.read().decode("utf-8")
        except Exception as e:
            return 500, str(e)

    def init_schema(self) -> Dict[str, Any]:
        # Ensure database exists
        for target in ("a", "b"):
            self.query(target, "CREATE DATABASE IF NOT EXISTS platform")

        ddl = """
        CREATE TABLE IF NOT EXISTS platform.telemetry_metrics (
            tenant_id UUID,
            metric_name LowCardinality(String),
            device_id UUID,
            timestamp DateTime64(3),
            batch_id UUID,
            value Float64,
            version UInt64
        ) ENGINE = ReplacingMergeTree(version)
        ORDER BY (tenant_id, metric_name, device_id, timestamp, batch_id)
        """
        status_a, resp_a = self.query("a", ddl)
        status_b, resp_b = self.query("b", ddl)
        return {
            "node_a_status": status_a,
            "node_a_resp": resp_a.strip(),
            "node_b_status": status_b,
            "node_b_resp": resp_b.strip(),
            "success": status_a == 200 and status_b == 200,
            "note": (
                "ORDER BY includes batch_id; replaying the SAME logical event with a "
                "DIFFERENT batch_id remains duplicated under FINAL. Stable event identity "
                "is REQUIRED for general idempotence (PENDING)."
            ),
        }

    def insert_batch(self, target: str, rows: List[Tuple]) -> Tuple[int, float]:
        lines = []
        for r in rows:
            lines.append("\t".join(str(x) for x in r))
        body = "\n".join(lines).encode("utf-8")

        base_url = self.url_a if target == "a" else self.url_b
        sql = (
            "INSERT INTO platform.telemetry_metrics "
            "(tenant_id, metric_name, device_id, timestamp, batch_id, value, version) "
            "FORMAT TabSeparated"
        )
        params = {
            "database": self.database,
            "user": self.user,
            "password": self.password,
            "query": sql,
        }
        full_url = f"{base_url}/?{urllib.parse.urlencode(params)}"
        t0 = time.time()
        try:
            req = urllib.request.Request(full_url, data=body, method="POST")
            with urllib.request.urlopen(req, timeout=10) as resp:
                elapsed_ms = (time.time() - t0) * 1000
                return resp.status, round(elapsed_ms, 2)
        except urllib.error.HTTPError as e:
            elapsed_ms = (time.time() - t0) * 1000
            return e.code, round(elapsed_ms, 2)
        except Exception:
            elapsed_ms = (time.time() - t0) * 1000
            return 500, round(elapsed_ms, 2)

    def dual_insert_batch(self, rows: List[Tuple]) -> Dict[str, Any]:
        t0 = time.time()
        status_a, lat_a = self.insert_batch("a", rows)
        status_b, lat_b = self.insert_batch("b", rows)
        total_ms = (time.time() - t0) * 1000
        return {
            "status_a": status_a,
            "latency_a_ms": lat_a,
            "status_b": status_b,
            "latency_b_ms": lat_b,
            "total_latency_ms": round(total_ms, 2),
            "dual_success": status_a == 200 and status_b == 200,
        }

    def count_rows(self, target: str, use_final: bool = True) -> int:
        table = "platform.telemetry_metrics FINAL" if use_final else "platform.telemetry_metrics"
        sql = f"SELECT count() FROM {table}"
        status, resp = self.query(target, sql)
        if status == 200:
            try:
                return int(resp.strip())
            except Exception:
                return -1
        return -1

    def count_by_batch_id(self, target: str, batch_id: str, use_final: bool = True) -> int:
        table = "platform.telemetry_metrics FINAL" if use_final else "platform.telemetry_metrics"
        sql = f"SELECT count() FROM {table} WHERE batch_id = '{batch_id}'"
        status, resp = self.query(target, sql)
        if status == 200:
            try:
                return int(resp.strip())
            except Exception:
                return -1
        return -1

    def schema_columns(self, target: str) -> str:
        check_sql = (
            "SELECT name, type FROM system.columns "
            "WHERE database='platform' AND table='telemetry_metrics' ORDER BY name"
        )
        status, cols = self.query(target, check_sql)
        if status != 200:
            return ""
        return cols.strip()

    def test_schema_migration(self, column_name: str) -> Dict[str, Any]:
        """Healthy A→B DDL path (both available)."""
        ddl = (
            f"ALTER TABLE platform.telemetry_metrics "
            f"ADD COLUMN IF NOT EXISTS {column_name} String DEFAULT ''"
        )
        status_a, resp_a = self.query("a", ddl)
        status_b, resp_b = self.query("b", ddl)
        cols_a = self.schema_columns("a")
        cols_b = self.schema_columns("b")
        schemas_match = (cols_a == cols_b) and (cols_a != "")
        return {
            "path": "healthy_dual_ddl",
            "status_a": status_a,
            "status_b": status_b,
            "resp_a": resp_a.strip(),
            "resp_b": resp_b.strip(),
            "schemas_match": schemas_match,
            "declared_ready": status_a == 200 and status_b == 200 and schemas_match,
        }

    def test_ddl_partial_failure_and_recovery(self, column_name: str) -> Dict[str, Any]:
        """
        Apply DDL on A, make B unavailable, verify NOT READY, restore B, reconcile DDL,
        verify schema equality.
        """
        result: Dict[str, Any] = {
            "path": "partial_ddl_failure_recovery",
            "evidence": "EXECUTED",
            "column": column_name,
        }
        ddl = (
            f"ALTER TABLE platform.telemetry_metrics "
            f"ADD COLUMN IF NOT EXISTS {column_name} String DEFAULT ''"
        )

        status_a, resp_a = self.query("a", ddl)
        result["status_a_initial"] = status_a
        result["resp_a_initial"] = resp_a.strip()

        self.docker.stop_container("ha-clickhouse-b", timeout_sec=2)
        time.sleep(1)
        status_b_down, resp_b_down = self.query("b", ddl, timeout=3)
        result["status_b_while_down"] = status_b_down
        result["resp_b_while_down"] = resp_b_down.strip()[:200]

        cols_a = self.schema_columns("a")
        cols_b = self.schema_columns("b")
        schemas_match_mid = (cols_a == cols_b) and cols_a != "" and cols_b != ""
        declared_ready_mid = status_a == 200 and status_b_down == 200 and schemas_match_mid
        result["declared_ready_while_b_down"] = declared_ready_mid
        result["not_ready_correct"] = declared_ready_mid is False

        self.docker.start_container("ha-clickhouse-b")
        for _ in range(60):
            if self.docker.is_running("ha-clickhouse-b"):
                st, _ = self.query("b", "SELECT 1", timeout=2)
                if st == 200:
                    break
            time.sleep(1)

        status_b_recover, resp_b_recover = self.query("b", ddl)
        result["status_b_after_restore"] = status_b_recover
        result["resp_b_after_restore"] = resp_b_recover.strip()

        cols_a2 = self.schema_columns("a")
        cols_b2 = self.schema_columns("b")
        schemas_match_final = (cols_a2 == cols_b2) and cols_a2 != ""
        declared_ready_final = (
            status_a == 200 and status_b_recover == 200 and schemas_match_final
        )
        result["schemas_match_final"] = schemas_match_final
        result["declared_ready_final"] = declared_ready_final
        result["success"] = (
            result["not_ready_correct"]
            and declared_ready_final
            and column_name in cols_a2
            and column_name in cols_b2
        )
        return result

    def list_batch_ids(self, target: str, use_final: bool = True) -> Set[str]:
        table = "platform.telemetry_metrics FINAL" if use_final else "platform.telemetry_metrics"
        sql = f"SELECT DISTINCT toString(batch_id) FROM {table}"
        status, resp = self.query(target, sql)
        if status != 200 or not resp.strip():
            return set()
        return {line.strip() for line in resp.strip().splitlines() if line.strip()}

    def reconcile_by_batch_id(self) -> Dict[str, Any]:
        """
        Spike-only reconciliation helper: copy missing batch_id rows A→B and B→A.
        Not a production resync product; proves lab convergence for tested keys.
        """
        batches_a = self.list_batch_ids("a", use_final=True)
        batches_b = self.list_batch_ids("b", use_final=True)
        missing_on_b = batches_a - batches_b
        missing_on_a = batches_b - batches_a

        copied_to_b = 0
        copied_to_a = 0
        for bid in missing_on_b:
            sql = (
                "SELECT tenant_id, metric_name, device_id, timestamp, batch_id, value, version "
                f"FROM platform.telemetry_metrics FINAL WHERE batch_id = '{bid}' FORMAT TabSeparated"
            )
            st, body = self.query("a", sql)
            if st == 200 and body.strip():
                rows = []
                for line in body.strip().splitlines():
                    parts = line.split("\t")
                    if len(parts) >= 7:
                        rows.append(tuple(parts[:7]))
                if rows:
                    st_ins, _ = self.insert_batch("b", rows)
                    if st_ins == 200:
                        copied_to_b += 1

        for bid in missing_on_a:
            sql = (
                "SELECT tenant_id, metric_name, device_id, timestamp, batch_id, value, version "
                f"FROM platform.telemetry_metrics FINAL WHERE batch_id = '{bid}' FORMAT TabSeparated"
            )
            st, body = self.query("b", sql)
            if st == 200 and body.strip():
                rows = []
                for line in body.strip().splitlines():
                    parts = line.split("\t")
                    if len(parts) >= 7:
                        rows.append(tuple(parts[:7]))
                if rows:
                    st_ins, _ = self.insert_batch("a", rows)
                    if st_ins == 200:
                        copied_to_a += 1

        batches_a2 = self.list_batch_ids("a", use_final=True)
        batches_b2 = self.list_batch_ids("b", use_final=True)
        return {
            "evidence": "EXECUTED",
            "missing_on_b_before": sorted(missing_on_b),
            "missing_on_a_before": sorted(missing_on_a),
            "copied_to_b": copied_to_b,
            "copied_to_a": copied_to_a,
            "converged": batches_a2 == batches_b2,
            "batches_a_after": sorted(batches_a2),
            "batches_b_after": sorted(batches_b2),
            "scope": "SPIKE_HELPER_NOT_PRODUCTION_RESYNC",
        }

    def test_event_identity_gap(self) -> Dict[str, Any]:
        """Same logical fields, different batch_id → two FINAL rows (key insufficient)."""
        tenant = str(uuid.uuid4())
        device = str(uuid.uuid4())
        ts = "2026-09-07 15:00:00.000"
        metric = "identity.gap"
        batch_x = str(uuid.uuid4())
        batch_y = str(uuid.uuid4())
        row_x = (tenant, metric, device, ts, batch_x, 1.0, 1)
        row_y = (tenant, metric, device, ts, batch_y, 1.0, 1)
        self.insert_batch("a", [row_x])
        self.insert_batch("a", [row_y])
        sql = (
            "SELECT count() FROM platform.telemetry_metrics FINAL "
            f"WHERE tenant_id='{tenant}' AND device_id='{device}' AND metric_name='{metric}' "
            f"AND timestamp='{ts}'"
        )
        st, resp = self.query("a", sql)
        count = int(resp.strip()) if st == 200 and resp.strip().isdigit() else -1
        return {
            "evidence": "EXECUTED",
            "final_row_count_same_logical_different_batch_id": count,
            "stable_event_identity_sufficient": count == 1,
            "requirement": (
                "ORDER BY currently includes batch_id; general event idempotence requires "
                "a stable event identity independent of transport batch_id."
            ),
        }

    def wait_running(self, name: str, max_wait: int = 60) -> bool:
        for _ in range(max_wait):
            if self.docker.is_running(name):
                st, _ = self.query("a" if "a" in name else "b", "SELECT 1", timeout=2)
                # After restart, HTTP may lag; container running is enough to continue waiting
                if name.endswith("-a"):
                    st, _ = self.query("a", "SELECT 1", timeout=2)
                else:
                    st, _ = self.query("b", "SELECT 1", timeout=2)
                if st == 200:
                    return True
            time.sleep(1)
        return self.docker.is_running(name)

    def run_clickhouse_failover_drill(self) -> Dict[str, Any]:
        """
        Lab service-switch lower bound for ClickHouse only.
        Fencing = Docker kill analogue, NOT production STONITH.
        """
        if not self.docker.is_running("ha-clickhouse-b"):
            self.docker.start_container("ha-clickhouse-b")
            self.wait_running("ha-clickhouse-b", 60)

        t0 = time.time()
        self.docker.kill_container("ha-clickhouse-a", "SIGKILL")
        while self.docker.is_running("ha-clickhouse-a"):
            time.sleep(0.05)
            if time.time() - t0 > 30:
                break
        t1 = time.time()
        fence_duration = t1 - t0

        node_b = CoreNode("core-b", CoreState.FENCING, initial_epoch=1)
        node_b.handle_fence_outcome(fence_confirmed=True)
        t2 = time.time()
        promotion_duration = t2 - t1

        test_batch = str(uuid.uuid4())
        test_rows = [
            (
                str(uuid.uuid4()),
                "cpu.utilization",
                str(uuid.uuid4()),
                "2026-09-07 12:00:00.000",
                test_batch,
                42.5,
                1,
            )
        ]
        status, _ = self.insert_batch("b", test_rows)
        assert status == 200
        t3 = time.time()
        ingest_duration = t3 - t2

        count = self.count_by_batch_id("b", test_batch, use_final=True)
        assert count >= 1
        t4 = time.time()

        self.docker.start_container("ha-clickhouse-a")
        self.wait_running("ha-clickhouse-a", 90)

        return {
            "t0_fault": t0,
            "t1_fenced": t1,
            "t2_promoted": t2,
            "t3_ingested": t3,
            "t4_query_ready": t4,
            "fence_sec": round(fence_duration, 4),
            "promotion_sec": round(promotion_duration, 4),
            "ingest_sec": round(ingest_duration, 4),
            "total_restore_sec": round(t4 - t0, 4),
            "batch_id_verified": test_batch,
            "query_found_batch": count >= 1,
            "scope": "LAB_SERVICE_SWITCH_LOWER_BOUND",
            "fencing_level": "LAB_FENCING_ANALOGUE",
            "not_certified_core_ha_60s": True,
        }
