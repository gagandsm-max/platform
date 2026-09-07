import json
import logging
import time
import uuid
from typing import Any, Dict, List, Optional

from kafka import KafkaAdminClient, KafkaConsumer, KafkaProducer
from kafka.admin import NewTopic

from harness.docker_client import DockerController
from harness.ha_state_machine import CoreNode, CoreState

logger = logging.getLogger("KafkaHarness")


class KafkaSpikeHarness:
    def __init__(
        self,
        bootstrap_a: str = "ha-kafka-a:9092",
        bootstrap_b: str = "ha-kafka-b:9092",
        topic: str = "telemetry-ingress",
    ):
        self.bootstrap_a = bootstrap_a
        self.bootstrap_b = bootstrap_b
        self.topic = topic
        self.docker = DockerController()
        self.producer_a: Optional[KafkaProducer] = None
        self.producer_b: Optional[KafkaProducer] = None

    def get_producer(self, target: str) -> Optional[KafkaProducer]:
        addr = self.bootstrap_a if target == "a" else self.bootstrap_b
        try:
            return KafkaProducer(
                bootstrap_servers=[addr],
                value_serializer=lambda v: json.dumps(v).encode("utf-8"),
                key_serializer=lambda k: k.encode("utf-8") if k else None,
                acks="all",
                retries=3,
                request_timeout_ms=5000,
                max_block_ms=5000,
            )
        except Exception as e:
            logger.warning(f"Could not connect producer to {addr}: {e}")
            return None

    def init_producers(self):
        self.producer_a = self.get_producer("a")
        self.producer_b = self.get_producer("b")

    def close_producers(self):
        for p in (self.producer_a, self.producer_b):
            if p is not None:
                try:
                    p.close()
                except Exception:
                    pass
        self.producer_a = None
        self.producer_b = None

    def ensure_topics(self, partitions: int = 3, replication_factor: int = 1):
        for name, addr in [("Kafka A", self.bootstrap_a), ("Kafka B", self.bootstrap_b)]:
            try:
                admin = KafkaAdminClient(bootstrap_servers=[addr], request_timeout_ms=5000)
                existing = admin.list_topics()
                if self.topic not in existing:
                    topic_obj = NewTopic(
                        name=self.topic,
                        num_partitions=partitions,
                        replication_factor=replication_factor,
                    )
                    admin.create_topics([topic_obj])
                    logger.info(f"Created topic {self.topic} on {name}")
                admin.close()
            except Exception as e:
                logger.warning(f"ensure_topics failed on {name}: {e}")

    def ingest_batch_dual_write(
        self,
        batch_id: str,
        sequence: int,
        records: List[Dict[str, Any]],
        producer_a: Optional[KafkaProducer] = None,
        producer_b: Optional[KafkaProducer] = None,
        spool_policy: str = "STRICT_DUAL",
    ) -> Dict[str, Any]:
        """
        STRICT_DUAL only for Sensor accepted-through ACK eligibility.

        DEGRADED_SINGLE_DURABLE that advances accepted_through_sequence is REJECTED:
        Sensor treats accepted_through as the spool deletion boundary.
        """
        if spool_policy == "DEGRADED_SINGLE_DURABLE":
            spool_policy = "STRICT_DUAL"

        pa = producer_a or self.producer_a
        pb = producer_b or self.producer_b
        start_time = time.time()
        record_payload = {
            "batch_id": batch_id,
            "sequence": sequence,
            "records": records,
            "timestamp": time.time(),
        }

        result: Dict[str, Any] = {
            "batch_id": batch_id,
            "sequence": sequence,
            "status": "RECEIVED",
            "latency_a_ms": None,
            "latency_b_ms": None,
            "latency_total_ms": None,
            "dual_durable": False,
            "ack_eligible": False,
            "accepted_through_sequence": None,
            "spool_retention_required": True,
            "degraded_ack_policy": "REJECTED",
            "error": None,
        }

        t_before_a = time.time()
        try:
            if not pa:
                raise RuntimeError("Producer A unavailable")
            future_a = pa.send(self.topic, key=batch_id, value=record_payload)
            future_a.get(timeout=5)
            result["latency_a_ms"] = round((time.time() - t_before_a) * 1000, 2)
            result["status"] = "A_WRITTEN"
        except Exception as e:
            result["error"] = f"Kafka A write failed: {e}"
            result["latency_total_ms"] = round((time.time() - start_time) * 1000, 2)
            return result

        t_before_b = time.time()
        try:
            if not pb:
                raise RuntimeError("Producer B unavailable")
            future_b = pb.send(self.topic, key=batch_id, value=record_payload)
            future_b.get(timeout=5)
            result["latency_b_ms"] = round((time.time() - t_before_b) * 1000, 2)
            result["dual_durable"] = True
            result["ack_eligible"] = True
            result["accepted_through_sequence"] = sequence
            result["spool_retention_required"] = False
            result["status"] = "DUAL_DURABLE"
        except Exception as e:
            result["error"] = f"Kafka B write failed: {e}"
            # Partial durability: NEVER advance accepted_through; Sensor retains spool.
            result["ack_eligible"] = False
            result["spool_retention_required"] = True
            result["accepted_through_sequence"] = None
            result["degraded_ack_policy"] = "REJECTED_NO_SEQUENCE_ADVANCE"

        result["latency_total_ms"] = round((time.time() - start_time) * 1000, 2)
        return result

    def write_single(
        self,
        target: str,
        batch_id: str,
        sequence: int,
        records: List[Dict[str, Any]],
    ) -> Dict[str, Any]:
        payload = {
            "batch_id": batch_id,
            "sequence": sequence,
            "records": records,
            "timestamp": time.time(),
        }
        result = {
            "batch_id": batch_id,
            "target": target,
            "success": False,
            "error": None,
            "ack_eligible": False,
            "accepted_through_sequence": None,
            "spool_retention_required": True,
        }
        p = self.get_producer(target)
        try:
            if not p:
                raise RuntimeError(f"Producer {target} unavailable")
            future = p.send(self.topic, key=batch_id, value=payload)
            future.get(timeout=5)
            result["success"] = True
        except Exception as e:
            result["error"] = str(e)
        finally:
            if p:
                try:
                    p.close()
                except Exception:
                    pass
        return result

    def consume_until_batch_id(
        self,
        target: str,
        batch_id: str,
        timeout_sec: float = 30.0,
    ) -> Dict[str, Any]:
        addr = self.bootstrap_a if target == "a" else self.bootstrap_b
        consumer = KafkaConsumer(
            self.topic,
            bootstrap_servers=[addr],
            auto_offset_reset="earliest",
            enable_auto_commit=False,
            group_id=f"ha-spike-verify-{uuid.uuid4().hex[:8]}",
            value_deserializer=lambda v: json.loads(v.decode("utf-8")),
            consumer_timeout_ms=int(timeout_sec * 1000),
        )
        t0 = time.time()
        found = False
        messages_seen = 0
        matched_offset = None
        try:
            for msg in consumer:
                messages_seen += 1
                val = msg.value if isinstance(msg.value, dict) else {}
                if val.get("batch_id") == batch_id:
                    found = True
                    matched_offset = msg.offset
                    break
                if time.time() - t0 > timeout_sec:
                    break
        finally:
            consumer.close()
        return {
            "found": found,
            "batch_id": batch_id,
            "target": target,
            "messages_seen": messages_seen,
            "matched_offset": matched_offset,
            "elapsed_ms": round((time.time() - t0) * 1000, 2),
        }

    def count_batch_copies(self, target: str, batch_id: str, timeout_sec: float = 20.0) -> int:
        addr = self.bootstrap_a if target == "a" else self.bootstrap_b
        consumer = KafkaConsumer(
            self.topic,
            bootstrap_servers=[addr],
            auto_offset_reset="earliest",
            enable_auto_commit=False,
            group_id=f"ha-spike-count-{uuid.uuid4().hex[:8]}",
            value_deserializer=lambda v: json.loads(v.decode("utf-8")),
            consumer_timeout_ms=int(timeout_sec * 1000),
        )
        count = 0
        try:
            for msg in consumer:
                val = msg.value if isinstance(msg.value, dict) else {}
                if val.get("batch_id") == batch_id:
                    count += 1
        finally:
            consumer.close()
        return count

    def consume_all(self, target: str = "a", timeout_sec: int = 5) -> List[Dict[str, Any]]:
        addr = self.bootstrap_a if target == "a" else self.bootstrap_b
        messages: List[Dict[str, Any]] = []
        try:
            consumer = KafkaConsumer(
                self.topic,
                bootstrap_servers=[addr],
                auto_offset_reset="earliest",
                enable_auto_commit=True,
                group_id=f"harness-verifier-{uuid.uuid4().hex[:8]}",
                consumer_timeout_ms=timeout_sec * 1000,
                value_deserializer=lambda v: json.loads(v.decode("utf-8")),
            )
            for msg in consumer:
                messages.append(msg.value)
            consumer.close()
        except Exception as e:
            logger.warning(f"consume_all failed on {target}: {e}")
        return messages

    def wait_healthy(self, name: str, max_wait: int = 60) -> bool:
        for _ in range(max_wait):
            if self.docker.is_healthy(name) or self.docker.is_running(name):
                # Prefer health when present; after kill/start health may lag.
                info = self.docker.inspect_container(name)
                if info and info.get("State", {}).get("Running"):
                    health = info.get("State", {}).get("Health")
                    if not health or health.get("Status") == "healthy":
                        return True
            time.sleep(1)
        return self.docker.is_running(name)

    def run_failover_drill(self) -> Dict[str, Any]:
        """
        Lab service-switch lower bound:
        T0 fault → lab fence analogue → promote model → produce unique batch to B →
        T4 ONLY after exact batch_id is consumed.

        NOT certified <60s Core HA (excludes heartbeat, production fencing, VIP/DNS,
        Core app promotion, TLS, Sensor reconnect/ACK, customer latency).
        """
        if not self.docker.is_running("ha-kafka-b"):
            self.docker.start_container("ha-kafka-b")
            self.wait_healthy("ha-kafka-b", 60)

        pb = self.get_producer("b")
        for _ in range(30):
            if pb:
                break
            time.sleep(1)
            pb = self.get_producer("b")
        if not pb:
            raise RuntimeError("Kafka B producer unavailable before failover drill")

        t0 = time.time()

        self.docker.kill_container("ha-kafka-a", "SIGKILL")
        while self.docker.is_running("ha-kafka-a"):
            time.sleep(0.05)
            if time.time() - t0 > 30:
                break
        t1 = time.time()
        fence_duration = t1 - t0

        node_b = CoreNode("core-b", CoreState.FENCING, initial_epoch=1)
        node_b.handle_fence_outcome(fence_confirmed=True)
        assert node_b.is_writable
        t2 = time.time()
        promotion_duration = t2 - t1

        test_batch_id = f"failover-verify-{uuid.uuid4()}"
        future = pb.send(
            self.topic,
            key=test_batch_id,
            value={"test_failover": True, "batch_id": test_batch_id, "sequence": 999001},
        )
        future.get(timeout=10)
        t3 = time.time()
        ingest_duration = t3 - t2
        try:
            pb.close()
        except Exception:
            pass

        consume = self.consume_until_batch_id("b", test_batch_id, timeout_sec=45.0)
        t4 = time.time()
        if not consume.get("found"):
            # Restore A then fail hard so results are not fabricated as success.
            self.docker.start_container("ha-kafka-a")
            self.wait_healthy("ha-kafka-a", 90)
            raise AssertionError(
                f"T4 consume failed: batch_id {test_batch_id} not observed on Kafka B "
                f"(seen={consume.get('messages_seen')})"
            )

        # Restore Kafka A for next drill
        self.docker.start_container("ha-kafka-a")
        self.wait_healthy("ha-kafka-a", 90)
        self.init_producers()

        return {
            "t0_fault": t0,
            "t1_fenced": t1,
            "t2_promoted": t2,
            "t3_ingested": t3,
            "t4_consumer_ready": t4,
            "fence_sec": round(fence_duration, 4),
            "promotion_sec": round(promotion_duration, 4),
            "ingest_sec": round(ingest_duration, 4),
            "consume_verify_sec": round(t4 - t3, 4),
            "total_restore_sec": round(t4 - t0, 4),
            "batch_consumed": True,
            "test_batch_id": test_batch_id,
            "consume_detail": consume,
            "scope": "LAB_SERVICE_SWITCH_LOWER_BOUND",
            "fencing_level": "LAB_FENCING_ANALOGUE",
            "not_certified_core_ha_60s": True,
        }
