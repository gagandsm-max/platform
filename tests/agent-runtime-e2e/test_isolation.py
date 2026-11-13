import copy
import unittest
from pathlib import Path
from isolation import isolate, verify


class IsolationTest(unittest.TestCase):
    def setUp(self):
        self.prefix = "agent-e2e-fixture-12345"
        self.raw = {"name": "platform", "services": {"sensor-infra": {
            "container_name": "platform-sensor-infra", "image": "platform-sensor:dev", "build": {"context": "/repo/sensor"},
            "ports": [{"target": 8444, "published": "28444"}],
            "deploy": {"resources": {"limits": {"pids": 128}}},
            "volumes": [{"type": "bind", "source": "/repo/deploy/docker/certs/ca.crt", "target": "/certs/ca.crt", "read_only": True},
                        {"type": "volume", "source": "spool", "target": "/spool"}]}},
            "volumes": {"spool": {"name": "shared"}}, "networks": {"default": {"name": "shared"}}}

    def test_namespace_and_pki(self):
        original = copy.deepcopy(self.raw)
        c = isolate(self.raw, self.prefix, Path("/repo"), Path("/evidence"))
        verify(c, self.prefix)
        self.assertNotIn("pids_limit", c["services"]["sensor-infra"])
        self.assertEqual(c["services"]["sensor-infra"]["deploy"], original["services"]["sensor-infra"]["deploy"])
        self.assertEqual(self.raw, original)
        self.assertEqual(c["services"]["sensor-infra"]["volumes"][0]["source"], "/evidence/pki/deploy/docker/certs/ca.crt")

    def test_shared_resources_and_privileges_rejected(self):
        for field in ("container", "volume", "network", "port", "writable", "image", "privilege", "host", "anonymous"):
            with self.subTest(field=field):
                c = isolate(self.raw, self.prefix, Path("/repo"), Path("/evidence"))
                s = c["services"]["sensor-infra"]
                if field == "container": s["container_name"] = "platform-sensor-infra"
                if field == "volume": c["volumes"]["spool"]["name"] = "shared"
                if field == "network": c["networks"]["default"]["external"] = True
                if field == "port": s["ports"][0]["host_ip"] = "0.0.0.0"
                if field == "writable": s["volumes"][0]["read_only"] = False
                if field == "image": s["image"] = "platform-sensor:dev"
                if field == "privilege": s["privileged"] = True
                if field == "host": s["network_mode"] = "host"
                if field == "anonymous": s["volumes"][1].pop("source")
                with self.assertRaises(ValueError): verify(c, self.prefix)


if __name__ == "__main__": unittest.main()
