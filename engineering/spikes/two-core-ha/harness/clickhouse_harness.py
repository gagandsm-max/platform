import json
import logging
import time
import urllib.parse
import urllib.request
import uuid
from typing import Any, Dict, List, Optional, Tuple

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
        }

    def insert_batch(self, target: str, rows: List[Tuple]) -> Tuple[int, float]:
        """Insert batch of tuples via TSV format."""
        lines = []
        for r in rows:
            lines.append("\t".join(str(x) for x in r))
        body = "\n".join(lines).encode("utf-8")

        base_url = self.url_a if target == "a" else self.url_b
        sql = "INSERT INTO platform.telemetry_metrics (tenant_id, metric_name, device_id, timestamp, batch_id, value, version) FORMAT TabSeparated"
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

    def test_schema_migration(self, column_name: str) -> Dict[str, Any]:
        """
        Deterministic Core-controlled DDL:
        Apply DDL to A -> Apply DDL to B -> Declare READY only after both match.
        """
        ddl = f"ALTER TABLE platform.telemetry_metrics ADD COLUMN IF NOT EXISTS {column_name} String DEFAULT ''"
        status_a, resp_a = self.query("a", ddl)
        status_b, resp_b = self.query("b", ddl)

        # Check schemas match
        check_sql = "SELECT name, type FROM system.columns WHERE database='platform' AND table='telemetry_metrics' ORDER BY name"
        _, cols_a = self.query("a", check_sql)
        _, cols_b = self.query("b", check_sql)

        schemas_match = (cols_a.strip() == cols_b.strip()) and (cols_a.strip() != "")
        return {
            "status_a": status_a,
            "status_b": status_b,
            "schemas_match": schemas_match,
            "declared_ready": status_a == 200 and status_b == 200 and schemas_match,
        }

    def run_clickhouse_failover_drill(self) -> Dict[str, float]:
        """
        Runs one iteration of fenced failover for ClickHouse:
        T0 = fault injection (kill ha-clickhouse-a)
        T1 = positive fence confirmation (container confirmed stopped)
        T2 = Standby B promoted to primary analytical query/ingest target
        T3 = ingest accepted on ClickHouse B
        T4 = analytical query returns valid data from ClickHouse B
        """
        # Ensure ha-clickhouse-b is healthy
        if not self.docker.is_running("ha-clickhouse-b"):
            self.docker.start_container("ha-clickhouse-b")
            time.sleep(3)

        t0 = time.time()

        # Step 1: Kill ClickHouse A
        self.docker.kill_container("ha-clickhouse-a", "SIGKILL")

        # Step 2: Fencing confirmation
        while self.docker.is_running("ha-clickhouse-a"):
            time.sleep(0.05)
        t1 = time.time()
        fence_duration = t1 - t0

        # Step 3: Promote B
        node_b = CoreNode("core-b", CoreState.FENCING, initial_epoch=1)
        node_b.handle_fence_outcome(fence_confirmed=True)
        t2 = time.time()
        promotion_duration = t2 - t1

        # Step 4: Ingest accepted on B
        test_rows = [
            (str(uuid.uuid4()), "cpu.utilization", str(uuid.uuid4()), "2026-09-07 12:00:00.000", str(uuid.uuid4()), 42.5, 1)
        ]
        status, _ = self.insert_batch("b", test_rows)
        assert status == 200
        t3 = time.time()
        ingest_duration = t3 - t2

        # Step 5: Analytical query returns data
        count = self.count_rows("b", use_final=True)
        assert count >= 1
        t4 = time.time()
        query_duration = t4 - t3

        # Restore ClickHouse A for next run
        self.docker.start_container("ha-clickhouse-a")
        time.sleep(3)

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
        }
