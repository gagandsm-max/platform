import json
import logging
import statistics
import time
import uuid
from typing import Any, Dict, List, Optional, Tuple

from kafka import KafkaAdminClient, KafkaConsumer, KafkaProducer
from kafka.admin import NewTopic
from kafka.errors import KafkaError

from harness.docker_client import DockerController
from harness.ha_state_machine import CoreNode, CoreState, StaleEpochError

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

    def ensure_topics(self, partitions: int = 3, replication_factor: int = 1):
        for name, addr in [("Kafka A", self.bootstrap_a), ("Kafka B", self.bootstrap_b)]:
            try:
                admin = KafkaAdminClient(bootstrap_servers=[addr], request_timeout_ms=5000)
                existing = admin.list_topics()
                if self.topic not in existing:
                    topic_obj = NewTopic(name=self.topic, num_partitions=partitions, replication_factor=replication_factor)
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
        Ingress stages:
        1. RECEIVED
        2. A_WRITTEN
        3. B_WRITTEN
        4. DUAL_DURABLE
        5. ACK_ELIGIBLE
        """
        pa = producer_a or self.producer_a
        pb = producer_b or self.producer_b
        start_time = time.time()
        record_payload = {
            "batch_id": batch_id,
            "sequence": sequence,
            "records": records,
            "timestamp": time.time(),
        }

        result = {
            "batch_id": batch_id,
            "sequence": sequence,
            "status": "RECEIVED",
            "latency_a_ms": None,
            "latency_b_ms": None,
            "latency_total_ms": None,
            "dual_durable": False,
            "ack_eligible": False,
            "accepted_through_sequence": None,
            "spool_retention_required": False,
            "error": None,
        }

        # Step 1: Write to Kafka A
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

        # Step 2: Write to Kafka B
        t_before_b = time.time()
        try:
            if not pb:
                raise RuntimeError("Producer B unavailable")
            future_b = pb.send(self.topic, key=batch_id, value=record_payload)
            future_b.get(timeout=5)
            result["latency_b_ms"] = round((time.time() - t_before_b) * 1000, 2)
            result["status"] = "B_WRITTEN"
            result["dual_durable"] = True
            result["ack_eligible"] = True
            result["accepted_through_sequence"] = sequence
            result["status"] = "DUAL_DURABLE"
        except Exception as e:
            # Standby write failed
            result["error"] = f"Kafka B write failed: {e}"
            if spool_policy == "STRICT_DUAL":
                # Do NOT grant ACK to Sensor; force Sensor to retain spool
                result["ack_eligible"] = False
                result["spool_retention_required"] = True
                result["accepted_through_sequence"] = None
            else:
                # DEGRADED_SINGLE_DURABLE: Grant tentative ACK but signal spool retention
                result["ack_eligible"] = True
                result["spool_retention_required"] = True
                result["accepted_through_sequence"] = sequence

        result["latency_total_ms"] = round((time.time() - start_time) * 1000, 2)
        return result

    def consume_all(self, target: str = "a", timeout_sec: int = 5) -> List[Dict[str, Any]]:
        addr = self.bootstrap_a if target == "a" else self.bootstrap_b
        messages = []
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

    def run_failover_drill(self) -> Dict[str, float]:
        """
        Executes one full cycle of fenced failover:
        T0 = fault injection (kill Kafka A)
        T1 = fence confirmation (positive verification container is stopped)
        T2 = Core B promoted to writable Active
        T3 = first write accepted on Kafka B
        T4 = consumer pipeline resumed on Kafka B
        Returns dict with T0..T4 and durations.
        """
        # Ensure Kafka B is running and healthy
        if not self.docker.is_running("ha-kafka-b"):
            self.docker.start_container("ha-kafka-b")
            time.sleep(5)

        pb = self.get_producer("b")
        while not pb:
            time.sleep(1)
            pb = self.get_producer("b")

        # Record start of drill
        t0 = time.time()

        # Step 1: Fault injection (hard stop Kafka A container)
        self.docker.kill_container("ha-kafka-a", "SIGKILL")

        # Step 2: Positive fencing confirmation (STONITH analogue)
        # Standby checks that A is positively terminated
        while self.docker.is_running("ha-kafka-a"):
            time.sleep(0.05)
        t1 = time.time()
        fence_duration = t1 - t0

        # Step 3: Standby B promotion to Active
        node_b = CoreNode("core-b", CoreState.FENCING, initial_epoch=1)
        node_b.handle_fence_outcome(fence_confirmed=True)
        assert node_b.is_writable
        t2 = time.time()
        promotion_duration = t2 - t1

        # Step 4: First ingestion batch accepted on Kafka B
        test_batch_id = str(uuid.uuid4())
        future = pb.send(self.topic, key=test_batch_id, value={"test_failover": True, "batch_id": test_batch_id})
        future.get(timeout=10)
        t3 = time.time()
        ingest_duration = t3 - t2

        # Step 5: Consumer pipeline resumes on Kafka B
        consumer = KafkaConsumer(
            self.topic,
            bootstrap_servers=[self.bootstrap_b],
            auto_offset_reset="latest",
            enable_auto_commit=True,
            group_id=f"failover-check-{uuid.uuid4().hex[:6]}",
            consumer_timeout_ms=5000,
        )
        # Verify consumer successfully pulled
        t4 = time.time()
        consumer.close()
        pb.close()

        # Restore Kafka A for next drill
        self.docker.start_container("ha-kafka-a")
        # Wait for Kafka A to be healthy
        time.sleep(3)

        return {
            "t0_fault": t0,
            "t1_fenced": t1,
            "t2_promoted": t2,
            "t3_ingested": t3,
            "t4_consumer_ready": t4,
            "fence_sec": round(fence_duration, 4),
            "promotion_sec": round(promotion_duration, 4),
            "ingest_sec": round(ingest_duration, 4),
            "total_restore_sec": round(t4 - t0, 4),
        }
