#!/usr/bin/env python3
"""Compose policy and process/resource custody for the isolated Agent fixture."""
import copy
import hashlib
import json
import os
from pathlib import Path
import re
import secrets
import shlex
import shutil
import signal
import stat
import subprocess
import sys
import tempfile
import time

HERE = Path(__file__).resolve().parent
ROOT = HERE.parents[1]
OWNER = "platform.e2e.owner"
PROJECT = "com.docker.compose.project"
BUILDS = {"core": "core", "sensor-infra": "sensor", "agent-runtime": "agent"}
IMAGES = {
    "postgres": "postgres:16-alpine", "clickhouse": "clickhouse/clickhouse-server:24.8-alpine",
    "clickhouse-migrate": "clickhouse/clickhouse-server:24.8-alpine", "kafka": "apache/kafka:3.8.0",
    "valkey": "valkey/valkey:7.2-alpine", "agent-runtime-init": "alpine:3.24",
    "sensor-agent-spool-init": "alpine:3.24", "unit-contracts": "golang:1.27.1-alpine3.24",
}
DORMANT = {"core-standby", "ui", "sensor-net", "sensor-net-02", "analytics"}
SERVICE_FIELDS = set("""
    build image container_name restart profiles environment ports volumes networks healthcheck
    cpus mem_limit deploy logging depends_on entrypoint command tmpfs user working_dir read_only
    pids_limit cap_drop security_opt labels
""".split())
VOLUMES = {
    "postgres": ("platform_pg_data", "/var/lib/postgresql/data"),
    "clickhouse": ("platform_ch_data", "/var/lib/clickhouse"),
    "kafka": ("platform_kafka_data", "/var/lib/kafka/data"),
    "valkey": ("platform_valkey_data", "/data"),
    "agent-runtime": ("platform_agent_e2e_state", "/var/lib/platform/agent"),
    "agent-runtime-init": ("platform_agent_e2e_state", "/var/lib/platform/agent"),
    "sensor-agent-spool-init": ("platform_sensor_infra_agent_spool", "/var/lib/platform/sensor/agent-spool"),
}
VOLUME_KEYS = {source for source, _ in VOLUMES.values()} | {"platform_sensor_infra_spool"}
PKI = "pki/deploy/docker/certs"
POSTGRES_INIT_SOURCE = "deploy/docker/postgres/init.sql"
POSTGRES_INIT_FIXTURE = "postgres-init.sql"
POSTGRES_INIT_TARGET = "/docker-entrypoint-initdb.d/01-init.sql"
HELPERS = {"pki", "parser", "kafka-helper"}
INVALID_CONFIG = "invalid-config"
INVALID_CONFIG_SERVICE = "agent-runtime-invalid-config"
COMPOSE_SERVICE = "com.docker.compose.service"
ID = re.compile(r"[a-f0-9]{64}")
REAP_GRACE = 0.1
GATE_ERRORS = (ValueError, KeyError, TypeError, AttributeError, OSError, subprocess.SubprocessError)


HEALTH_FORMAT = "{{if .State.Health}}{{.State.Health.Status}}{{else}}{{.State.Status}}{{end}}"
MATRIX_HEALTH_FORMAT = "{{.State.Health.Status}}"
MATRIX_STARTED_AT_FORMAT = "{{.State.StartedAt}}"
KAFKA_VOLUME_FORMAT = '{{index .Labels "issue223.owner"}}|{{index .Labels "platform.kafka.project"}}|{{index .Labels "platform.kafka.storage-id"}}|{{index .Labels "platform.kafka.resource"}}'
KAFKA_CONTAINER_FORMATS = {
    '{{.Name}}|{{index .Config.Labels "com.docker.compose.project"}}|{{index .Config.Labels "com.docker.compose.service"}}|{{join .Config.Entrypoint " "}}|{{.Config.Image}}|{{.State.Status}}',
    '{{range .Mounts}}{{if eq .Destination "/var/lib/kafka/data"}}{{.Type}}|{{.Name}}{{end}}{{end}}',
    '{{range .Mounts}}{{println .Destination}}{{end}}',
    '{{range .Config.Env}}{{println .}}{{end}}',
}
KAFKA_COPY_PATHS = {
    "/tmp/kafka-logs/meta.properties", "/tmp/kraft-combined-logs/meta.properties",
    "/tmp/platform-kafka-server.properties", "/opt/kafka/config/server.properties",
}
EXEC_PROGRAMS = {
    "agent-runtime": {"sh", "wget"},
    "sensor-infra": {"sh", "cat", "curl"},
    "postgres": {"psql"},
    "clickhouse": {"sh"},
    "kafka": {"/opt/kafka/bin/" + tool + ".sh" for tool in
              ("kafka-topics", "kafka-get-offsets", "kafka-console-consumer")},
}


def require(condition, reason):
    if not condition:
        raise ValueError(reason)


def bind_paths(service, root, evidence, gap):
    paths = {
        "postgres": {evidence / POSTGRES_INIT_FIXTURE: POSTGRES_INIT_TARGET},
        "clickhouse": {"deploy/docker/clickhouse/init.sql": "/docker-entrypoint-initdb.d/01-init.sql"},
        "clickhouse-migrate": {"deploy/docker/clickhouse/migrations": "/migrations"},
        "kafka": {"deploy/docker/kafka": "/platform-kafka"},
        "core": {evidence / PKI: "/certs"},
        "unit-contracts": {"agent": "/source"},
        "agent-runtime-init": {"tests/agent-runtime-e2e/identity.json": "/seed/identity.json",
                               evidence / PKI / "ca.crt": "/seed/ca.crt"},
        "sensor-infra": {},
    }.get(service, {})
    if service == "agent-runtime-init":
        for name in ("agent-e2e.crt", "agent-e2e.key"):
            paths[evidence / "agent-certs" / name] = "/seed/" + name
    if service == "sensor-infra":
        names = ["ca.crt", "sensor-infra.crt", "sensor-infra.key"]
        if gap:
            names += ["sensor-net.crt", "sensor-net.key"]
            paths[evidence / "timesync-marker"] = "/run/systemd/timesync/synchronized"
        for name in names:
            paths[evidence / PKI / name] = "/certs/" + name
        for name in ("sensor-infra-agent-server.crt", "sensor-infra-agent-server.key"):
            paths[evidence / "agent-certs" / name] = "/certs/" + name
    return {(str(root / path), target) for path, target in paths.items()}


def canonical(path, exists=False):
    require(path.is_absolute() and path == path.resolve(strict=exists), f"noncanonical path: {path}")
    if exists:
        require(path.is_file() or path.is_dir(), f"unsupported path type: {path}")


def audit_tree(path, private=False):
    """Directory binds expose descendants, including untracked files."""
    canonical(path, exists=True)
    entries = path.rglob("*") if path.is_dir() else (path,)
    for entry in entries:
        mode = entry.lstat().st_mode
        if stat.S_ISLNK(mode):
            target = entry.resolve(strict=True)
            require(target.is_file() and target.is_relative_to(path), f"escaping bind symlink: {entry}")
            continue
        if stat.S_ISDIR(mode):
            continue
        require(stat.S_ISREG(mode) and entry.stat().st_nlink == 1, f"unsafe bind entry: {entry}")
        if not private:
            require(entry.suffix.lower() not in {".p12", ".pfx", ".jks", ".pem", ".key"}
                    or entry.name == "llvm-snapshot.gpg.key", f"key-like file in source bind: {entry}")
            with entry.open("rb") as stream:
                require(not re.search(rb"(?m)^\s*-----BEGIN [A-Z0-9 ]*PRIVATE KEY-----\s*$", stream.read()),
                        f"private key in source bind: {entry}")


def stage_postgres_init(root, evidence):
    source = root / POSTGRES_INIT_SOURCE
    audit_tree(source)
    require(source.is_file(), "PostgreSQL bootstrap source must be a regular file")
    with (evidence / POSTGRES_INIT_FIXTURE).open("xb") as stream:
        stream.write(source.read_bytes())
        stream.flush()
        # The container's postgres user reads the file through a file bind.
        os.fchmod(stream.fileno(), 0o644)


def verify(config, prefix, root=None, evidence=None, *, ready=False, owner=None):
    root = root if root is not None else ROOT
    evidence = evidence if evidence is not None else Path(os.environ["PLATFORM_E2E_RUN_DIR"])
    owner = owner if owner is not None else os.environ["E2E_CUSTODY"]
    gap = os.environ.get("PLATFORM_E2E_REAL_SENSOR_GAP") == "1"
    require(re.fullmatch(r"agent-e2e-[a-z0-9-]{8,80}", prefix), "invalid isolated project")
    require(re.fullmatch(r"[a-f0-9]{32}", owner), "invalid run owner")
    require(set(config) == {"name", "services", "networks", "volumes"}
            and config["name"] == prefix, "unexpected project configuration")
    require(set(config["services"]) == IMAGES.keys() | BUILDS.keys(), "unexpected services")
    require(set(config["networks"]) == {"platform_net"} and set(config["volumes"]) == VOLUME_KEYS,
            "unexpected networks or volumes")
    labels = {OWNER: owner}
    network = config["networks"]["platform_net"]
    require(network.get("ipam", {}) == {} and
            {k: v for k, v in network.items() if k != "ipam"} ==
            {"name": prefix + "-platform_net", "driver": "bridge", "labels": labels},
            "unexpected network policy")
    for name, resource in config["volumes"].items():
        expected = {"name": prefix + "-" + name}
        expected.update({"external": True} if name == "platform_kafka_data" else {"labels": labels})
        require(resource == expected, f"unexpected volume policy: {name}")
    for name, service in config["services"].items():
        require(set(service) <= SERVICE_FIELDS, f"unexpected service options: {name}")
        require(service.get("container_name") == prefix + "-" + name and service.get("restart") == "no"
                and service.get("labels") == labels, f"unexpected service identity: {name}")
        networks = service.get("networks")
        require(networks in (["platform_net"], {"platform_net": None}, {"platform_net": {}}),
                f"unexpected service networks: {name}")
        require(set(service.get("depends_on", ())) <= config["services"].keys(),
                f"unexpected dependency: {name}")
        deploy = service.get("deploy", {})
        resources = deploy.get("resources", {})
        require(set(deploy) <= {"resources", "placement"} and deploy.get("placement", {}) == {}
                and set(resources) <= {"limits"}
                and set(resources.get("limits", {})) <= {"cpus", "memory", "pids"},
                f"unexpected deploy options: {name}")
        require(service.get("security_opt", []) == (["no-new-privileges:true"] if name == "unit-contracts" else [])
                and service.get("cap_drop", []) == (["ALL"] if name == "unit-contracts" else []),
                f"unexpected security options: {name}")
        build = service.get("build")
        if name in BUILDS:
            context = root / BUILDS[name]
            canonical(context, exists=ready)
            canonical(context / "Dockerfile", exists=ready)
            require(build == {"context": str(context), "dockerfile": "Dockerfile", "labels": labels}
                    and service.get("image") == prefix + ":" + name, f"unexpected build: {name}")
        else:
            require("build" not in service and service.get("image") == IMAGES[name], f"unexpected image: {name}")
        ports = service.get("ports", [])
        if name == "sensor-infra":
            require(len(ports) == 1 and ports[0].get("mode", "ingress") == "ingress"
                    and {k: v for k, v in ports[0].items() if k != "mode"} ==
                    {"target": 8444, "published": "0", "host_ip": "127.0.0.1", "protocol": "tcp"},
                    "receiver must use an ephemeral loopback port")
        else:
            require(not ports, f"unexpected published port: {name}")
        require(service.get("tmpfs", []) == {
            "kafka": ["/etc/kafka/secrets", "/mnt/shared/config"],
            "clickhouse-migrate": ["/var/lib/clickhouse"],
            "unit-contracts": ["/tmp:rw,exec,size=1073741824,mode=1777"],
        }.get(name, []), f"unexpected tmpfs: {name}")
        binds = bind_paths(name, root, evidence, gap)
        volumes = {VOLUMES[name]} if name in VOLUMES else set()
        if name == "sensor-infra":
            volumes = {VOLUMES["sensor-agent-spool-init"], ("platform_sensor_infra_spool", "/var/lib/platform/sensor/spool")}
        for mount in service.get("volumes", []):
            pair = (mount["source"], mount["target"])
            if mount["type"] == "bind":
                require(pair in binds and mount.get("read_only") is True
                        and set(mount) <= {"type", "source", "target", "read_only", "bind"}
                        and mount.get("bind") == {"create_host_path": False}, f"unexpected bind: {name}")
                source = Path(pair[0])
                canonical(source, exists=ready)
                if ready:
                    audit_tree(source, private=source.is_relative_to(evidence) and name != "postgres")
                    if name == "postgres":
                        require(source.is_file() and stat.S_IMODE(source.stat().st_mode) == 0o644,
                                "invalid staged PostgreSQL bootstrap file or mode")
                        require(source.read_bytes() == (root / POSTGRES_INIT_SOURCE).read_bytes(),
                                "staged PostgreSQL bootstrap bytes differ from source")
                binds.remove(pair)
            else:
                require(mount["type"] == "volume" and pair in volumes
                        and set(mount) <= {"source", "target", "type", "volume"}
                        and mount.get("volume", {}) == {}, f"unexpected volume mount: {name}")
                volumes.remove(pair)
        require(not binds and not volumes, f"missing fixture mount: {name}")


def isolate(config, prefix, root, evidence, owner=None):
    owner = owner if owner is not None else os.environ["E2E_CUSTODY"]
    c = copy.deepcopy(config)
    if "x-logging" in c:
        require(c.pop("x-logging") == {"driver": "json-file", "options": {"max-size": "10m", "max-file": "3"}},
                "unexpected logging extension")
    require(set(c) == {"name", "services", "volumes", "networks"}, "unexpected rendered top-level resource")
    require(set(c["services"]) <= (IMAGES.keys() | BUILDS.keys() | DORMANT) - {"unit-contracts"},
            "unexpected rendered service")
    for name in DORMANT:
        c["services"].pop(name, None)
    c["volumes"].pop("platform_sensor_net_spool", None)
    c["name"] = prefix
    if os.environ.get("PLATFORM_E2E_REAL_SENSOR_GAP") == "1":
        for source, target in ((evidence / "timesync-marker", "/run/systemd/timesync/synchronized"),
                               (evidence / PKI / "sensor-net.crt", "/certs/sensor-net.crt"),
                               (evidence / PKI / "sensor-net.key", "/certs/sensor-net.key")):
            c["services"]["sensor-infra"]["volumes"].append({
                "type": "bind", "source": str(source), "target": target, "read_only": True,
            })
        c["services"]["sensor-infra"]["environment"].update({
            "SENSOR_AGENT_DRAIN_IDLE_INTERVAL_SECONDS": "3600",
            "SENSOR_AGENT_DRAIN_MAX_BACKOFF_SECONDS": "3600",
        })
    c["services"]["unit-contracts"] = {
        "image": IMAGES["unit-contracts"], "user": "1000:1000",
        "working_dir": "/source", "read_only": True, "cpus": 2,
        "mem_limit": "2g", "pids_limit": 128, "cap_drop": ["ALL"],
        "security_opt": ["no-new-privileges:true"],
        "tmpfs": ["/tmp:rw,exec,size=1073741824,mode=1777"],
        "environment": {"CGO_ENABLED": "0", "GOMAXPROCS": "2", "GOCACHE": "/tmp/cache", "GOPATH": "/tmp/go"},
        "networks": {"platform_net": None},
        "volumes": [{"type": "bind", "source": str(root / "agent"), "target": "/source", "read_only": True}],
        "command": ["go", "test", "-count=1", "-p=1", "-timeout=180s", "./common/client", "./common/delivery", "./common/queue", "./common/health", "./common/config"],
    }
    for name, service in c["services"].items():
        require("labels" not in service, f"unexpected input labels: {name}")
        service.update(container_name=prefix + "-" + name, restart="no", labels={OWNER: owner})
        if name == "agent-runtime":
            service.setdefault("pids_limit", 128)
        if name in ("agent-runtime-init", "sensor-agent-spool-init"):
            service.update(cpus=0.25, mem_limit="128m", pids_limit=64)
        if "build" in service:
            require("labels" not in service["build"], "unexpected input build labels")
            service["build"]["labels"] = {OWNER: owner}
            service["image"] = prefix + ":" + name
        service["ports"] = ([{"target": 8444, "published": "0", "host_ip": "127.0.0.1", "protocol": "tcp"}]
                            if name == "sensor-infra" else [])
        for mount in service.get("volumes", []):
            if mount["type"] != "bind":
                continue
            require(mount.get("bind", {}) in ({}, {"create_host_path": True}, {"create_host_path": False}),
                    "unexpected input bind options")
            mount["bind"] = {"create_host_path": False}
            source = Path(mount["source"])
            if name == "postgres" and mount["target"] == POSTGRES_INIT_TARGET:
                require(mount["source"] == str(root / POSTGRES_INIT_SOURCE),
                        "unexpected PostgreSQL bootstrap source")
                mount["source"] = str(evidence / POSTGRES_INIT_FIXTURE)
                continue
            for old, new in ((root / "deploy/docker/certs", evidence / PKI),
                             (root / "tests/agent-runtime-e2e/certs", evidence / "agent-certs")):
                if source.is_relative_to(old):
                    mount["source"] = str(new / source.relative_to(old))
                    break
    for kind in ("volumes", "networks"):
        for name, resource in c[kind].items():
            require("labels" not in resource, f"unexpected input resource labels: {name}")
            resource["name"] = prefix + "-" + name
            if not resource.get("external"):
                resource["labels"] = {OWNER: owner}
    verify(c, prefix, root, evidence, owner=owner)
    return c

class Custody:
    def __init__(self, docker, prefix, owner, evidence):
        self.docker = docker
        self.prefix = prefix
        self.owner = owner
        self.evidence = evidence
        self.anchor = prefix + "-custody"
        self.deadline = None
        self.log = None

    def query(self, *args):
        if self.deadline is None:
            # The workload supervisor owns the aggregate deadline for this group.
            return subprocess.check_output(self.docker + list(args), text=True)
        remaining = self.deadline - time.monotonic() - REAP_GRACE
        if remaining <= 0:
            raise TimeoutError("teardown deadline")
        stderr = subprocess.STDOUT if args[0] == "logs" else self.log
        process = subprocess.Popen(self.docker + list(args), stdout=subprocess.PIPE, stderr=stderr,
                                   text=True, start_new_session=True)
        try:
            output, _ = process.communicate(timeout=min(15, remaining))
        except subprocess.TimeoutExpired:
            kill_group(process, signal.SIGKILL)
            process.stdout.close()
            reap(process, self.deadline)
            raise
        if process.returncode:
            raise subprocess.CalledProcessError(process.returncode, args)
        return output

    def inspect(self, kind, ref):
        rows = json.loads(self.query(kind, "inspect", ref))
        require(isinstance(rows, list) and len(rows) == 1 and isinstance(rows[0], dict),
                f"ambiguous {kind} inspect: {ref}")
        return rows[0]

    def inventory(self, *, fresh=False):
        inventory = {}
        owned_containers = set()
        for kind, command, fields in (
            ("container", ["container", "ls", "-a", "--no-trunc"], ("ID", "Names")),
            ("network", ["network", "ls", "--no-trunc"], ("ID", "Name")),
            ("volume", ["volume", "ls"], ("Name",)),
            ("image", ["image", "ls", "--no-trunc"], ("ID", "Repository", "Tag")),
        ):
            fmt = "\t".join("{{." + field + "}}" for field in fields)
            rows = self.query(*command, "--format", fmt).splitlines()
            selected = set()
            for label in (f"{OWNER}={self.owner}", f"{PROJECT}={self.prefix}"):
                selected.update(self.query(*command, "--filter", f"label={label}", "--format", fmt).splitlines())
            require(selected <= set(rows), f"ambiguous {kind} inventory")
            resources = {}
            names = set()
            for row in rows:
                parts = row.split("\t")
                require(len(parts) == len(fields) and all(parts), f"malformed {kind} inventory")
                ref, name = (parts[0], parts[-1]) if kind != "image" else (parts[0], ":".join(parts[1:]))
                if not (row in selected or name == self.prefix or name.startswith((self.prefix + "-", self.prefix + ":"))):
                    continue
                require(not fresh, f"private project collision: {kind} {name}")
                require(name not in names and ref not in resources and (kind == "volume" or
                        ID.fullmatch(ref.removeprefix("sha256:"))), f"ambiguous {kind} identity: {name}")
                item = self.inspect(kind, ref)
                self.check(kind, ref, name, item)
                names.add(name)
                resources[ref] = (name, item)
            inventory[kind] = resources
            if kind == "container":
                owned_containers = resources.keys()
            if kind == "network":
                for name, item in resources.values():
                    require(set(item.get("Containers") or {}) <= owned_containers,
                            f"foreign network attachment: {name}")
            if kind == "volume":
                for _, container in inventory["container"].values():
                    for mount in container.get("Mounts", ()):
                        if mount.get("Type") == "volume":
                            require(mount.get("Name") in resources, "unexpected or unowned container volume")
                for ref in resources:
                    users = self.query("container", "ls", "-aq", "--no-trunc", "--filter", f"volume={ref}").splitlines()
                    require(set(users) <= owned_containers, f"foreign volume attachment: {ref}")
        return inventory

    def check(self, kind, ref, name, item):
        labels = (item.get("Config", {}).get("Labels") if kind in {"image", "container"}
                  else item.get("Labels")) or {}
        project = labels.get(PROJECT)
        own = labels.get(OWNER) == self.owner
        leaf = name.removeprefix(self.prefix + "-")
        if kind != "image":
            require(name == self.prefix + "-" + leaf, f"unexpected name for labelled {kind}: {name}")
        if kind == "container":
            service = labels.get(COMPOSE_SERVICE)
            allowed = (leaf == INVALID_CONFIG and service == INVALID_CONFIG_SERVICE) or (
                leaf in HELPERS and service is None
            ) or (
                service in IMAGES.keys() | BUILDS.keys() and leaf == service
            ) or (service in {"kafka", "unit-contracts"} and re.fullmatch(service + r"-run-[a-f0-9]{12}", leaf))
            valid = item.get("Name") == "/" + name and item.get("Id") == ref and project == self.prefix and allowed
        elif kind == "network":
            valid = (leaf in {"custody", "platform_net"} and item.get("Name") == name
                     and item.get("Id") == ref and item.get("Driver") == "bridge"
                     and project == self.prefix and (
                         (leaf == "custody" and item.get("Internal") is True)
                         or (leaf == "platform_net" and labels.get("com.docker.compose.network") == leaf)))
        elif kind == "volume":
            valid = (leaf in VOLUME_KEYS and item.get("Name") == ref == name
                     and item.get("Driver") == "local" and not item.get("Options"))
            if leaf == "platform_kafka_data":
                valid = valid and all(labels.get(key) == self.prefix for key in
                                      ("issue223.owner", "platform.kafka.project", "platform.kafka.storage-id"))
                valid = valid and labels.get("platform.kafka.resource") == leaf
            else:
                valid = valid and project == self.prefix and labels.get("com.docker.compose.volume") == leaf
        else:
            valid = (name in {self.prefix + ":" + service for service in BUILDS}
                     and item.get("Id") == ref and item.get("RepoTags") == [name])
        require(own and valid, f"unproven ownership: {kind} {name}")
        if kind == "container" and leaf == INVALID_CONFIG:
            image = item.get("Image", "")
            require(isinstance(image, str) and image.startswith("sha256:") and ID.fullmatch(image[7:])
                    and item.get("Mounts") == [] and item.get("HostConfig", {}).get("NetworkMode") == "none",
                    f"unproven invalid-config isolation: {name}")
            self.check("image", image, self.prefix + ":agent-runtime", self.inspect("image", image))

    def admit(self):
        self.inventory(fresh=True)
        # Journal before the first daemon write so partial creation has custody.
        (self.evidence / "reservation-attempted").touch(exist_ok=False)
        rc = self.mutate(["network", "create", "--driver", "bridge", "--internal",
                          "--label", f"{OWNER}={self.owner}", "--label", f"{PROJECT}={self.prefix}", self.anchor],
                         "network", "create")
        require(rc == 0, "private reservation creation failed")
        inventory = self.inventory()
        refs = [ref for ref, (name, _) in inventory["network"].items() if name == self.anchor]
        require(len(refs) == 1, "private ownership reservation missing")
        (self.evidence / "reservation-id").write_text(refs[0] + "\n")

    def active(self):
        inventory = self.inventory()
        receipt = self.evidence / "reservation-id"
        ref = receipt.read_text().strip() if receipt.is_file() else ""
        require(ref in inventory["network"] and inventory["network"][ref][0] == self.anchor,
                "private ownership reservation missing or replaced")
        return inventory

    def journal(self, category, verb, services):
        pending = self.evidence / ("pending-" + secrets.token_hex(12))
        self.write_journal(pending, category, verb, services)
        return pending

    def write_journal(self, pending, category, verb, services, rc=None):
        # Callers supply labels from the admitted operation, never command options or object IDs.
        record = {"category": category, "verb": verb, "services": services, "outcome": "submitted"}
        if rc is not None:
            record["outcome"] = "uncertain"
            if 0 < rc <= 255:
                record.update(outcome="returned-nonzero", exit_code=rc)
        with tempfile.NamedTemporaryFile(mode="w", dir=self.evidence, prefix=".journal-",
                                         delete=False) as temporary:
            source = Path(temporary.name)
            try:
                temporary.write(json.dumps(record) + "\n")
                temporary.flush()
                os.fsync(temporary.fileno())
                os.replace(source, pending)
            finally:
                source.unlink(missing_ok=True)

    def mutate(self, args, category, verb, services=()):
        pending = self.journal(category, verb, services)
        rc = subprocess.call(self.docker + args)
        if rc == 0:
            pending.unlink()
        else:
            self.write_journal(pending, category, verb, services, rc)
        return rc

    def exec_fixture(self, args, program_index, service):
        # A foreground exec can return an application status or a transport
        # failure. For fixture probes that expect failure, capture an exit
        # receipt from inside the container without changing their stdout.
        pending = self.journal("container", "exec", [service])
        token = secrets.token_hex(16)
        command = [*args[:program_index], "sh", "-c",
                   '"$@"; code=$?; printf "\\n%s:%s\\n" "$0" "$code" >&2; exit "$code"',
                   token, *args[program_index:]]
        result = subprocess.run(self.docker + command, stderr=subprocess.PIPE)
        receipt = f"\n{token}:{result.returncode}\n".encode()
        reported = result.stderr.endswith(receipt)
        sys.stderr.buffer.write(result.stderr[:-len(receipt)] if reported else result.stderr)
        if result.returncode == 0 or reported and 0 < result.returncode < 256:
            pending.unlink()
        else:
            self.write_journal(pending, "container", "exec", [service], result.returncode)
        return result.returncode

    def invalid_config(self, args, name):
        # Disable auto-removal long enough to prove the exit on an immutable ID.
        services = ["agent-runtime"]
        pending = self.journal("invalid-config", "create", services)
        # Override the service label inherited from the Compose-built image.
        create = ["create", "--label", f"{COMPOSE_SERVICE}={INVALID_CONFIG_SERVICE}",
                  *(arg for arg in args[1:] if arg != "--rm")]
        result = subprocess.run(self.docker + create, stdout=subprocess.PIPE, text=True)
        if result.returncode:
            self.write_journal(pending, "invalid-config", "create", services, result.returncode)
        require(result.returncode == 0, "invalid-config creation unconfirmed")
        ref = result.stdout.strip()
        containers = self.active()["container"]
        require(ID.fullmatch(ref) and ref in containers and containers[ref][0] == name
                and containers[ref][1].get("Image") == args[-1], "invalid-config creation unconfirmed")
        self.write_journal(pending, "invalid-config", "start", services)
        rc = subprocess.call(self.docker + ["start", "-a", ref])
        if rc:
            self.write_journal(pending, "invalid-config", "start", services, rc)
        item = self.inspect("container", ref)
        self.check("container", ref, name, item)
        state = item["State"]
        require(item.get("Image") == args[-1] and rc in {0, 1}
                and state.get("Status") == "exited" and state.get("ExitCode") == 1
                and state.get("Error") == "" and state.get("FinishedAt", "") > "0001-01-01T00:00:00Z",
                "invalid-config exit unconfirmed")
        self.query("container", "rm", ref)
        containers = self.active()["container"]
        require(ref not in containers and all(entry != name for entry, _ in containers.values()),
                "invalid-config removal unconfirmed")
        pending.unlink()
        return 1

    def teardown(self):
        ok = True
        had_pending = any(self.evidence.glob("pending-*"))
        # Leave a budget for the final independent absence inventory.
        final_deadline = self.deadline
        self.deadline -= min(20, max(0, (self.deadline - time.monotonic()) / 3))
        try:
            receipt = self.evidence / "reservation-id"
            inventory = self.active() if receipt.exists() else self.inventory()
            for kind in ("container", "network", "volume", "image"):
                for ref, (name, item) in inventory[kind].items():
                    if kind == "network" and name == self.anchor:
                        continue
                    if kind == "container":
                        state_dir = self.evidence / "container-state"
                        state_dir.mkdir(exist_ok=True)
                        (state_dir / (ref + ".json")).write_text(json.dumps(item))
                        try:
                            logs = self.query("logs", ref)
                            with (self.evidence / "stack.log").open("a") as stack:
                                stack.write(f"\n=== {name} ===\n" + logs)
                            if name == self.prefix + "-clickhouse":
                                self.query("cp", ref + ":/var/log/clickhouse-server", str(state_dir / "clickhouse-logs"))
                        except GATE_ERRORS as exc:
                            ok = False
                            print(f"evidence collection failed: {exc}", file=self.log or sys.stderr)
                        if item.get("State", {}).get("Paused"):
                            self.query("unpause", ref)
                    try:
                        self.check(kind, ref, name, self.inspect(kind, ref))
                        options = {"container": ["--force"], "image": ["--no-prune"]}.get(kind, [])
                        self.query(kind, "rm", *options, ref)
                    except GATE_ERRORS as exc:
                        ok = False
                        print(f"cleanup removal failed: {exc}", file=self.log or sys.stderr)
            remaining = self.inventory()
            anchors = remaining["network"]
            if ok and not had_pending and len(anchors) == 1 and sum(map(len, remaining.values())) == 1:
                ref, (name, _) = next(iter(anchors.items()))
                receipt = self.evidence / "reservation-id"
                require(name == self.anchor and (not receipt.exists() or receipt.read_text().strip() == ref),
                        "unexpected final reservation identity")
                self.check("network", ref, name, self.inspect("network", ref))
                self.query("network", "rm", ref)
        except GATE_ERRORS as exc:
            ok = False
            print(f"cleanup inventory or operation failed: {exc}", file=self.log or sys.stderr)
        self.deadline = final_deadline
        try:
            remaining = self.inventory()
            for kind, resources in remaining.items():
                (self.evidence / f"remaining-{kind}s.txt").write_text(
                    "".join(name + "\n" for name, _ in resources.values()))
            require(not any(remaining.values()), "private resources remain")
        except GATE_ERRORS as exc:
            ok = False
            print(f"absence unverified: {exc}", file=self.log or sys.stderr)
        if had_pending:
            print("interrupted Docker operation may still be in flight", file=self.log or sys.stderr)
        return 0 if ok and not had_pending else 1


def read_config(file):
    def unique(pairs):
        obj = dict(pairs)
        require(len(obj) == len(pairs), "duplicate Compose property")
        return obj
    return json.loads(Path(file).read_text(), object_pairs_hook=unique)


def kill_group(process, sig):
    try:
        os.killpg(process.pid, sig)
    except ProcessLookupError:
        pass


def reap(process, deadline):
    try:
        process.wait(timeout=min(REAP_GRACE, max(0, deadline - time.monotonic())))
    except subprocess.TimeoutExpired:
        pass


def supervise(command, cwd, env, work_seconds, cleanup_seconds, custody):
    interrupted = None

    def latch(sig, _frame):
        nonlocal interrupted
        if interrupted is None:
            interrupted = sig

    previous = {sig: signal.signal(sig, latch) for sig in (signal.SIGTERM, signal.SIGINT)}
    work_exit = 1
    cleanup_exit = 0
    try:
        deadline = time.monotonic() + work_seconds
        stop_at = deadline - REAP_GRACE
        if interrupted is None:
            process = subprocess.Popen(command, cwd=cwd, env=env, start_new_session=True,
                                       stdin=subprocess.DEVNULL)
            try:
                while process.poll() is None and interrupted is None and time.monotonic() < stop_at:
                    time.sleep(min(0.05, max(0, stop_at - time.monotonic())))
                if interrupted is not None or process.poll() is None:
                    work_exit = 128 + interrupted if interrupted is not None else 124
                    kill_group(process, signal.SIGTERM)
                    time.sleep(min(1, max(0, stop_at - time.monotonic())))
                else:
                    work_exit = process.returncode if process.returncode >= 0 else 128 - process.returncode
            finally:
                # Also stop descendants left behind by an already exited shell.
                kill_group(process, signal.SIGKILL)
                reap(process, deadline)
        else:
            work_exit = 128 + interrupted
    finally:
        try:
            if (custody.evidence / "reservation-attempted").exists():
                custody.deadline = time.monotonic() + cleanup_seconds
                with (custody.evidence / "cleanup.log").open("w") as log:
                    custody.log = log
                    try:
                        cleanup_exit = custody.teardown()
                    finally:
                        custody.log = None
                    log.write(f"cleanup_exit={cleanup_exit}\n")
            if work_exit == 0 and interrupted is not None:
                work_exit = 128 + interrupted
            (custody.evidence / "exits.txt").write_text(
                f"workload_exit={work_exit}\ncleanup_exit={cleanup_exit}\n")
            (custody.evidence / "end.utc").write_text(time.strftime("%Y-%m-%dT%H:%M:%SZ", time.gmtime()) + "\n")
        finally:
            for sig, handler in previous.items():
                signal.signal(sig, handler)
    return work_exit or cleanup_exit


def raw_run(args, custody, inventory):
    """Constrain the non-Compose PKI, Kafka and invalid-config helpers."""
    options = []
    mounts = []
    requested_name = None
    network = None
    flags = {"--rm", "-i", "--read-only"}
    values = {"--name", "--label", "--network", "--cpus", "--memory", "--pids-limit",
              "--cap-drop", "--security-opt", "--entrypoint", "--user", "-u", "-w",
              "--tmpfs", "--mount", "-v", "-e"}
    index = 0
    while index < len(args) and args[index].startswith("-"):
        option = args[index]
        index += 1
        if option in flags:
            options.append(option)
            continue
        require(option in values and index < len(args), "unexpected helper option")
        value = args[index]
        index += 1
        if option in {"--mount", "-v"}:
            mounts.append((option, value))
        if option == "--name":
            require(requested_name is None, "duplicate helper name")
            requested_name = value
            continue
        if option == "--network":
            require(network is None and value == "none", "unexpected helper network")
            network = value
            continue
        if option == "--label":
            require(value in {f"{PROJECT}={custody.prefix}", f"issue223.owner={custody.prefix}"},
                    "unexpected helper label")
            if value.startswith(PROJECT + "="):
                continue
        if option == "--tmpfs":
            require(value in {"/etc/kafka/secrets", "/mnt/shared/config"}, "unexpected helper tmpfs")
        options.extend((option, value))
    require(index < len(args) and "--rm" in options, "unsupported helper invocation")
    image, *command = args[index:]
    if image == "eclipse-temurin:21-jre-alpine":
        helper = "pki"
        pki = custody.evidence / PKI
        require(mounts == [("-v", f"{pki}:/certs")], "unexpected PKI helper mount")
        require(network is None and options == [
            "--rm", "-u", f"{os.getuid()}:{os.getgid()}", "-v", f"{pki}:/certs", "-w", "/certs",
        ] and len(command) == 13 and command[:10] == [
            "keytool", "-importcert", "-noprompt", "-alias", "platform-root-ca", "-file", "/certs/ca.crt",
            "-keystore", "/certs/core-truststore.p12", "-storepass",
        ] and command[11:] == ["-storetype", "PKCS12"], "unexpected PKI helper invocation")
        audit_tree(pki, private=True)
    elif image == IMAGES["kafka"]:
        helper = "kafka-helper"
        volume = custody.prefix + "-platform_kafka_data"
        require(volume in inventory["volume"], "unowned Kafka helper volume")
        storage = f"type=volume,src={volume},dst=/var/lib/kafka/data"
        bind = f"type=bind,src={ROOT / 'deploy/docker/kafka'},dst=/platform-kafka,readonly"
        require(mounts in ([("--mount", storage)], [("--mount", storage + ",readonly"), ("--mount", bind)]),
                "unexpected Kafka helper mount")
        readonly = len(mounts) == 2
        common = ["--rm", "--cpus", "1" if readonly else "0.25", "--memory", "512m" if readonly else "128m",
                  "--user", "1000:1000" if readonly else "0",
                  "--tmpfs", "/etc/kafka/secrets", "--tmpfs", "/mnt/shared/config",
                  "--label", f"issue223.owner={custody.prefix}"]
        mount_args = [arg for mount in mounts for arg in mount]
        suffix = [*mount_args, "--entrypoint", "/bin/bash"]
        env_args = options[len(common):-len(suffix)]
        require(network == "none" and options[:len(common)] == common and options[-len(suffix):] == suffix,
                "unexpected Kafka helper options")
        if readonly:
            require(env_args and len(env_args) % 2 == 0 and all(
                flag == "-e" and re.fullmatch(r"[A-Z][A-Z0-9_]*=[^\r\n]*", value)
                for flag, value in zip(env_args[::2], env_args[1::2])
            ) and command == ["/platform-kafka/guarded-start.sh", "--inspect"], "unexpected Kafka inspection")
        else:
            require(not env_args and command == ["-c",
                'test -z "$(find /var/lib/kafka/data -mindepth 1 -maxdepth 1 -print -quit)" && chown 1000:1000 /var/lib/kafka/data',
            ], "unexpected Kafka volume initialization")
        custody.check("volume", volume, volume, custody.inspect("volume", volume))
        audit_tree(ROOT / "deploy/docker/kafka")
    else:
        require(not mounts, "unexpected helper mount")
        if image == "node:20-alpine":
            helper = "parser"
            require(network == "none" and options == ["--rm", "-i", "--cpus", "0.25", "--memory", "128m"]
                    and len(command) == 3 and command[:2] == ["node", "-e"],
                    "unexpected parser invocation")
        else:
            helper = INVALID_CONFIG
            require(image == custody.prefix + ":agent-runtime"
                    and any(name == image for name, _ in inventory["image"].values()),
                    "unexpected helper image")
    name = custody.prefix + "-" + helper
    require(requested_name is None or requested_name == name, "unexpected helper name")
    require(not any(entry == name for entry, _ in inventory["container"].values()), "helper name in use")
    return ["run", "--name", name, "--network", "none",
            "--label", f"{OWNER}={custody.owner}", "--label", f"{PROJECT}={custody.prefix}",
            *options, *args[index:]]


def owned_target(resources, target):
    matches = [(ref, name) for ref, (name, _) in resources.items() if target in {ref, name}]
    require(len(matches) == 1, "target has no custody")
    return matches[0]


def read_target(args, option_names):
    options = {}
    targets = []
    while args:
        arg, *args = args
        if arg.startswith("-"):
            require(arg in option_names, "unexpected Docker read option")
            option = option_names[arg]
            require(option not in options, "duplicate Docker read option")
            if option == "--no-stream":
                options[option] = None
            else:
                require(args and not args[0].startswith("-"), "missing Docker read option value")
                options[option], *args = args
        else:
            targets.append(arg)
    require(len(targets) == 1, "expected exactly one Docker read target")
    return targets[0], options


def list_owned(custody, args):
    prefix = custody.prefix
    volume = prefix + "-platform_kafka_data"
    by_project = ["ps", "-aq", "--filter", f"label={PROJECT}={prefix}", "--filter",
                  "label=com.docker.compose.service=kafka"]
    by_name = ["ps", "-aq", "--filter", f"name=^/{prefix}-kafka$"]
    by_volume = ["ps", "-aq", "--filter", f"volume={volume}"]
    volumes = ["volume", "ls", "--format", "{{.Name}}"]
    running = ["ps", "--format", "{{.Names}}"]
    require(args in (by_project, by_name, by_volume, volumes, running), "unexpected Docker list invocation")
    inventory = custody.active()
    if args == volumes:
        rows = inventory["volume"]
    else:
        rows = []
        for ref, (name, item) in inventory["container"].items():
            if args == running:
                if item.get("State", {}).get("Running") is True:
                    rows.append(name)
            elif args == by_project:
                if item["Config"]["Labels"].get("com.docker.compose.service") == "kafka":
                    rows.append(ref)
            elif args == by_name:
                if name == prefix + "-kafka":
                    rows.append(ref)
            elif any(m.get("Type") == "volume" and m.get("Name") == volume for m in item.get("Mounts", ())):
                rows.append(ref)
    for row in rows:
        print(row)
    return 0


def resource_read(custody, args, kind, verb):
    format_options = {"-f": "--format", "--format": "--format"}
    forms = {
        "inspect": format_options,
        "logs": {"--tail": "--tail"},
        "wait": {},
        "stats": {**format_options, "--no-stream": "--no-stream"},
    }
    target, options = read_target(args, forms[verb])
    fmt = options.get("--format")
    if verb == "inspect":
        formats = ({None, KAFKA_VOLUME_FORMAT} if kind == "volume" else
                   {None, HEALTH_FORMAT, "{{.State.ExitCode}}", MATRIX_HEALTH_FORMAT,
                    MATRIX_STARTED_AT_FORMAT} | KAFKA_CONTAINER_FORMATS)
        require(fmt in formats, "unexpected Docker inspect format")
    elif verb == "logs":
        require(options.get("--tail") in {None, "80", "120"}, "unexpected Docker log tail")
    elif verb == "stats":
        require(options == {"--no-stream": None, "--format": "{{json .}}"}, "unexpected Docker stats options")
    inventory = custody.active()[kind]
    if (kind == "container" and fmt == HEALTH_FORMAT
            and target in {custody.prefix + "-" + service for service in ("core", "sensor-infra")}
            and all(target != name for name, _ in inventory.values())):
        return 1
    ref, name = owned_target(inventory, target)
    if kind == "container" and fmt in {MATRIX_HEALTH_FORMAT, MATRIX_STARTED_AT_FORMAT}:
        allowed = {custody.prefix + "-core"}
        if fmt == MATRIX_HEALTH_FORMAT:
            allowed.add(custody.prefix + "-sensor-infra")
        require(name in allowed, "unexpected matrix inspect service")
        custody.check(kind, ref, name, custody.inspect(kind, ref))
    if kind == "volume":
        require(target == name, "volume requires its exact name")
        custody.check(kind, ref, name, custody.inspect(kind, ref))
    command = [kind, verb] if verb == "inspect" else [verb]
    for option, value in options.items():
        command.append(option)
        if value is not None:
            command.append(value)
    os.execv(custody.docker[0], custody.docker + command + [ref])


def docker_gate(custody, args):
    prefix = custody.prefix
    execution = custody.evidence / "compose.json"
    expected = ["compose", "-p", prefix, "-f", str(execution)]
    if args[:2] == ["compose", "version"]:
        require(args == ["compose", "version", "--short"], "unexpected Compose version arguments")
    elif args[:len(expected)] == expected:
        require(len(args) > len(expected), "missing Compose command")
        verb = args[len(expected)]
        require(verb in {"config", "port", "build", "up", "run", "start", "restart"},
                "unexpected Compose command")
        require(hashlib.sha256(execution.read_bytes()).hexdigest() ==
                (custody.evidence / "compose.sha256").read_text(), "validated Compose file changed")
        verify(read_config(execution), prefix, evidence=custody.evidence, owner=custody.owner,
               ready=verb in {"build", "up", "run", "start", "restart"})
        tail = args[len(expected) + 1:]
        kafka_run = ["-T", "--rm", "--no-deps", "--user", "1000:1000",
                     "-e", f"PLATFORM_KAFKA_NEW_VOLUME_CONFIRM={prefix}", "--entrypoint", "/bin/bash",
                     "kafka", "/platform-kafka/provision-fresh.sh", "--new-empty-volume"]
        plans = {
            "config": [["--format", "json"]],
            "port": [["sensor-infra", "8444"]],
            "build": [[service] for service in BUILDS],
            "up": [
                ["-d", "postgres", "clickhouse", "kafka", "valkey", "core"],
                ["-d", "--no-build", "--force-recreate", "sensor-infra"],
                ["-d", "--force-recreate", "--no-deps", "agent-runtime-init"],
                ["-d", "--no-deps", "--no-build", "--force-recreate", "agent-runtime"],
            ],
            "run": [kafka_run, ["--rm", "--no-deps", "unit-contracts"]],
            "start": [["core"]],
            "restart": [["sensor-infra"], ["core"]],
        }
        require(tail in plans[verb], "unexpected fixture Compose invocation")
        # Only the prepared config can be interpreted, with an explicit empty env file.
        args = [*expected, "--env-file", str(custody.evidence / "empty.env"), *args[len(expected):]]
        if verb == "port":
            owned_target(custody.active()["container"], prefix + "-sensor-infra")
        if verb not in {"config", "port"}:
            custody.active()
            services = [arg for arg in tail if arg in IMAGES or arg in BUILDS]
            if verb == "run":
                service = "kafka" if tail == kafka_run else "unit-contracts"
                services = [service]
                name = f"{prefix}-{service}-run-{secrets.token_hex(6)}"
                position = len(expected) + 3  # Includes the explicit env-file option pair.
                args[position:position] = ["--name", name]
            return custody.mutate(args, "compose", verb, services)
    elif args[:2] == ["volume", "create"]:
        inventory = custody.active()
        volume = prefix + "-platform_kafka_data"
        expected_create = [
            "volume", "create", "--label", f"issue223.owner={prefix}", "--label",
            f"platform.kafka.project={prefix}", "--label", f"platform.kafka.storage-id={prefix}",
            "--label", "platform.kafka.resource=platform_kafka_data", volume,
        ]
        require(args == expected_create and volume not in inventory["volume"], "unexpected Kafka volume creation")
        return custody.mutate([*args[:-1], "--label", f"{OWNER}={custody.owner}", volume], "volume", "create")
    elif args[0] == "run":
        inventory = custody.active()
        command = raw_run(args[1:], custody, inventory)
        name = command[2]
        if name == prefix + "-" + INVALID_CONFIG:
            require(args[1:] == [
                "--rm", "--name", name, "--label", f"{PROJECT}={prefix}", "--network", "none",
                "--read-only", "--cpus", "0.25", "--memory", "128m", "--pids-limit", "32",
                "--cap-drop", "ALL", "--security-opt", "no-new-privileges:true",
                "--entrypoint", "/usr/local/bin/platform-agent-linux", prefix + ":agent-runtime",
            ], "unexpected invalid-config invocation")
            command[-1], = (ref for ref, (tag, _) in inventory["image"].items() if tag == command[-1])
            return custody.invalid_config(command, name)
        return custody.mutate(command, "helper", "run")
    elif args[0] in {"start", "stop", "restart", "kill", "pause", "unpause", "exec"}:
        target = 2 if args[:2] == ["exec", "-i"] else 1
        ref, name = owned_target(custody.active()["container"], args[target] if len(args) > target else "")
        args = [*args[:target], ref, *args[target + 1:]]
        service = name.removeprefix(prefix + "-")
        if args[0] == "exec":
            require(len(args) > target + 1 and args[target + 1] in EXEC_PROGRAMS.get(service, ()),
                    "unexpected fixture exec program")
            if args[target + 1] in {"wget", "/opt/kafka/bin/kafka-console-consumer.sh"}:
                return custody.exec_fixture(args, target + 1, service)
        else:
            require(len(args) == 2, "unexpected container arguments")
        services = [service] if service in IMAGES or service in BUILDS else []
        return custody.mutate(args, "container", args[0], services)
    elif args[0] == "ps" or args[:2] == ["volume", "ls"]:
        return list_owned(custody, args)
    elif args[:2] == ["volume", "inspect"]:
        return resource_read(custody, args[2:], "volume", "inspect")
    elif args[0] in {"inspect", "logs", "wait", "stats"}:
        return resource_read(custody, args[1:], "container", args[0])
    elif args[0] == "cp":
        require(len(args) == 3 and ":" in args[1] and ":" not in args[2], "only copy from fixture containers")
        require(args[2] == "-", "copy destination is outside evidence stream")
        target, path = args[1].split(":", 1)
        containers = custody.active()["container"]
        ref, name = owned_target(containers, target)
        if name == prefix + "-agent-runtime":
            require(path == "/var/lib/platform/agent/queue/sequence.meta", "unexpected fixture copy path")
            item = custody.inspect("container", ref)
            custody.check("container", ref, name, item)
            state = item.get("State") or {}
            require(state.get("Running") is True and state.get("Paused") is True,
                    "sequence copy requires the owned paused Agent")
        else:
            require(name == prefix + "-kafka" and path in KAFKA_COPY_PATHS, "unexpected fixture copy path")
        args = ["cp", ref + ":" + path, "-"]
    else:
        raise ValueError("unexpected Docker command")

    os.execv(custody.docker[0], custody.docker + args)


def docker_command(env):
    return [env["E2E_DOCKER"], "--host", "unix:///var/run/docker.sock"]


def workload(custody):
    evidence = custody.evidence
    original = evidence / "rendered-original.json"
    execution = evidence / "compose.json"
    for name, args in (
        ("source-head.txt", ["git", "rev-parse", "HEAD", "HEAD^{tree}"]),
        ("source-status.txt", ["git", "status", "--porcelain=v1"]),
        ("source.diff", ["git", "diff", "--binary"]),
        ("source-files.sha256", ["bash", "-o", "pipefail", "-c",
                                 "git ls-files -co --exclude-standard -z | xargs -0 sha256sum"]),
    ):
        with (evidence / name).open("w") as output:
            subprocess.run(args, stdout=output, check=True)
    require(custody.query("compose", "version", "--short").strip() == "5.5.0",
            "the guarded fixture requires Compose 5.5.0 in the private CLI configuration")
    original.write_text(custody.query(
        "compose", "--env-file", ".env.example", "-f", "compose.yaml", "-f", "compose.kafka-guarded.yaml",
        "-f", str(HERE / "compose.override.yaml"), "config", "--no-env-resolution", "--format", "json"))
    config = isolate(read_config(original), custody.prefix, ROOT, evidence, custody.owner)
    stage_postgres_init(ROOT, evidence)
    # Compose 5.5.0 config JSON already escapes literal dollars for reparse.
    execution.write_text(json.dumps(config, indent=2))
    dc = ["compose", "-p", custody.prefix, "-f", str(execution), "--env-file", str(evidence / "empty.env")]
    normalized = evidence / "rendered-isolated.json"
    normalized.write_text(custody.query(*dc, "config", "--no-env-resolution", "--format", "json"))
    verify(read_config(normalized), custody.prefix, evidence=evidence, owner=custody.owner)
    (evidence / "compose.sha256").write_text(hashlib.sha256(execution.read_bytes()).hexdigest())
    custody.admit()

    pki_script = evidence / "pki/scripts/generate-dev-certs.sh"
    pki_script.parent.mkdir(parents=True)
    shutil.copyfile(ROOT / "scripts/generate-dev-certs.sh", pki_script)
    if os.environ.get("PLATFORM_E2E_REAL_SENSOR_GAP") == "1":
        (evidence / "timesync-marker").touch()
    with (evidence / "pki.log").open("w") as output:
        for script in (pki_script, HERE / "generate-certs.sh"):
            subprocess.run(["bash", str(script)], stdout=output, stderr=subprocess.STDOUT, check=True)
    # The outer evidence directory stays 0700. The mounted directory itself
    # needs traversal permission for the Core image's non-root user.
    (evidence / PKI).chmod(0o755)
    verify(read_config(execution), custody.prefix, evidence=evidence, ready=True, owner=custody.owner)
    with (evidence / "kafka-provision.log").open("w") as output:
        subprocess.run(["bash", "deploy/docker/kafka/deployment-preflight.sh", "provision",
                        "-p", custody.prefix, "-f", str(execution)], stdout=output, check=True)
    for service in BUILDS:
        with (evidence / f"build-{service}.log").open("w") as output:
            subprocess.run(["docker", "compose", "-p", custody.prefix, "-f", str(execution),
                            "build", service], stdout=output, stderr=subprocess.STDOUT, check=True)
    return subprocess.call(["bash", str(HERE / "run.sh")])


def launch():
    evidence = Path(os.environ["PLATFORM_E2E_RUN_DIR"])
    canonical(evidence)
    require(not evidence.is_relative_to(ROOT) and not any(char in str(evidence) for char in "$,:\n\r"),
            "evidence must be outside the checkout and use a plain canonical path")
    cli = shutil.which("docker")
    require(cli is not None, "Docker CLI unavailable")
    # Use the local daemon and empty Docker/Compose config. Keep only the host
    # process essentials and explicitly supported fixture mode switches.
    env = {key: os.environ[key] for key in ("PATH", "HOME", "LANG", "LC_ALL",
           "PLATFORM_E2E_REAL_SENSOR_GAP", "PLATFORM_E2E_SEQUENCE_DISPOSITION") if key in os.environ}
    def budget(key, default, maximum):
        value = int(os.environ.get(key, default))
        require(1 <= value <= maximum, f"invalid {key}")
        return value
    work = budget("PLATFORM_E2E_WORK_SECONDS", 3600, 7200)
    cleanup = budget("PLATFORM_E2E_CLEANUP_SECONDS", 180, 600)
    os.umask(0o077)
    evidence.mkdir(mode=0o700)
    prefix = "agent-e2e-" + secrets.token_hex(16)
    owner = secrets.token_hex(16)
    cluster = secrets.token_urlsafe(16)
    if cluster.startswith("-"):
        cluster = "A" + cluster[1:]
    env.update(
        E2E_DOCKER=str(Path(cli).resolve()), E2E_CUSTODY=owner, PLATFORM_E2E_PREFIX=prefix,
        PLATFORM_E2E_RUN_DIR=str(evidence), PLATFORM_E2E_CA_DIR=str(evidence / PKI),
        PLATFORM_E2E_CERT_DIR=str(evidence / "agent-certs"),
        PLATFORM_E2E_ISOLATED_CONFIG=str(evidence / "compose.json"),
        PLATFORM_KAFKA_DATA_VOLUME=prefix + "-platform_kafka_data", PLATFORM_KAFKA_STORAGE_ID=prefix,
        PLATFORM_KAFKA_NEW_VOLUME_CONFIRM=prefix, PLATFORM_KAFKA_CLUSTER_ID=cluster,
        COMPOSE_PROFILES="dev", COMPOSE_PARALLEL_LIMIT="1", DOCKER_CONFIG=str(evidence / "docker-config"),
        PYTHONDONTWRITEBYTECODE="1",
    )
    (evidence / "custody.json").write_text(json.dumps({"project": prefix, "owner": owner}) + "\n")
    (evidence / "start.utc").write_text(time.strftime("%Y-%m-%dT%H:%M:%SZ", time.gmtime()) + "\n")
    (evidence / "empty.env").touch()
    (evidence / "docker-config").mkdir()
    shim = evidence / "bin/docker"
    shim.parent.mkdir()
    shim.write_text("#!/bin/sh\nexec " + shlex.join([sys.executable, str(HERE / "isolation.py"), "docker"])
                    + ' "$@"\n')
    shim.chmod(0o700)
    env["PATH"] = str(shim.parent) + os.pathsep + env["PATH"]
    # The supervisor's daemon calls must use the same environment as the worker.
    os.environ.clear()
    os.environ.update(env)
    custody = Custody(docker_command(env), prefix, owner, evidence)
    return supervise([sys.executable, __file__, "workload"], ROOT, env, work, cleanup, custody)


def main(args):
    action = args[0]
    if action == "launch":
        return launch()
    custody = Custody(docker_command(os.environ), os.environ["PLATFORM_E2E_PREFIX"],
                      os.environ["E2E_CUSTODY"], Path(os.environ["PLATFORM_E2E_RUN_DIR"]))
    if action == "verify":
        execution = custody.evidence / "compose.json"
        require(Path(args[1]) == execution and args[2] == custody.prefix
                and shutil.which("docker") == str(custody.evidence / "bin/docker")
                and hashlib.sha256(execution.read_bytes()).hexdigest() ==
                (custody.evidence / "compose.sha256").read_text(), "use the isolated workload entry point")
        verify(read_config(execution), custody.prefix)
        custody.active()
        return 0
    if action == "workload":
        return workload(custody)
    if action == "docker":
        require(len(args) > 1, "missing Docker command")
        return docker_gate(custody, args[1:])
    raise ValueError("unknown isolation command")


if __name__ == "__main__":
    try:
        sys.exit(main(sys.argv[1:]))
    except subprocess.CalledProcessError as exc:
        print(f"E2E isolation: {exc}", file=sys.stderr)
        sys.exit(exc.returncode if exc.returncode >= 0 else 128 - exc.returncode)
    except GATE_ERRORS as exc:
        print(f"E2E isolation: {exc}", file=sys.stderr)
        sys.exit(125)
