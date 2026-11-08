#!/usr/bin/env python3
"""Render/verify a fresh run namespace; never accept shared runtime names."""
import copy
import json
from pathlib import Path
import re
import sys


def verify(config, prefix):
    if not re.fullmatch(r"agent-e2e-[a-z0-9-]{8,80}", prefix):
        raise ValueError("invalid isolated project prefix")
    if config.get("name") != prefix:
        raise ValueError("project mismatch")
    for name, service in config["services"].items():
        if service.get("container_name") != prefix + "-" + name:
            raise ValueError("shared container name")
        if service.get("network_mode") or service.get("privileged"):
            raise ValueError("host namespace or privileged service")
        for port in service.get("ports", []):
            if port.get("host_ip") != "127.0.0.1" or str(port.get("published")) != "0":
                raise ValueError("port is not ephemeral loopback")
        for mount in service.get("volumes", []):
            if mount["type"] == "bind" and not mount.get("read_only"):
                raise ValueError("writable host bind")
            if mount["type"] == "volume" and mount.get("source") not in config["volumes"]:
                raise ValueError("anonymous or undeclared volume")
        if "build" in service and service.get("image") != prefix + ":" + name:
            raise ValueError("shared build tag")
    for kind in ("volumes", "networks"):
        for name, resource in config.get(kind, {}).items():
            if resource.get("name") != prefix + "-" + name:
                raise ValueError("shared resource name")
            if resource.get("external") and not (kind == "volumes" and name == "platform_kafka_data"):
                raise ValueError("unexpected external resource")


def isolate(config, prefix, root, evidence):
    c = copy.deepcopy(config)
    c["name"] = prefix
    c["services"]["unit-contracts"] = {
        "image": "golang:1.27.1-alpine3.24", "user": "1000:1000",
        "working_dir": "/source", "read_only": True, "cpus": 2,
        "mem_limit": "2g", "pids_limit": 128, "cap_drop": ["ALL"],
        "security_opt": ["no-new-privileges:true"],
        "tmpfs": ["/tmp:rw,exec,size=1073741824,mode=1777"],
        "environment": {"CGO_ENABLED": "0", "GOMAXPROCS": "2", "GOCACHE": "/tmp/cache", "GOPATH": "/tmp/go"},
        "networks": list(c["networks"]),
        "volumes": [{"type": "bind", "source": str(root / "agent"), "target": "/source", "read_only": True}],
        "command": ["go", "test", "-count=1", "-p=1", "-timeout=180s", "./common/client", "./common/delivery", "./common/queue", "./common/health", "./common/config"],
    }
    for name, service in c["services"].items():
        service["container_name"] = prefix + "-" + name
        service["restart"] = "no"
        # Preserve existing infrastructure resource contracts. ClickHouse's
        # governed image needs more than 512 startup threads; imposing a new
        # unrelated PID ceiling here breaks its existing configuration.
        if name in ("agent-runtime", "unit-contracts"):
            service.setdefault("pids_limit", 128)
        if name in ("agent-runtime-init", "sensor-agent-spool-init"):
            service.update(cpus=0.25, mem_limit="128m", pids_limit=64)
        if "build" in service:
            service["image"] = prefix + ":" + name
        # Only the receiver needs a host port; all other probes run via exec.
        service["ports"] = ([{"target": 8444, "published": "0", "host_ip": "127.0.0.1", "protocol": "tcp"}]
                            if name == "sensor-infra" else [])
        for mount in service.get("volumes", []):
            if mount["type"] != "bind":
                continue
            source = Path(mount["source"])
            for old, new in ((root / "deploy/docker/certs", evidence / "pki/deploy/docker/certs"),
                             (root / "tests/agent-runtime-e2e/certs", evidence / "agent-certs")):
                if source == old or old in source.parents:
                    mount["source"] = str(new / source.relative_to(old))
    for kind in ("volumes", "networks"):
        for name, resource in c.get(kind, {}).items():
            resource["name"] = prefix + "-" + name
    verify(c, prefix)
    return c


if __name__ == "__main__":
    action, file, prefix = sys.argv[1:4]
    config = json.loads(Path(file).read_text())
    if action == "render":
        print(json.dumps(isolate(config, prefix, Path(sys.argv[4]).resolve(), Path(sys.argv[5]).resolve()), indent=2))
    elif action == "verify":
        verify(config, prefix)
    else:
        raise SystemExit("unknown action")
