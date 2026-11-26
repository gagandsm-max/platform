import copy
import json
import os
from pathlib import Path
import stat
import tempfile
import unittest
from unittest.mock import patch

from isolation import BUILDS, IMAGES, OWNER, PKI, ROOT, VOLUME_KEYS, VOLUMES, audit_tree, bind_paths, isolate, read_config, verify
from isolation import POSTGRES_INIT_FIXTURE, POSTGRES_INIT_SOURCE, POSTGRES_INIT_TARGET, stage_postgres_init

PREFIX = "agent-e2e-fixture-12345"
TOKEN = "1" * 32


def rendered(root, evidence):
    """Model the normalized canonical fixture before the isolation transform."""
    services = {}
    for name in (IMAGES.keys() | BUILDS.keys()) - {"unit-contracts"}:
        service = {"image": IMAGES.get(name, "shared:dev"), "networks": {"platform_net": None}}
        if name in BUILDS:
            service["build"] = {"context": str(root / BUILDS[name]), "dockerfile": "Dockerfile"}
        mounts = []
        for source, target in bind_paths(name, root, evidence, False):
            source = Path(source)
            if source == evidence / POSTGRES_INIT_FIXTURE:
                source = root / POSTGRES_INIT_SOURCE
            for private, shared in ((evidence / PKI, root / "deploy/docker/certs"),
                                    (evidence / "agent-certs", root / "tests/agent-runtime-e2e/certs")):
                if source.is_relative_to(private):
                    source = shared / source.relative_to(private)
                    break
            mounts.append({"type": "bind", "source": str(source), "target": target,
                           "read_only": True, "bind": {"create_host_path": True}})
        volumes = [VOLUMES[name]] if name in VOLUMES else []
        if name == "sensor-infra":
            volumes = [VOLUMES["sensor-agent-spool-init"], ("platform_sensor_infra_spool", "/var/lib/platform/sensor/spool")]
        mounts.extend({"type": "volume", "source": source, "target": target, "volume": {}}
                      for source, target in volumes)
        if mounts:
            service["volumes"] = mounts
        services[name] = service
    services["sensor-infra"]["environment"] = {}
    services["sensor-infra"]["deploy"] = {"resources": {"limits": {"pids": 128}}, "placement": {}}
    services["postgres"]["healthcheck"] = {"test": ["CMD-SHELL", "pg_isready -U $${POSTGRES_USER:-platform_app}"]}
    services["clickhouse-migrate"]["tmpfs"] = ["/var/lib/clickhouse"]
    services["kafka"]["tmpfs"] = ["/etc/kafka/secrets", "/mnt/shared/config"]
    services["ui"] = {"image": "shared-ui"}
    config = {"name": "platform", "services": services, "networks": {"platform_net": {"name": "shared", "driver": "bridge", "ipam": {}}},
              "x-logging": {"driver": "json-file", "options": {"max-size": "10m", "max-file": "3"}},
              "volumes": {name: {"name": "shared"} for name in VOLUME_KEYS | {"platform_sensor_net_spool"}}}
    config["volumes"]["platform_kafka_data"]["external"] = True
    return config


class IsolationTest(unittest.TestCase):
    def setUp(self):
        self.tmp = tempfile.TemporaryDirectory()
        self.addCleanup(self.tmp.cleanup)
        self.root = Path(self.tmp.name) / "source"
        self.evidence = Path(self.tmp.name) / "run"
        self.raw = rendered(self.root, self.evidence)

    def isolate(self):
        return isolate(self.raw, PREFIX, self.root, self.evidence, TOKEN)

    def verify(self, config):
        verify(config, PREFIX, self.root, self.evidence, owner=TOKEN)

    def test_namespace_and_pki(self):
        original = copy.deepcopy(self.raw)
        c = self.isolate()
        self.verify(c)
        sensor = c["services"]["sensor-infra"]
        self.assertNotIn("pids_limit", sensor)
        self.assertEqual(sensor["deploy"], original["services"]["sensor-infra"]["deploy"])
        self.assertEqual(self.raw, original)
        self.assertNotIn("ui", c["services"])
        self.assertNotIn("x-logging", c)
        self.assertEqual(c["services"]["postgres"]["healthcheck"], original["services"]["postgres"]["healthcheck"])
        self.assertEqual(sensor["labels"], {OWNER: TOKEN})
        self.assertIn(str(self.evidence / PKI / "ca.crt"), {m["source"] for m in sensor["volumes"]})

    def test_gap_fixture_remains_private(self):
        with patch.dict("os.environ", {"PLATFORM_E2E_REAL_SENSOR_GAP": "1"}):
            c = self.isolate()
            self.verify(c)
        self.assertIn(str(self.evidence / "timesync-marker"),
                      {m["source"] for m in c["services"]["sensor-infra"]["volumes"]})

    def test_postgres_bootstrap_under_restrictive_umask(self):
        config = self.isolate()
        source = self.root / POSTGRES_INIT_SOURCE
        staged = self.evidence / POSTGRES_INIT_FIXTURE
        original = b"-- fixture\r\nSELECT 1;\r\n"
        previous_umask = os.umask(0o077)
        try:
            source.parent.mkdir(parents=True)
            source.write_bytes(original)
            self.evidence.mkdir(mode=0o700)
            stage_postgres_init(self.root, self.evidence)
            directories = [self.root / path for path in (*BUILDS.values(), "deploy/docker/kafka",
                                                           "deploy/docker/clickhouse/migrations")]
            for path in (*directories, self.evidence / PKI):
                path.mkdir(parents=True, exist_ok=True)
            for path in BUILDS.values():
                (self.root / path / "Dockerfile").touch()
            for service in config["services"].values():
                for mount in service.get("volumes", []):
                    if mount["type"] == "bind":
                        path = Path(mount["source"])
                        if not path.exists():
                            path.parent.mkdir(parents=True, exist_ok=True)
                            path.touch()
        finally:
            os.umask(previous_umask)

        self.assertEqual(stat.S_IMODE(self.evidence.stat().st_mode), 0o700)
        self.assertEqual(stat.S_IMODE(source.stat().st_mode), 0o600)
        self.assertEqual(stat.S_IMODE(staged.stat().st_mode), 0o644)
        self.assertEqual(staged.read_bytes(), original)
        self.assertEqual(source.read_bytes(), original)
        self.assertEqual(stat.S_IMODE((self.evidence / "agent-certs/agent-e2e.key").stat().st_mode), 0o600)
        bind, = [m for m in config["services"]["postgres"]["volumes"] if m["type"] == "bind"]
        self.assertEqual(bind, {"type": "bind", "source": str(staged), "target": POSTGRES_INIT_TARGET,
                                "read_only": True, "bind": {"create_host_path": False}})

        def verify_ready():
            verify(config, PREFIX, self.root, self.evidence, ready=True, owner=TOKEN)

        verify_ready()
        staged.chmod(0o600)
        with self.assertRaisesRegex(ValueError, "bootstrap file or mode"):
            verify_ready()
        staged.chmod(0o644)
        staged.write_bytes(b"SELECT 2;")
        with self.assertRaisesRegex(ValueError, "bootstrap bytes differ"):
            verify_ready()
        bind["source"] = str(source)
        with self.assertRaisesRegex(ValueError, "unexpected bind: postgres"):
            self.verify(config)

    def test_unexpected_rendered_service_rejected(self):
        self.raw["services"]["unexpected"] = {}
        with self.assertRaisesRegex(ValueError, "unexpected rendered service"):
            self.isolate()

    def test_resource_and_mount_denials(self):
        for field in ("container", "volume", "network", "port", "writable", "image", "privilege", "host",
                      "anonymous", "socket", "home", "key", "build_ssh", "extends", "env_file", "pid",
                      "additional_network", "volume_driver", "bind_options", "service_label"):
            with self.subTest(field=field):
                c = self.isolate()
                s = c["services"]["sensor-infra"]
                bind = next(m for m in s["volumes"] if m["type"] == "bind")
                volume = next(m for m in s["volumes"] if m["type"] == "volume")
                if field == "container": s["container_name"] = "platform-sensor-infra"
                if field == "volume": c["volumes"]["platform_pg_data"]["name"] = "shared"
                if field == "network": c["networks"]["platform_net"]["external"] = True
                if field == "port": s["ports"][0]["host_ip"] = "0.0.0.0"
                if field == "writable": bind["read_only"] = False
                if field == "image": s["image"] = "platform-sensor:dev"
                if field == "privilege": s["privileged"] = True
                if field == "host": s["network_mode"] = "host"
                if field == "anonymous": volume["source"] = ""
                if field == "socket": bind["source"] = "/var/run/docker.sock"
                if field == "home": bind["source"] = "/home/user"
                if field == "key": bind["source"] = "/other/ca.key"
                if field == "build_ssh": s["build"]["ssh"] = ["default"]
                if field == "extends": s["extends"] = {"file": "elsewhere"}
                if field == "env_file": s["env_file"] = ["/home/user/.env"]
                if field == "pid": s["pid"] = "host"
                if field == "additional_network": s["networks"] = ["platform_net", "host"]
                if field == "volume_driver": c["volumes"]["platform_pg_data"]["driver_opts"] = {"device": "/"}
                if field == "bind_options": bind["bind"]["propagation"] = "rshared"
                if field == "service_label": s["labels"][OWNER] = "different"
                with self.assertRaises(ValueError):
                    self.verify(c)

    def test_approved_path_with_symlink_is_denied(self):
        c = self.isolate()
        pki = self.evidence / PKI
        pki.parent.mkdir(parents=True)
        pki.symlink_to(self.tmp.name)
        with self.assertRaisesRegex(ValueError, "noncanonical"):
            self.verify(c)

    def test_source_tree_policy_matches_checkout(self):
        for path in (ROOT / "agent", ROOT / "deploy/docker/kafka", ROOT / "deploy/docker/clickhouse/migrations"):
            audit_tree(path)

    def test_directory_bind_rejects_escaping_links_and_key_material(self):
        self.root.mkdir()
        outside = Path(self.tmp.name) / "outside"
        outside.write_text("fixture")
        link = self.root / "escape"
        link.symlink_to(outside)
        with self.assertRaisesRegex(ValueError, "escaping bind"):
            audit_tree(self.root)
        link.unlink()
        (self.root / "unknown").write_text("-----BEGIN OPENSSH PRIVATE KEY-----\nfixture\n")
        with self.assertRaisesRegex(ValueError, "private key"):
            audit_tree(self.root)

    def test_duplicate_rendered_property_is_denied(self):
        path = Path(self.tmp.name) / "compose.json"
        path.write_text('{"name":"one","name":"two"}')
        with self.assertRaisesRegex(ValueError, "duplicate"):
            read_config(path)
